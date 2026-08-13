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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

func newFakeClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.VectorPipeline{}, &v1alpha1.ClusterVectorPipeline{}).
		WithObjects(objs...).
		Build()
}

// collidingVPs builds the same "team"+"a-x" / "team-a"+"x" flat-key ambiguity used in
// internal/config's collision tests, with an explicit CreationTimestamp gap so
// attribution ("oldest survives") is unambiguous.
func collidingVPs(t0 time.Time) (older, younger *v1alpha1.VectorPipeline) {
	newVP := func(ns, name string, created time.Time) *v1alpha1.VectorPipeline {
		return &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(created),
			},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"auth": {"user": "SECRET[es.username]"}, "inputs": ["logs"], "type": "elasticsearch"}}`,
				)},
			},
		}
	}
	return newVP("team", "a-x", t0), newVP("team-a", "x", t0.Add(time.Hour))
}

func staticGetter(secrets map[string]*corev1.Secret) func(context.Context, string, string) (*corev1.Secret, error) {
	return func(_ context.Context, namespace, name string) (*corev1.Secret, error) {
		return secrets[namespace+"/"+name], nil
	}
}

func agentFilter() pipeline.FilterPipelines {
	return pipeline.FilterPipelines{Scope: pipeline.AllPipelines, Role: v1alpha1.VectorPipelineRoleAgent}
}

// A pipeline edited while collision-failed must NOT be reinstated
// on the strength of its stale, pre-edit valid status - see resolveWorkloadPipelines'
// doc comment on the IsPipelineChanged guard. Unguarded, this reinstates a pipeline
// whose CURRENT spec was never actually validated as "valid" with RelatedSecretsHash
// nil, and pipeline_controller.go's own "no changes" skip (comparing against the stale
// LastAppliedPipelineHash the collision-marking write left behind) can then hide the
// broken edit from ever being properly reconciled - the pipeline stays green forever,
// the workload stays failed forever.
func TestResolveWorkloadPipelinesDoesNotReinstateAnEditedVictim(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older, younger := collidingVPs(t0)
	older.Status.ConfigCheckResult = boolPtr(true)
	older.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	younger.Status.ConfigCheckResult = boolPtr(true)
	younger.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)

	c := newFakeClient(older, younger)
	getter := staticGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	// First pass: the collision fails the younger pipeline.
	result, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "a-x", result[0].GetName())

	gotYounger := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	require.False(t, gotYounger.IsValid())
	require.True(t, len(*gotYounger.Status.Reason) > 0)

	// The collision is resolved (older deleted) AND the younger pipeline's spec is
	// edited before its own reconcile has run - the edit is what must gate
	// reinstatement.
	require.NoError(t, c.Delete(context.Background(), older))
	gotYounger.Spec.Sinks = &runtime.RawExtension{Raw: []byte(
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.password]"}}}`,
	)}
	require.NoError(t, c.Update(context.Background(), gotYounger))

	// Second pass: must NOT reinstate - the edited spec was never actually validated.
	result, _, err = resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)
	assert.Empty(t, result, "the edited victim must stay excluded until its own reconcile validates the new spec")

	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.False(t, gotYounger.IsValid(), "status must be left untouched, not flipped to valid")
	assert.NotNil(t, gotYounger.Status.Reason)
}

