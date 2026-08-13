/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8s

import (
	corev1 "k8s.io/api/core/v1"
)

// SecretAssetsVolumeName is the reserved name of the operator-owned volume that
// projects the aggregated pipeline-secrets assets Secret into the vector container.
// It carries logic, not just cosmetics - the write-order gate
// (HasOperatorSecretAssetsMount), the workload pod-template generators and the
// configcheck pod all have to agree on it byte for byte, so it is named once here
// rather than spelled out at each site.
//
// It lives in this package, next to the authoritative-volume helpers, because
// internal/config already imports internal/utils/k8s (so the reverse direction is
// not available) - config.SecretsMountPath, its counterpart, is the mount path side
// of the same pair.
const SecretAssetsVolumeName = "secret-assets"

// SetAuthoritativeVolume returns volumes with v present exactly once: any
// user-supplied volume sharing v's name is replaced, not honored. Used for
// operator-owned volumes (the pipeline-secrets assets mount) where a same-named user
// volume would silently shadow the operator's source while the generated config still
// points at the operator's mount path.
func SetAuthoritativeVolume(volumes []corev1.Volume, v corev1.Volume) []corev1.Volume {
	out := make([]corev1.Volume, 0, len(volumes)+1)
	for _, existing := range volumes {
		if existing.Name != v.Name {
			out = append(out, existing)
		}
	}
	return append(out, v)
}

// SetAuthoritativeVolumeMount returns mounts with m present exactly once: any
// user-supplied mount sharing m's name or mount path is replaced, not honored.
func SetAuthoritativeVolumeMount(mounts []corev1.VolumeMount, m corev1.VolumeMount) []corev1.VolumeMount {
	out := make([]corev1.VolumeMount, 0, len(mounts)+1)
	for _, existing := range mounts {
		if existing.Name != m.Name && existing.MountPath != m.MountPath {
			out = append(out, existing)
		}
	}
	return append(out, m)
}

// HasOperatorSecretAssetsMount reports whether spec already carries the OPERATOR'S
// OWN secret-assets mount - a volume named "secret-assets" whose source is exactly
// the Secret secretName, AND a container VolumeMount of that same volume at
// mountPath - not just a volume that happens to be named "secret-assets".
//
// That distinction is load-bearing, not pedantic: SetAuthoritativeVolume only ever
// replaces a same-named volume once ctrl.SecretAssets is non-empty (see its own doc
// comment and generateVectorAgentVolume/generateVectorAggregatorVolume) - before a
// workload's very first secret reference, ctrl.SecretAssets is empty and the
// operator never touches the volume list at all, so a user's own, unrelated volume
// literally named "secret-assets" (an EmptyDir they happened to pick that name for)
// passes through completely untouched. A caller that treated that as "the mount
// already exists" would skip the exact write ordering (config.SecretsMountPath must
// never be readable before it resolves) that hasSecretAssetsMount exists to enforce
// on the round the real one is first added.
func HasOperatorSecretAssetsMount(spec corev1.PodSpec, secretName, mountPath string) bool {
	hasVolume := false
	for _, v := range spec.Volumes {
		if v.Name == SecretAssetsVolumeName && v.Secret != nil && v.Secret.SecretName == secretName {
			hasVolume = true
			break
		}
	}
	if !hasVolume {
		return false
	}
	for _, c := range spec.Containers {
		for _, m := range c.VolumeMounts {
			if m.Name == SecretAssetsVolumeName && m.MountPath == mountPath {
				return true
			}
		}
	}
	return false
}
