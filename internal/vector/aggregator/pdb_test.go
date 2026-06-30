package aggregator

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func TestCreateVectorAggregatorPodDisruptionBudget_Defaults(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{
			Replicas: ptr.To[int32](3),
		}, false)

	pdb := ctrl.createVectorAggregatorPodDisruptionBudget()

	g.Expect(pdb).NotTo(BeNil(), "PodDisruptionBudget should not be nil")

	// We always keep at most one pod unavailable during voluntary disruptions.
	g.Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil(), "MaxUnavailable should be set")
	g.Expect(*pdb.Spec.MaxUnavailable).To(Equal(intstr.FromInt32(1)))
	g.Expect(pdb.Spec.MinAvailable).To(BeNil(), "MinAvailable should not be set")

	// Name and namespace track the aggregator Deployment.
	g.Expect(pdb.ObjectMeta.Name).To(Equal("test-aggregator-aggregator"))
	g.Expect(pdb.ObjectMeta.Namespace).To(Equal("default"))
}

func TestCreateVectorAggregatorPodDisruptionBudget_Selector(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "test-namespace",
		&vectorv1alpha1.VectorAggregatorCommon{
			Replicas: ptr.To[int32](2),
		}, false)

	pdb := ctrl.createVectorAggregatorPodDisruptionBudget()

	// The selector must target only this aggregator's pods, matching the Deployment selector.
	g.Expect(pdb.Spec.Selector).NotTo(BeNil(), "Selector should be set")
	g.Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "Aggregator"))
	g.Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-aggregator"))
}

func TestCreateVectorAggregatorPodDisruptionBudget_ClusterAggregator(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("cluster-test-aggregator", "vector-system",
		&vectorv1alpha1.VectorAggregatorCommon{
			Replicas: ptr.To[int32](4),
		}, true)

	pdb := ctrl.createVectorAggregatorPodDisruptionBudget()

	g.Expect(pdb).NotTo(BeNil())
	g.Expect(*pdb.Spec.MaxUnavailable).To(Equal(intstr.FromInt32(1)))
	g.Expect(pdb.ObjectMeta.Namespace).To(Equal("vector-system"))
	g.Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "cluster-test-aggregator"))
}
