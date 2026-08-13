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

package vectoragent

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func newFakeClient(g *WithT, objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())
	return crfake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func TestEnsureVectorAgentSecretAssets_CreatesWhenNonEmpty(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	cl := newFakeClient(g)
	ctrl := NewController(v, cl, nil)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	secret := &corev1.Secret{}
	g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: ctrl.getSecretAssetsName(), Namespace: "vector"}, secret)).To(Succeed())
	g.Expect(secret.Data).To(Equal(ctrl.SecretAssets))
}

func TestEnsureVectorAgentSecretAssets_DeletesWhenEmpty(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"}}
	cl := newFakeClient(g, existing)
	ctrl := NewController(v, cl, nil)

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	err := cl.Get(context.Background(), types.NamespacedName{Name: "test-agent-secret-assets", Namespace: "vector"}, &corev1.Secret{})
	g.Expect(api_errors.IsNotFound(err)).To(BeTrue(), "leftover secret-assets Secret must be deleted once empty")
}

func TestEnsureVectorAgentSecretAssets_NoopWhenAbsentAndEmpty(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	cl := newFakeClient(g)
	ctrl := NewController(v, cl, nil)

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())
}

// While checkpoint migration is on, both config Secret variants are maintained for
// pods not yet rolled after a mode switch; the assets Secret must mirror that, or
// the old-name generation loses secret rotations (and remounts) mid-rollout.
func TestEnsureVectorAgentSecretAssets_MigrationOnMaintainsBothVariants(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	stale := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"foo_bar": []byte("outdated")},
	}
	cl := newFakeClient(g, stale)
	ctrl := NewController(v, cl, nil)
	ctrl.CheckpointMigration = true
	ctrl.OptimizeSources = true
	// Simulates a round where the alt config build succeeded - see
	// ensureVectorAgentSecretAssets' doc comment for why the alt assets are only
	// touched when this is set.
	ctrl.AltByteConfig = []byte("alt-config")
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("current")}

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	for _, name := range []string{"test-agent-secret-assets", "test-agent-opt-secret-assets"} {
		secret := &corev1.Secret{}
		g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "vector"}, secret)).To(Succeed())
		g.Expect(secret.Data).To(Equal(ctrl.SecretAssets), "%s must carry the current data during migration", name)
	}
}

func TestEnsureVectorAgentSecretAssets_MigrationOnRotationUpdatesBothVariants(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	cl := newFakeClient(g)
	ctrl := NewController(v, cl, nil)
	ctrl.CheckpointMigration = true
	ctrl.AltByteConfig = []byte("alt-config")
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("v1")}

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("v2")}
	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	for _, name := range []string{"test-agent-secret-assets", "test-agent-opt-secret-assets"} {
		secret := &corev1.Secret{}
		g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "vector"}, secret)).To(Succeed())
		g.Expect(secret.Data).To(Equal(ctrl.SecretAssets), "%s must be rotated to the new value, not left stale", name)
	}
}

func TestEnsureVectorAgentSecretAssets_MigrationOffRemovesOptVariant(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	stale := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-agent-opt-secret-assets", Namespace: "vector"}}
	cl := newFakeClient(g, stale)
	ctrl := NewController(v, cl, nil)
	ctrl.SecretAssets = map[string][]byte{"foo_bar": []byte("x")}

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	err := cl.Get(context.Background(), types.NamespacedName{Name: "test-agent-opt-secret-assets", Namespace: "vector"}, &corev1.Secret{})
	g.Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the standby assets Secret must be deleted together with the standby config Secret once migration is off")

	secret := &corev1.Secret{}
	g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: "test-agent-secret-assets", Namespace: "vector"}, secret)).To(Succeed())
	g.Expect(secret.Data).To(Equal(ctrl.SecretAssets))
}

func TestEnsureVectorAgentSecretAssets_MigrationOnEmptyRemovesBothVariants(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	staleActive := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-agent-opt-secret-assets", Namespace: "vector"}}
	staleAlt := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"}}
	cl := newFakeClient(g, staleActive, staleAlt)
	ctrl := NewController(v, cl, nil)
	ctrl.CheckpointMigration = true
	ctrl.OptimizeSources = true
	ctrl.AltByteConfig = []byte("alt-config")

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	for _, name := range []string{"test-agent-secret-assets", "test-agent-opt-secret-assets"} {
		err := cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "vector"}, &corev1.Secret{})
		g.Expect(api_errors.IsNotFound(err)).To(BeTrue(), "%s must be deleted once no pipeline references a secret", name)
	}
}

// Regression guard for a checkpoint-migration/deferred-prune interaction nobody had
// checked: config.BuildAgentConfig for the alt (standby) mode can fail independently
// of the active mode's build - createOrUpdateVector only logs that failure and
// leaves ctrl.AltByteConfig nil, so the alt config Secret on the cluster is left
// exactly as it was from the last round that DID succeed (see
// ensureVectorAgentConfig's own AltByteConfig != nil guard). If the active variant's
// assets prune down to a narrower target this round regardless, the alt variant's
// assets would be pruned to the SAME (narrower) content even though the alt config
// Secret - untouched, stale - still references the wider, OLDER set of keys: a pod
// still running that stale alt config would permanently lose access to a key its
// config needs, with nothing ever triggering a fresh reconcile to fix it (the stale
// config Secret doesn't change, so it never re-triggers anything on its own).
func TestEnsureVectorAgentSecretAssets_AltBuildFailureLeavesAltAssetsUntouched(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	// With CheckpointMigration+OptimizeSources both true, the ACTIVE name is
	// "...-opt" (getConfigSecretName) and the ALT (standby) name is the bare one
	// (getAltConfigSecretName) - see getConfigSecretName's doc comment. The alt
	// variant's assets still hold a key (from an earlier, successful round) that the
	// active variant's target no longer needs.
	staleAlt := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"old_and_new": []byte("v"), "alt_only": []byte("still-needed-by-stale-alt-config")},
	}
	cl := newFakeClient(g, staleAlt)
	ctrl := NewController(v, cl, nil)
	ctrl.CheckpointMigration = true
	ctrl.OptimizeSources = true
	// AltByteConfig is deliberately left nil - simulates this round's alt build
	// having failed (config.BuildAgentConfig returned an error, only logged).
	ctrl.AltByteConfig = nil
	// The active variant's target this round no longer needs alt_only.
	ctrl.SecretAssets = map[string][]byte{"old_and_new": []byte("v")}

	g.Expect(ctrl.ensureVectorAgentSecretAssets(context.Background())).To(Succeed())

	activeSecret := &corev1.Secret{}
	g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: "test-agent-opt-secret-assets", Namespace: "vector"}, activeSecret)).To(Succeed())
	g.Expect(activeSecret.Data).To(Equal(ctrl.SecretAssets), "the active variant must still be synced to this round's target")

	altSecret := &corev1.Secret{}
	g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: "test-agent-secret-assets", Namespace: "vector"}, altSecret)).To(Succeed())
	g.Expect(altSecret.Data).To(Equal(staleAlt.Data),
		"the alt variant must be left completely untouched - its config Secret was not updated this round, so pruning "+
			"its assets to the active variant's narrower target would starve a key the stale alt config still references")
}
