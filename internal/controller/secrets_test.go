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

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

func newFakeReader(objs ...client.Object) client.Reader {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// sinkUsing returns a Sinks raw extension whose single sink actually references the
// given alias/key, so config.UsedSecretBackends reports the alias as used.
func sinkUsing(alias, key string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(
		`{"out": {"type": "elasticsearch", "inputs": ["src"], "auth": {"user": "SECRET[` + alias + `.` + key + `]"}}}`,
	)}
}

// resolveForTest mirrors the production call sequence: derive the used-alias set from
// the spec, then resolve with it.
func resolveForTest(t *testing.T, reader client.Reader, p pipeline.Pipeline) ([]types.NamespacedName, *int64, error) {
	t.Helper()
	used, err := config.UsedSecretBackends(p)
	require.NoError(t, err)
	return resolveRelatedSecrets(context.Background(), reader, p, used)
}

func TestResolveRelatedSecretsNoDeclaredBackends(t *testing.T) {
	vp := &v1alpha1.VectorPipeline{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	refs, token, err := resolveForTest(t, newFakeReader(), vp)
	require.NoError(t, err)
	require.Nil(t, refs)
	require.Nil(t, token)
}

// A declared backend nothing references is never read: no API call, no index entry,
// no token contribution, and - the regression this pins - a missing unused Secret can
// no longer fail the pipeline.
func TestResolveRelatedSecretsIgnoresDeclaredButUnusedBackend(t *testing.T) {
	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"unused": {Type: "kubernetes_secret", Name: "missing-and-never-read"},
			},
			Sinks: &runtime.RawExtension{Raw: []byte(`{"out": {"type": "console", "inputs": ["src"]}}`)},
		},
	}

	refs, token, err := resolveForTest(t, newFakeReader(), vp)
	require.NoError(t, err, "an unused backend must not be resolved, so its absence cannot fail the pipeline")
	require.Nil(t, refs)
	require.Nil(t, token)
}

func TestResolveRelatedSecretsVPUsesOwnNamespace(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "creds", UID: "uid-1", ResourceVersion: "1"},
		Data:       map[string][]byte{"user": []byte("alice")},
	}
	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds"},
			},
			Sinks: sinkUsing("es", "user"),
		},
	}

	refs, token, err := resolveForTest(t, newFakeReader(secret), vp)
	require.NoError(t, err)
	require.Equal(t, []types.NamespacedName{{Namespace: "ns", Name: "creds"}}, refs)
	require.NotNil(t, token)
}

func TestResolveRelatedSecretsCVPUsesBackendNamespace(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "other-ns", Name: "creds", UID: "uid-1", ResourceVersion: "1"},
		Data:       map[string][]byte{"token": []byte("tok")},
	}
	cvp := &v1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "cvp"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "other-ns"},
			},
			Sinks: sinkUsing("es", "token"),
		},
	}

	refs, token, err := resolveForTest(t, newFakeReader(secret), cvp)
	require.NoError(t, err)
	require.Equal(t, []types.NamespacedName{{Namespace: "other-ns", Name: "creds"}}, refs)
	require.NotNil(t, token)
}

func TestResolveRelatedSecretsMissingSecretReturnsRefsAndError(t *testing.T) {
	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "missing"},
			},
			Sinks: sinkUsing("es", "user"),
		},
	}

	refs, token, err := resolveForTest(t, newFakeReader(), vp)
	require.Error(t, err)
	require.Nil(t, token)
	// refs must still be fully populated on error, so the caller can keep the
	// SecretIndex accurate for a pipeline that references a not-yet-created secret.
	require.Equal(t, []types.NamespacedName{{Namespace: "ns", Name: "missing"}}, refs)
}

// Shape validation still covers declared-but-unused backends: it is a spec-level
// error, free to check, and matches the config-build rule.
func TestResolveRelatedSecretsShapeValidatedEvenWhenUnused(t *testing.T) {
	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"unused": {Type: "kubernetes_secret", Name: "creds", Namespace: "elsewhere"},
			},
			Sinks: &runtime.RawExtension{Raw: []byte(`{"out": {"type": "console", "inputs": ["src"]}}`)},
		},
	}

	_, _, err := resolveForTest(t, newFakeReader(), vp)
	require.Error(t, err)
	var shapeErr *invalidSecretShapeError
	require.ErrorAs(t, err, &shapeErr)
}

