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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// ExistingSecretAssets is what createOrUpdateVector reads BEFORE building this
// round's config, to compute a safe bridge round via
// config.BridgeAssets/planSecretAssetsBridge - see EnsureVectorAgent's doc comment
// for why the actual union/bridge decision now lives upstream of this package. It
// returns primary and alt SEPARATELY (never merged) - see its own doc comment for why
// a merged view would let planSecretAssetsBridge compute against a fabricated,
// over-budget baseline no real Secret ever has to satisfy.

func TestExistingSecretAssets_ReturnsCurrentData(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"old_key": []byte("old")},
	}
	cl := newFakeClient(g, existing)
	ctrl := NewController(v, cl, nil)
	ctrl.APIReader = ctrl.Client

	primary, alt, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(primary).To(Equal(map[string][]byte{"old_key": []byte("old")}))
	g.Expect(alt).To(BeNil(), "no alt variant when checkpoint migration is off")
}

func TestExistingSecretAssets_EmptyWhenAbsent(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	cl := newFakeClient(g)
	ctrl := NewController(v, cl, nil)
	ctrl.APIReader = ctrl.Client

	primary, alt, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(primary).To(BeEmpty())
	g.Expect(alt).To(BeNil())
}

// During checkpoint migration, either config Secret variant might still be mounted
// by a live pod, so both variants' current data must be read - not just the active
// one, and never merged together (each has its own, independent size budget - see
// planSecretAssetsBridge's doc comment).
func TestExistingSecretAssets_MigrationOnReturnsBothVariantsSeparately(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	active := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-opt-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"active_only": []byte("a")},
	}
	standby := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"standby_only": []byte("b")},
	}
	cl := newFakeClient(g, active, standby)
	ctrl := NewController(v, cl, nil)
	ctrl.APIReader = ctrl.Client
	ctrl.CheckpointMigration = true
	ctrl.OptimizeSources = true

	primary, alt, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(primary).To(Equal(map[string][]byte{"active_only": []byte("a")}))
	g.Expect(alt).To(Equal(map[string][]byte{"standby_only": []byte("b")}))
}

func TestExistingSecretAssets_MigrationOnToleratesOneVariantMissing(t *testing.T) {
	g := NewWithT(t)

	v := &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
	active := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-opt-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"active_only": []byte("a")},
	}
	cl := newFakeClient(g, active) // standby variant does not exist yet
	ctrl := NewController(v, cl, nil)
	ctrl.APIReader = ctrl.Client
	ctrl.CheckpointMigration = true
	ctrl.OptimizeSources = true

	primary, alt, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(primary).To(Equal(map[string][]byte{"active_only": []byte("a")}))
	g.Expect(alt).To(BeEmpty())
}
