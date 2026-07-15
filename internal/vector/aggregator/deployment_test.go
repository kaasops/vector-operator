package aggregator

import (
	"testing"

	. "github.com/onsi/gomega"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

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
