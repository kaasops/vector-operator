package aggregator

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func persistentSpec() *vectorv1alpha1.VectorAggregatorCommon {
	storageClass := "gp3"
	return &vectorv1alpha1.VectorAggregatorCommon{
		Replicas: ptr.To(int32(2)),
		Persistence: vectorv1alpha1.VectorAggregatorPersistence{
			Enabled:          true,
			Size:             resource.MustParse("20Gi"),
			StorageClassName: &storageClass,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			RetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			},
		},
	}
}

func TestCreateVectorAggregatorStatefulSet(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test", "default", persistentSpec(), false)
	sts := ctrl.createVectorAggregatorStatefulSet()

	g.Expect(sts).NotTo(BeNil())
	g.Expect(sts.Spec.ServiceName).To(Equal("test-aggregator-headless"), "should reference the headless service")
	g.Expect(sts.Spec.PodManagementPolicy).To(Equal(appsv1.ParallelPodManagement))
	g.Expect(sts.Spec.PersistentVolumeClaimRetentionPolicy).NotTo(BeNil())
	g.Expect(sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted).To(Equal(appsv1.RetainPersistentVolumeClaimRetentionPolicyType))
	g.Expect(sts.Spec.Template.Spec.Containers).NotTo(BeEmpty())

	// Replicas are set because autoscaling is disabled.
	g.Expect(sts.Spec.Replicas).NotTo(BeNil())
	g.Expect(*sts.Spec.Replicas).To(Equal(int32(2)))

	// Exactly one "data" volume claim template, built from the persistence fields.
	g.Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
	vct := sts.Spec.VolumeClaimTemplates[0]
	g.Expect(vct.Name).To(Equal("data"))
	g.Expect(vct.Spec.AccessModes).To(Equal([]corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}))
	g.Expect(vct.Spec.StorageClassName).To(Equal(ptr.To("gp3")))
	size := vct.Spec.Resources.Requests[corev1.ResourceStorage]
	g.Expect(size.Cmp(resource.MustParse("20Gi"))).To(Equal(0))
}

func TestStatefulSetReplicasNilWhenAutoscaling(t *testing.T) {
	g := NewWithT(t)

	spec := persistentSpec()
	spec.Autoscaling.Enabled = true
	ctrl := createTestController("test", "default", spec, false)

	sts := ctrl.createVectorAggregatorStatefulSet()
	g.Expect(sts.Spec.Replicas).To(BeNil(), "replicas must be nil so the operator does not fight the HPA")
}

func TestVolumeClaimTemplatesEscapeHatch(t *testing.T) {
	g := NewWithT(t)

	spec := persistentSpec()
	raw := []corev1.PersistentVolumeClaim{
		{Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "custom"}},
	}
	spec.Persistence.VolumeClaimTemplates = raw
	ctrl := createTestController("test", "default", spec, false)

	g.Expect(ctrl.volumeClaimTemplates()).To(Equal(raw), "raw templates take precedence over the convenience fields")
}

func TestDataVolumeOnlyForDeploymentPath(t *testing.T) {
	g := NewWithT(t)

	hasData := func(volumes []corev1.Volume) bool {
		for _, v := range volumes {
			if v.Name == "data" {
				return true
			}
		}
		return false
	}

	// Persistent mode: the volume claim template provides "data", so the pod
	// template must not also declare a hostPath "data" volume.
	pctrl := createTestController("test", "default", persistentSpec(), false)
	g.Expect(hasData(pctrl.generateVectorAggregatorVolume())).To(BeFalse(), "persistent pod template should not carry a hostPath data volume")

	// Deployment mode: the hostPath "data" volume is still added.
	dctrl := createTestController("test", "default", &vectorv1alpha1.VectorAggregatorCommon{}, false)
	dctrl.Spec.DataDir = "/var/lib/vector"
	g.Expect(hasData(dctrl.generateVectorAggregatorVolume())).To(BeTrue(), "deployment pod template should carry the hostPath data volume")
}
