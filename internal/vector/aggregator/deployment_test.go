package aggregator

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
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

// SecretAssets non-empty: the pod template (shared by the Deployment and
// StatefulSet paths) must mount the aggregated secret-assets Secret at
// config.SecretsMountPath.
func TestGenerateVolumesIncludesSecretAssetsWhenPresent(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test", "default", &vectorv1alpha1.VectorAggregatorCommon{}, false)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}

	volumes := ctrl.generateVectorAggregatorVolume()
	var vol *corev1.Volume
	for i := range volumes {
		if volumes[i].Name == "secret-assets" {
			vol = &volumes[i]
		}
	}
	g.Expect(vol).NotTo(BeNil(), "secret-assets volume not found")
	g.Expect(vol.Secret).NotTo(BeNil())
	g.Expect(vol.Secret.SecretName).To(Equal(ctrl.getSecretAssetsName()))

	mounts := ctrl.generateVectorAggregatorVolumeMounts()
	var mount *corev1.VolumeMount
	for i := range mounts {
		if mounts[i].Name == "secret-assets" {
			mount = &mounts[i]
		}
	}
	g.Expect(mount).NotTo(BeNil(), "secret-assets volume mount not found")
	g.Expect(mount.MountPath).To(Equal(config.SecretsMountPath))
}

// SecretAssets empty (the default, zero-churn case): neither the volume nor the
// mount must appear.
func TestGenerateVolumesExcludesSecretAssetsWhenAbsent(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test", "default", &vectorv1alpha1.VectorAggregatorCommon{}, false)

	volumes := ctrl.generateVectorAggregatorVolume()
	names := make([]string, 0, len(volumes))
	for _, v := range volumes {
		names = append(names, v.Name)
	}
	g.Expect(names).NotTo(ContainElement("secret-assets"))

	mounts := ctrl.generateVectorAggregatorVolumeMounts()
	mountNames := make([]string, 0, len(mounts))
	for _, m := range mounts {
		mountNames = append(mountNames, m.Name)
	}
	g.Expect(mountNames).NotTo(ContainElement("secret-assets"))
}

// A user-supplied volume/mount squatting the reserved "secret-assets" name must be
// replaced, not honored - same contract as the agent DaemonSet.
func TestDeploymentSecretAssetsVolumeIsAuthoritative(t *testing.T) {
	g := NewWithT(t)

	common := &vectorv1alpha1.VectorAggregatorCommon{
		VectorCommon: vectorv1alpha1.VectorCommon{
			Volumes: []corev1.Volume{
				{Name: "secret-assets", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "secret-assets", MountPath: "/somewhere/else"},
			},
		},
	}
	ctrl := createTestController("test", "default", common, false)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}

	var vols []corev1.Volume
	for _, v := range ctrl.generateVectorAggregatorVolume() {
		if v.Name == "secret-assets" {
			vols = append(vols, v)
		}
	}
	g.Expect(vols).To(HaveLen(1))
	g.Expect(vols[0].Secret).NotTo(BeNil())
	g.Expect(vols[0].Secret.SecretName).To(Equal(ctrl.getSecretAssetsName()))

	var mounts []corev1.VolumeMount
	for _, m := range ctrl.generateVectorAggregatorVolumeMounts() {
		if m.Name == "secret-assets" {
			mounts = append(mounts, m)
		}
	}
	g.Expect(mounts).To(HaveLen(1))
	g.Expect(mounts[0].MountPath).To(Equal(config.SecretsMountPath))
}

// A user-supplied mount occupying the reserved secrets path under a different
// volume name must be replaced: Kubernetes rejects duplicate container mountPaths.
func TestDeploymentSecretAssetsMountPathIsAuthoritative(t *testing.T) {
	g := NewWithT(t)

	common := &vectorv1alpha1.VectorAggregatorCommon{
		VectorCommon: vectorv1alpha1.VectorCommon{
			Volumes: []corev1.Volume{
				{Name: "user-shadow", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "user-shadow", MountPath: config.SecretsMountPath},
			},
		},
	}
	ctrl := createTestController("test", "default", common, false)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}

	var mounts []corev1.VolumeMount
	for _, m := range ctrl.generateVectorAggregatorVolumeMounts() {
		if m.MountPath == config.SecretsMountPath {
			mounts = append(mounts, m)
		}
	}
	g.Expect(mounts).To(HaveLen(1))
	g.Expect(mounts[0].Name).To(Equal("secret-assets"))
}