// Symmetric control: if the retry candidate's spec is UNCHANGED, it must still be
// reinstated once the collision is gone - this is the behavior
// TestResolveWorkloadPipelinesDoesNotReinstateAnEditedVictim above must not break.
// It must also prove: resolveWorkloadPipelines alone must NOT write the reinstated
// status - only reinstatePipelines, called after a successful build, may do that.
func TestResolveWorkloadPipelinesReinstatesAnUnchangedVictim(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older, younger := collidingVPs(t0)
	older.Status.ConfigCheckResult = boolPtr(true)
	older.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	younger.Status.ConfigCheckResult = boolPtr(true)
	younger.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)

	c := newFakeClient(older, younger)
	getter := staticGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	_, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)

	require.NoError(t, c.Delete(context.Background(), older))

	result, reinstateCandidates, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "x", result[0].GetName())
	require.Len(t, reinstateCandidates, 1, "the unchanged victim must come back as a reinstate candidate")
	assert.Equal(t, "x", reinstateCandidates[0].GetName())

	// resolveWorkloadPipelines alone must not have written anything yet - the status
	// finalization is deferred to reinstatePipelines, called only once the caller's
	// build has actually succeeded.
	gotYounger := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.False(t, gotYounger.IsValid(), "resolveWorkloadPipelines must not finalize status on its own")
	assert.NotNil(t, gotYounger.Status.Reason, "the failed status must be untouched until reinstatePipelines runs")

	require.NoError(t, reinstatePipelines(context.Background(), c, reinstateCandidates))

	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.True(t, gotYounger.IsValid())
	assert.Nil(t, gotYounger.Status.Reason)
}

// A broken pool member must not make resolveWorkloadPipelines
// reinstate an unrelated, already collision-failed pipeline on the strength of an
// aborted scan - DetectSecretCollisions could not vouch for anything this round.
func TestResolveWorkloadPipelinesDoesNotReinstateOnAbortedDetection(t *testing.T) {
	valid := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "valid", Namespace: "ns-ok", CreationTimestamp: metav1.NewTime(time.Now())},
		Spec: v1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
			Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["logs"]}}`)},
		},
		Status: v1alpha1.VectorPipelineStatus{ConfigCheckResult: boolPtr(true), Role: rolePtr(v1alpha1.VectorPipelineRoleAgent)},
	}
	// Individually flagged valid (simulating "validated before"), but its spec's
	// secret backend has an explicit Namespace, which is forbidden on a VectorPipeline
	// - processPipelineSecrets aborts on it inside DetectSecretCollisions.
	broken := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "ns-broken", CreationTimestamp: metav1.NewTime(time.Now())},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "not-allowed"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"logs2": {"type": "kubernetes_logs"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out2": {"type": "elasticsearch", "inputs": ["logs2"], "auth": {"user": "SECRET[es.username]"}}}`,
			)},
		},
		Status: v1alpha1.VectorPipelineStatus{ConfigCheckResult: boolPtr(true), Role: rolePtr(v1alpha1.VectorPipelineRoleAgent)},
	}
	reason := secretCollisionReasonPrefix + "flat key \"x\" collides with pipeline other/pipeline on Vector default/v"
	existingVictim := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "victim", Namespace: "ns-victim", CreationTimestamp: metav1.NewTime(time.Now())},
		Spec: v1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(`{"logs3": {"type": "kubernetes_logs"}}`)},
			Sinks:   &runtime.RawExtension{Raw: []byte(`{"out3": {"type": "blackhole", "inputs": ["logs3"]}}`)},
		},
		Status: v1alpha1.VectorPipelineStatus{
			ConfigCheckResult: boolPtr(false),
			Role:              rolePtr(v1alpha1.VectorPipelineRoleAgent),
			Reason:            &reason,
		},
	}

	c := newFakeClient(valid, broken, existingVictim)
	getter := staticGetter(nil)

	result, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err, "an aborted scan must not surface as a hard error from resolveWorkloadPipelines itself")

	names := make([]string, 0, len(result))
	for _, p := range result {
		names = append(names, p.GetName())
	}
	assert.ElementsMatch(t, []string{"valid", "broken"}, names,
		"falls back to the individually-valid subset, same as GetValidPipelines before collision attribution existed")

	gotVictim := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(existingVictim), gotVictim))
	assert.False(t, gotVictim.IsValid(), "must stay excluded/failed - the aborted scan could not vouch for it")
	assert.Equal(t, reason, *gotVictim.Status.Reason)
}

func boolPtr(b bool) *bool                                               { return &b }
func rolePtr(r v1alpha1.VectorPipelineRole) *v1alpha1.VectorPipelineRole { return &r }
