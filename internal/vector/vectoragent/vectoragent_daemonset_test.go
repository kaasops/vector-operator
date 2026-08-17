/*
Copyright 2024.

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

package vectoragent

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
)

func testController(checkpointMigration, optimizeSources, compress bool) *Controller {
	v := &vectorv1alpha1.Vector{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"},
		Spec: vectorv1alpha1.VectorSpec{
			Agent: &vectorv1alpha1.VectorAgent{
				VectorCommon: vectorv1alpha1.VectorCommon{
					CompressConfigFile: compress,
				},
			},
		},
	}
	ctrl := NewController(v, nil, nil)
	ctrl.CheckpointMigration = checkpointMigration
	ctrl.OptimizeSources = optimizeSources
	return ctrl
}

func TestConfigSecretNameBoundToMode(t *testing.T) {
	cases := []struct {
		migration, optimize bool
		active, alt         string
	}{
		{false, false, "test-agent", "test-agent-opt"},
		{false, true, "test-agent", "test-agent-opt"},
		{true, false, "test-agent", "test-agent-opt"},
		{true, true, "test-agent-opt", "test-agent"},
	}
	for _, c := range cases {
		ctrl := testController(c.migration, c.optimize, false)
		if got := ctrl.getConfigSecretName(); got != c.active {
			t.Errorf("migration=%v optimize=%v: active secret %q, want %q", c.migration, c.optimize, got, c.active)
		}
		if got := ctrl.getAltConfigSecretName(); got != c.alt {
			t.Errorf("migration=%v optimize=%v: alt secret %q, want %q", c.migration, c.optimize, got, c.alt)
		}
	}
}

func TestDaemonSetVolumeFollowsMode(t *testing.T) {
	ds := testController(true, true, false).createVectorAgentDaemonSet()
	for _, v := range ds.Spec.Template.Spec.Volumes {
		if v.Name == "config" {
			if v.Secret == nil || v.Secret.SecretName != "test-agent-opt" {
				t.Fatalf("config volume secret = %+v, want test-agent-opt", v.VolumeSource)
			}
			return
		}
	}
	t.Fatal("config volume not found")
}

func TestMergerInitContainer(t *testing.T) {
	ds := testController(true, true, false).createVectorAgentDaemonSet()
	inits := ds.Spec.Template.Spec.InitContainers
	if len(inits) != 1 || inits[0].Name != "checkpoint-merger" {
		t.Fatalf("init containers = %v, want single checkpoint-merger", inits)
	}
	if inits[0].Image == "" {
		t.Error("merger image not defaulted")
	}

	// off by default
	ds = testController(false, true, false).createVectorAgentDaemonSet()
	if len(ds.Spec.Template.Spec.InitContainers) != 0 {
		t.Error("init container present without checkpoint migration")
	}
}

// SecretAssets non-empty: the DaemonSet must mount the aggregated secret-assets
// Secret at config.SecretsMountPath, in both the config volume and the vector
// agent container.
func TestDaemonSetSecretAssetsVolumeWhenPresent(t *testing.T) {
	ctrl := testController(false, false, false)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}
	ds := ctrl.createVectorAgentDaemonSet()

	var vol *corev1.Volume
	for i := range ds.Spec.Template.Spec.Volumes {
		if ds.Spec.Template.Spec.Volumes[i].Name == "secret-assets" {
			vol = &ds.Spec.Template.Spec.Volumes[i]
		}
	}
	if vol == nil {
		t.Fatal("secret-assets volume not found")
	}
	if vol.Secret == nil || vol.Secret.SecretName != ctrl.getSecretAssetsName() {
		t.Fatalf("secret-assets volume secret = %+v, want SecretName %q", vol.VolumeSource, ctrl.getSecretAssetsName())
	}

	var mount *corev1.VolumeMount
	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name != ctrl.getNameVectorAgent() {
			continue
		}
		for j := range c.VolumeMounts {
			if c.VolumeMounts[j].Name == "secret-assets" {
				mount = &c.VolumeMounts[j]
			}
		}
	}
	if mount == nil {
		t.Fatal("secret-assets volume mount not found on the vector agent container")
	}
	if mount.MountPath != config.SecretsMountPath {
		t.Fatalf("secret-assets mount path = %q, want %q", mount.MountPath, config.SecretsMountPath)
	}
}

// SecretAssets empty (the default, zero-churn case): neither the volume nor the
// mount must appear, so the pod template of non-users of the feature is unchanged.
func TestDaemonSetNoSecretAssetsVolumeWhenEmpty(t *testing.T) {
	ctrl := testController(false, false, false)
	ds := ctrl.createVectorAgentDaemonSet()

	for _, v := range ds.Spec.Template.Spec.Volumes {
		if v.Name == "secret-assets" {
			t.Fatal("secret-assets volume present despite empty SecretAssets")
		}
	}
	for _, c := range ds.Spec.Template.Spec.Containers {
		for _, vm := range c.VolumeMounts {
			if vm.Name == "secret-assets" {
				t.Fatal("secret-assets volume mount present despite empty SecretAssets")
			}
		}
	}
}

func TestMergerRunsAfterConfigReloader(t *testing.T) {
	ctrl := testController(true, true, true)
	ctrl.Vector.Spec.Agent.ConfigReloaderImage = "config-reloader:test"
	ds := ctrl.createVectorAgentDaemonSet()
	inits := ds.Spec.Template.Spec.InitContainers
	if len(inits) != 2 || inits[0].Name != "init-config-reloader" || inits[1].Name != "checkpoint-merger" {
		names := make([]string, 0, len(inits))
		for _, c := range inits {
			names = append(names, c.Name)
		}
		t.Fatalf("init container order = %v, want [init-config-reloader checkpoint-merger]", names)
	}
}

// A user-supplied volume/mount squatting the reserved "secret-assets" name must be
// replaced, not honored: the generated config reads config.SecretsMountPath, so
// honoring the user's entry would silently point Vector at the wrong source.
func TestDaemonSetSecretAssetsVolumeIsAuthoritative(t *testing.T) {
	ctrl := testController(false, false, false)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}
	ctrl.Vector.Spec.Agent.Volumes = []corev1.Volume{
		{Name: "secret-assets", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	ctrl.Vector.Spec.Agent.VolumeMounts = []corev1.VolumeMount{
		{Name: "secret-assets", MountPath: "/somewhere/else"},
	}

	var vols []corev1.Volume
	for _, v := range ctrl.generateVectorAgentVolume() {
		if v.Name == "secret-assets" {
			vols = append(vols, v)
		}
	}
	if len(vols) != 1 {
		t.Fatalf("want exactly one secret-assets volume, got %d", len(vols))
	}
	if vols[0].Secret == nil || vols[0].Secret.SecretName != ctrl.getSecretAssetsName() {
		t.Fatalf("secret-assets volume must be the operator's Secret source, got %+v", vols[0].VolumeSource)
	}

	var mounts []corev1.VolumeMount
	for _, m := range ctrl.generateVectorAgentVolumeMounts() {
		if m.Name == "secret-assets" {
			mounts = append(mounts, m)
		}
	}
	if len(mounts) != 1 {
		t.Fatalf("want exactly one secret-assets mount, got %d", len(mounts))
	}
	if mounts[0].MountPath != config.SecretsMountPath {
		t.Fatalf("secret-assets mount path = %q, want %q", mounts[0].MountPath, config.SecretsMountPath)
	}
}

// A user-supplied mount occupying the reserved secrets path under a different
// volume name must be replaced: Kubernetes rejects duplicate container mountPaths.
func TestDaemonSetSecretAssetsMountPathIsAuthoritative(t *testing.T) {
	ctrl := testController(false, false, false)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}
	ctrl.Vector.Spec.Agent.Volumes = []corev1.Volume{
		{Name: "user-shadow", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	ctrl.Vector.Spec.Agent.VolumeMounts = []corev1.VolumeMount{
		{Name: "user-shadow", MountPath: config.SecretsMountPath},
	}

	var mounts []corev1.VolumeMount
	for _, m := range ctrl.generateVectorAgentVolumeMounts() {
		if m.MountPath == config.SecretsMountPath {
			mounts = append(mounts, m)
		}
	}
	if len(mounts) != 1 {
		t.Fatalf("want exactly one mount at %q, got %d", config.SecretsMountPath, len(mounts))
	}
	if mounts[0].Name != "secret-assets" {
		t.Fatalf("mount at %q must use the operator's secret-assets volume, got %q", config.SecretsMountPath, mounts[0].Name)
	}
}