// An existing-but-empty-Data secret used to hash to nil (indistinguishable from "no
// secret referenced"), leaving a recovery-skip boundary. The identity token closes
// it: any existing secret has a UID/resourceVersion, so the token is non-nil and a
// delete-then-recreate (new UID) always reads as a change.
func TestResolveRelatedSecretsEmptyDataStillTokenized(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "creds", UID: "uid-1", ResourceVersion: "1"},
		// Data intentionally nil/empty.
	}
	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds"},
			},
			Sinks: sinkUsing("es", "user"),
		},
	}

	refs, token, err := resolveForTest(t, newFakeReader(secret), vp)
	require.NoError(t, err, "an empty-Data secret is not a resolve failure - it exists and is readable")
	require.Equal(t, []types.NamespacedName{{Namespace: "ns", Name: "creds"}}, refs)
	require.NotNil(t, token, "an existing secret must tokenize non-nil regardless of its data")

	recreated := secret.DeepCopy()
	recreated.UID = "uid-2"
	recreated.ResourceVersion = "7"
	_, token2, err := resolveForTest(t, newFakeReader(recreated), vp)
	require.NoError(t, err)
	require.False(t, relatedSecretsHashEqual(token, token2),
		"delete-then-recreate of an empty secret must read as a change (new UID)")
}

func TestResolveRelatedSecretsNilReaderErrors(t *testing.T) {
	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds"},
			},
			Sinks: sinkUsing("es", "user"),
		},
	}

	refs, token, err := resolveRelatedSecrets(context.Background(), nil, vp, map[string]struct{}{"es": {}})
	require.Error(t, err)
	require.Nil(t, refs)
	require.Nil(t, token)
}

func TestRelatedSecretsTokenNilForNoIdentities(t *testing.T) {
	require.Nil(t, relatedSecretsToken(nil))
	require.Nil(t, relatedSecretsToken([]corev1.ObjectReference{}))
}

func TestRelatedSecretsTokenDeterministic(t *testing.T) {
	ids := []corev1.ObjectReference{{Namespace: "ns", Name: "a", UID: "u1", ResourceVersion: "1"}}
	h1 := relatedSecretsToken(ids)
	h2 := relatedSecretsToken(ids)
	require.NotNil(t, h1)
	require.Equal(t, *h1, *h2)
}

// The token is a pure function of secret identity, never of secret data - the
// security property that kills the status fingerprint oracle: no bytes of the secret
// value influence the published hash.
func TestRelatedSecretsTokenIndependentOfData(t *testing.T) {
	base := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "creds", UID: "u1", ResourceVersion: "1"},
		Data:       map[string][]byte{"user": []byte("low-entropy-1")},
	}
	other := base.DeepCopy()
	other.Data = map[string][]byte{"user": []byte("low-entropy-2")}

	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds"},
			},
			Sinks: sinkUsing("es", "user"),
		},
	}

	_, t1, err := resolveForTest(t, newFakeReader(base), vp)
	require.NoError(t, err)
	_, t2, err := resolveForTest(t, newFakeReader(other), vp)
	require.NoError(t, err)
	require.True(t, relatedSecretsHashEqual(t1, t2),
		"same identity+resourceVersion must produce the same token no matter the data - the status must not fingerprint values")
}

func TestRelatedSecretsTokenChangesWithResourceVersion(t *testing.T) {
	h1 := relatedSecretsToken([]corev1.ObjectReference{{Namespace: "ns", Name: "a", UID: "u1", ResourceVersion: "1"}})
	h2 := relatedSecretsToken([]corev1.ObjectReference{{Namespace: "ns", Name: "a", UID: "u1", ResourceVersion: "2"}})
	require.NotEqual(t, *h1, *h2, "any data change bumps resourceVersion, and the token must follow")
}

