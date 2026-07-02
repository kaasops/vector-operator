package aggregator

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func TestEnsureVectorAggregatorHPA_EnabledCreates(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{
			Autoscaling: vectorv1alpha1.VectorAggregatorAutoscaling{
				Enabled:     true,
				MinReplicas: ptr.To[int32](2),
				MaxReplicas: 4,
			},
		}, false)
	ctrl.Client = newFakeClient(g)

	g.Expect(ctrl.ensureVectorAggregatorHPA(context.Background())).To(Succeed())

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	g.Expect(ctrl.Client.Get(context.Background(),
		types.NamespacedName{Name: "test-aggregator-aggregator", Namespace: "default"}, hpa)).To(Succeed())

	g.Expect(hpa.Spec.MinReplicas).To(HaveValue(Equal(int32(2))))
	g.Expect(hpa.Spec.MaxReplicas).To(Equal(int32(4)))
	g.Expect(hpa.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
	g.Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal("test-aggregator-aggregator"))
	g.Expect(hpa.Spec.ScaleTargetRef.APIVersion).To(Equal("apps/v1"))
}

func TestEnsureVectorAggregatorHPA_DisabledRemovesExisting(t *testing.T) {
	g := NewWithT(t)

	existing := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-aggregator-aggregator", Namespace: "default"},
	}

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{}, false)
	ctrl.Client = newFakeClient(g, existing)

	g.Expect(ctrl.ensureVectorAggregatorHPA(context.Background())).To(Succeed())

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	err := ctrl.Client.Get(context.Background(),
		types.NamespacedName{Name: "test-aggregator-aggregator", Namespace: "default"}, hpa)
	g.Expect(api_errors.IsNotFound(err)).To(BeTrue(), "HPA should be removed when autoscaling is disabled")
}

func TestEnsureVectorAggregatorHPA_DisabledWithoutExistingIsNoop(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{}, false)
	ctrl.Client = newFakeClient(g)

	g.Expect(ctrl.ensureVectorAggregatorHPA(context.Background())).To(Succeed())
}
