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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
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
