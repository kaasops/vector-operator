package configcheck

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/utils/hash"
)

// Hash identifies what a config check validates: the config and the env and volumes
// the check pod copies from the workload. A change to any of them needs a new check,
// or the workload rolls out with env or mounts the config no longer resolves against.
func Hash(config []byte, vc *vectorv1alpha1.VectorCommon) (int64, error) {
	b, err := json.Marshal(struct {
		Config       []byte
		Env          []corev1.EnvVar
		EnvFrom      []corev1.EnvFromSource
		Volumes      []corev1.Volume
		VolumeMounts []corev1.VolumeMount
	}{config, vc.Env, vc.EnvFrom, vc.Volumes, vc.VolumeMounts})
	if err != nil {
		return 0, err
	}
	return int64(hash.Get(b)), nil
}
