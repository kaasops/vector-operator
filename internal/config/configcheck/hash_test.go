package configcheck

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func TestHashFollowsWhatTheCheckPodCopies(t *testing.T) {
	config := []byte(`{"sources":{}}`)
	base := func() *vectorv1alpha1.VectorCommon {
		return &vectorv1alpha1.VectorCommon{
			Image: "timberio/vector:0.48.0-distroless-libc",
			Env:   []corev1.EnvVar{{Name: "TOKEN", Value: "x"}},
		}
	}
	h := func(c []byte, vc *vectorv1alpha1.VectorCommon) int64 {
		t.Helper()
		v, err := Hash(c, vc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	want := h(config, base())

	if got := h(config, base()); got != want {
		t.Fatalf("same input gave %d and %d", want, got)
	}

	changes := map[string]func(*vectorv1alpha1.VectorCommon){
		"env removed": func(vc *vectorv1alpha1.VectorCommon) { vc.Env = nil },
		"envFrom": func(vc *vectorv1alpha1.VectorCommon) {
			vc.EnvFrom = []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{}}}
		},
		"volumes": func(vc *vectorv1alpha1.VectorCommon) { vc.Volumes = []corev1.Volume{{Name: "certs"}} },
		"volume mounts": func(vc *vectorv1alpha1.VectorCommon) {
			vc.VolumeMounts = []corev1.VolumeMount{{Name: "certs", MountPath: "/certs"}}
		},
	}
	for name, change := range changes {
		vc := base()
		change(vc)
		if h(config, vc) == want {
			t.Errorf("%s: hash did not change", name)
		}
	}
	if h([]byte(`{"sources":{"a":{}}}`), base()) == want {
		t.Error("config: hash did not change")
	}

	vc := base()
	vc.Resources.Limits = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}
	vc.Image = "timberio/vector:0.49.0-distroless-libc"
	if h(config, vc) != want {
		t.Error("resources and image are outside the hash, but changed it")
	}
}