// Length-prefixed framing regression: without it, field concatenation lets two
// different identity sets serialize identically (the exact bug class that silently
// swallowed rotations in the data-based predecessor: "x\nk=v" as one value framed the
// same as two entries).
func TestRelatedSecretsTokenFramingInjection(t *testing.T) {
	h1 := relatedSecretsToken([]corev1.ObjectReference{{Namespace: "a", Name: "b-c", UID: "u", ResourceVersion: "1"}})
	h2 := relatedSecretsToken([]corev1.ObjectReference{{Namespace: "a-b", Name: "c", UID: "u", ResourceVersion: "1"}})
	require.NotEqual(t, *h1, *h2, "field boundaries must be part of the hashed input")

	h3 := relatedSecretsToken([]corev1.ObjectReference{
		{Namespace: "ns", Name: "a", UID: "u1", ResourceVersion: "1"},
		{Namespace: "ns", Name: "b", UID: "u2", ResourceVersion: "2"},
	})
	h4 := relatedSecretsToken([]corev1.ObjectReference{
		{Namespace: "ns", Name: "a", UID: "u1", ResourceVersion: "1ns"},
		{Namespace: "", Name: "b", UID: "u2", ResourceVersion: "2"},
	})
	require.NotEqual(t, *h3, *h4, "entry boundaries must be part of the hashed input")
}

// TestMapSecretToPipelinesUnknownSecretReturnsNil covers the degenerate half of the
// Secret-watch envtest (pipeline_controller_secret_watch_test.go), which only proves
// "no reconcile happens" indirectly via RelatedSecretsHash staying put - a signal that
// doesn't actually depend on requeue behavior, so it can't fail on a regression here.
// This asserts mapSecretToPipelines' own return value directly for a Secret no pipeline
// declared - the overwhelming common case in a real cluster.
func TestMapSecretToPipelinesUnknownSecretReturnsNil(t *testing.T) {
	r := &PipelineReconciler{SecretIndex: pipeline.NewSecretIndex()}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "unrelated"}}

	require.Nil(t, r.mapSecretToPipelines(context.Background(), secret))
}

func TestMapSecretToPipelinesKnownSecretReturnsRequests(t *testing.T) {
	si := pipeline.NewSecretIndex()
	p1 := types.NamespacedName{Namespace: "ns", Name: "p1"}
	p2 := types.NamespacedName{Namespace: "ns", Name: "p2"}
	sec := types.NamespacedName{Namespace: "ns", Name: "creds"}
	si.Set(p1, []types.NamespacedName{sec})
	si.Set(p2, []types.NamespacedName{sec})

	r := &PipelineReconciler{SecretIndex: si}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "creds"}}

	reqs := r.mapSecretToPipelines(context.Background(), secret)
	got := make([]types.NamespacedName, len(reqs))
	for i, req := range reqs {
		got[i] = req.NamespacedName
	}
	require.ElementsMatch(t, []types.NamespacedName{p1, p2}, got)
}

func TestRelatedSecretsHashEqual(t *testing.T) {
	var a, b int64 = 5, 5
	var c int64 = 6
	require.True(t, relatedSecretsHashEqual(nil, nil))
	require.False(t, relatedSecretsHashEqual(&a, nil))
	require.False(t, relatedSecretsHashEqual(nil, &c))
	require.True(t, relatedSecretsHashEqual(&a, &b))
	require.False(t, relatedSecretsHashEqual(&a, &c))
}

// Regression guard: the SecretIndex must be populated even when the reconcile bails out
// early because no Vector/aggregator CRs exist yet ("Vectors not found"). Otherwise an
// operator restart in such a cluster leaves the index empty, and once a workload CR
// appears, a secret rotation no longer requeues the pipelines that reference it.
func TestReconcilePopulatesSecretIndexWithoutWorkloads(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "creds", UID: "u1", ResourceVersion: "1"},
		Data:       map[string][]byte{"user": []byte("x")},
	}
	vp := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds"},
			},
			Sinks: sinkUsing("es", "user"),
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, vp).
		WithStatusSubresource(&v1alpha1.VectorPipeline{}).Build()

	r := &PipelineReconciler{
		Client:      cl,
		APIReader:   cl,
		SecretIndex: pipeline.NewSecretIndex(),
	}

	res, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "p"},
	})
	require.NoError(t, err)
	require.Zero(t, res.RequeueAfter)

	reqs := r.mapSecretToPipelines(context.Background(), secret)
	require.Len(t, reqs, 1, "the pipeline must be indexed for its secret even though no workload CR exists")
	require.Equal(t, types.NamespacedName{Namespace: "ns", Name: "p"}, reqs[0].NamespacedName)
}
