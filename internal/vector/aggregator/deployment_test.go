package aggregator

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// zoneSpreadConstraint spreads pods evenly across zones without ever leaving one
// Pending, which is the case the field exists for.
func zoneSpreadConstraint() corev1.TopologySpreadConstraint {
	return corev1.TopologySpreadConstraint{
		MaxSkew:           1,
		TopologyKey:       corev1.LabelTopologyZone,
		WhenUnsatisfiable: corev1.ScheduleAnyway,
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app.kubernetes.io/instance": "test"},
		},
	}
}

func TestGenerateVolumesKeepsPreUpgradeOrderWhenNotPersistent(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test", "default", &vectorv1alpha1.VectorAggregatorCommon{}, false)

	volumes := ctrl.generateVectorAggregatorVolume()

	names := make([]string, 0, len(volumes))
	for _, v := range volumes {
		names = append(names, v.Name)
	}
	// The exact order v0.4.1 generated: a reordered (even if semantically equal)
	// pod template rolls every aggregator Deployment on operator upgrade.
	g.Expect(names).To(Equal([]string{"config", "data", "procfs", "sysfs"}))
}

func TestDeploymentTopologySpreadConstraints(t *testing.T) {
	g := NewWithT(t)

	constraint := zoneSpreadConstraint()
	ctrl := createTestController("test", "default", &vectorv1alpha1.VectorAggregatorCommon{
		Replicas:                  ptr.To(int32(3)),
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{constraint},
	}, false)

	deployment := ctrl.createVectorAggregatorDeployment()

	g.Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints).To(Equal([]corev1.TopologySpreadConstraint{constraint}))
}

func TestDeploymentTopologySpreadConstraintsUnset(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test", "default", &vectorv1alpha1.VectorAggregatorCommon{}, false)

	deployment := ctrl.createVectorAggregatorDeployment()

	// An unset field must leave the pod spec untouched, so existing aggregators do
	// not roll on operator upgrade.
	g.Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
}

func TestEventCollectorInheritsTopologySpreadConstraints(t *testing.T) {
	g := NewWithT(t)

	constraint := zoneSpreadConstraint()
	ctrl := createTestController("test", "default", &vectorv1alpha1.VectorAggregatorCommon{
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{constraint},
	}, false)

	deployment := ctrl.createEventCollectorDeployment()

	g.Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints).To(Equal([]corev1.TopologySpreadConstraint{constraint}))
}
