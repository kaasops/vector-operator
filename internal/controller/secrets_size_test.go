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
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

func bigValue(n int) []byte { return bytes.Repeat([]byte("a"), n) }

// oversizedVPs builds two unrelated (non-colliding) pipelines whose secret values
// individually fit under corev1.MaxSecretSize but together overflow it - the live
// cross-tenant overflow repro.
func oversizedVPs(t0 time.Time) (older, younger *v1alpha1.VectorPipeline) {
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
					`{"out": {"auth": {"user": "SECRET[es.cert]"}, "inputs": ["logs"], "type": "elasticsearch"}}`,
				)},
			},
			Status: v1alpha1.VectorPipelineStatus{ConfigCheckResult: boolPtr(true), Role: rolePtr(v1alpha1.VectorPipelineRoleAgent)},
		}
	}
	return newVP("team-a", "older", t0), newVP("team-b", "younger", t0.Add(time.Hour))
}

func TestResolveWorkloadPipelinesExcludesYoungerOnSizeOverflow(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older, younger := oversizedVPs(t0)

	c := newFakeClient(older, younger)
	getter := staticGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bigValue(600000)}},
		"team-b/creds": {Data: map[string][]byte{"cert": bigValue(600000)}},
	})

	result, reinstate, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "older", result[0].GetName(), "the workload must keep the older pipeline")
	assert.Empty(t, reinstate)

	gotYounger := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.False(t, gotYounger.IsValid(), "the younger pipeline must be excluded and failed")
	require.NotNil(t, gotYounger.Status.Reason)
	assert.Contains(t, *gotYounger.Status.Reason, secretSizeExclusionReasonPrefix)
	assert.Contains(t, *gotYounger.Status.Reason, "1048576", "the reason must name the Kubernetes Secret limit")
	assert.Nil(t, gotYounger.Status.RelatedSecretsHash,
		"clearing it is what lets the pipeline recover once reconsidered - same idiom as the collision path")

	gotOlder := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(older), gotOlder))
	assert.True(t, gotOlder.IsValid(), "the older pipeline must stay valid")
}

// Symmetric recovery test: once the older (large) pipeline is gone, the younger one
// must come back as a reinstate candidate on the very next reconcile, the same
// self-healing property the collision path already has.
func TestResolveWorkloadPipelinesReinstatesSizeExcludedPipelineOnceRoomFrees(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older, younger := oversizedVPs(t0)

	c := newFakeClient(older, younger)
	getter := staticGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bigValue(600000)}},
		"team-b/creds": {Data: map[string][]byte{"cert": bigValue(600000)}},
	})

	_, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)

	require.NoError(t, c.Delete(context.Background(), older))

	result, reinstateCandidates, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "younger", result[0].GetName())
	require.Len(t, reinstateCandidates, 1, "the unchanged, now-fitting pipeline must come back as a reinstate candidate")
	assert.Equal(t, "younger", reinstateCandidates[0].GetName())

	// resolveWorkloadPipelines alone must not finalize status - same sequencing rule
	// as the collision path (see reinstatePipelines' doc comment).
	gotYounger := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.False(t, gotYounger.IsValid())

	require.NoError(t, reinstatePipelines(context.Background(), c, reinstateCandidates))

	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.True(t, gotYounger.IsValid())
	assert.Nil(t, gotYounger.Status.Reason)
}

// Regression guard: a retry candidate whose collision has resolved but whose own
// Secret key has since been deleted must still be handed to the caller (as a build
// candidate AND a tentative reinstate candidate) so Build*Config gets to discover and
// report the real problem itself - exactly the pre-size-feature behavior. Wrongly
// treating the resulting per-pipeline data error from the size pre-pass as a
// structural "detection aborted" would fall back to the individually-valid subset of
// the pool, which drops this still-invalid retry candidate silently: the workload
// build then runs on an empty pipeline list, "succeeds" trivially, and the real
// missing-key problem is never reported anywhere.
func TestResolveWorkloadPipelinesDoesNotDropRetryCandidateOnSizeDataError(t *testing.T) {
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

	// First pass: the collision fails the younger pipeline (same as the collision
	// suite's setup).
	_, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)

	// Resolve the collision, but ALSO break younger's own build by deleting the key
	// its SECRET[] reference needs.
	require.NoError(t, c.Delete(context.Background(), older))
	brokenGetter := staticGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{}}, // "username" key gone
	})

	result, reinstateCandidates, err := resolveWorkloadPipelines(context.Background(), c, brokenGetter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err, "a per-pipeline data error must not surface as a hard error from resolveWorkloadPipelines itself")
	require.Len(t, result, 1, "the retry candidate must still reach the caller so Build*Config can discover and report its own real problem")
	assert.Equal(t, "x", result[0].GetName())
	assert.Len(t, reinstateCandidates, 1, "tentatively reinstated - the caller only finalizes this after Build*Config actually succeeds")
}

// A collision victim must not consume any of the size budget: it never reaches
// Build*Config either way, so folding its bytes into the running total would reject
// pipelines the real merged build would have accepted.
func TestResolveWorkloadPipelinesSizeBudgetExcludesCollisionVictims(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// older and collisionLoser collide on the same flat key (see collidingVPs); if
	// collisionLoser's large value were wrongly folded into the size total anyway,
	// it alone would be enough to push fits over the limit.
	older, collisionLoser := collidingVPs(t0)
	older.Status.ConfigCheckResult = boolPtr(true)
	older.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	collisionLoser.Status.ConfigCheckResult = boolPtr(true)
	collisionLoser.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	fits := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fits", Namespace: "team-c", CreationTimestamp: metav1.NewTime(t0.Add(2 * time.Hour)),
		},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es2": {Type: "kubernetes_secret", Name: "creds2"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"logs3": {"type": "kubernetes_logs"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out3": {"type": "elasticsearch", "inputs": ["logs3"], "auth": {"user": "SECRET[es2.cert]"}}}`,
			)},
		},
		Status: v1alpha1.VectorPipelineStatus{ConfigCheckResult: boolPtr(true), Role: rolePtr(v1alpha1.VectorPipelineRoleAgent)},
	}

	c := newFakeClient(older, collisionLoser, fits)
	getter := staticGetter(map[string]*corev1.Secret{
		"team/creds":    {Data: map[string][]byte{"username": []byte("small")}},
		"team-a/creds":  {Data: map[string][]byte{"username": bigValue(900000)}}, // collisionLoser's own backend
		"team-c/creds2": {Data: map[string][]byte{"cert": bigValue(500000)}},
	})

	result, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)

	names := make([]string, 0, len(result))
	for _, p := range result {
		names = append(names, p.GetName())
	}
	assert.ElementsMatch(t, []string{"a-x", "fits"}, names,
		"fits must not be size-excluded: the collision victim's 900000 bytes must never enter the size budget")

	gotFits := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(fits), gotFits))
	assert.True(t, gotFits.IsValid())
}

// The sticky-priority trap: a younger pipeline B is already resident (its value
// staged in the assets Secret from an earlier round) when an OLDER pipeline A
// appears. resolveWorkloadPipelines' own attribution must not be biased by B's
// residency - A is the correct final winner purely by CreationTimestamp, regardless
// of what happens to already be staged - and planSecretAssetsBridge must not let B's
// mere presence in the assets Secret block A from ever getting in: it holds A back
// only until B's stale data is actually pruned, never as a permanent, residency-based
// override of the age-based decision.
func TestStickyPriorityDoesNotSurviveAnOlderPipelineAppearing(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older, younger := oversizedVPs(t0) // older="team-a/older", younger="team-b/younger"
	// Simulate steady state where only younger was ever known before now: it is
	// already individually valid, as if admitted in an earlier round.
	younger.Status.ConfigCheckResult = boolPtr(true)
	younger.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	// older is brand new this round, also individually valid on its own.
	older.Status.ConfigCheckResult = boolPtr(true)
	older.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)

	c := newFakeClient(older, younger)
	getter := staticGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bigValue(600000)}},
		"team-b/creds": {Data: map[string][]byte{"cert": bigValue(600000)}},
	})

	pipelines, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, pipelines, 1, "the final target must be decided by age alone, unaffected by who is currently staged")
	assert.Equal(t, "older", pipelines[0].GetName(), "A must be the correct winner even though B is the one currently resident")

	gotYounger := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.False(t, gotYounger.IsValid(), "B must lose to the older A per the normal size-attribution rule")

	// younger's value is still staged (unpruned) from the earlier round.
	existing := map[string][]byte{"team_b_younger_es_cert": bigValue(600000)}

	bridgeDataPerVariant, bridgePipelines, waitingPipelines, err := planSecretAssetsBridge(context.Background(), getter, testAssetsPrototype(), pipelines, existing)
	require.NoError(t, err)
	assert.Empty(t, bridgePipelines, "A cannot be staged alongside B's still-present stale data this round")
	require.Len(t, waitingPipelines, 1)
	assert.Same(t, pipelines[0], waitingPipelines[0], "A waits - it is not rejected, and it is not silently swapped out for B")
	require.Len(t, bridgeDataPerVariant, 1)
	assert.Equal(t, existing, bridgeDataPerVariant[0], "nothing already staged is dropped just because A does not fit yet")

	// Once B's stale key is actually pruned (a later round, simulated here as the
	// next planSecretAssetsBridge call against the post-prune state), A must get in -
	// residency never becomes a permanent override of the age-based decision.
	bridgeDataPerVariant2, bridgePipelines2, waitingPipelines2, err := planSecretAssetsBridge(context.Background(), getter, testAssetsPrototype(), pipelines, map[string][]byte{})
	require.NoError(t, err)
	require.Len(t, bridgePipelines2, 1)
	assert.Same(t, pipelines[0], bridgePipelines2[0])
	assert.Empty(t, waitingPipelines2)
	require.Len(t, bridgeDataPerVariant2, 1)
	assert.Equal(t, map[string][]byte{"team_a_older_es_cert": bigValue(600000)}, bridgeDataPerVariant2[0])
}

// Known, pre-existing limitation (already true for flat-key collisions before this
// branch - see docs/secrets.md's collision section - and inherited unchanged by size
// attribution, which makes it far more likely to actually trigger in practice: any
// shared agent operating near the aggregated Secret's ceiling can hit it, not just a
// deliberately crafted name collision).
//
// A pipeline P selected by two workloads can globally OSCILLATE: workload A's pool
// does not have room for P (it loses to another pipeline Q on A), while workload B's
// pool - which does not include Q at all - has room for P on its own.
// resolveWorkloadPipelines' reinstate/exclude decision is written to P's single,
// GLOBAL .status.reason field, so whichever workload reconciles last wins that field
// - there is no per-workload status representation. Nothing in the pipeline's own
// spec changes between A's and B's reconciles, so each one's decision is internally
// correct for its OWN pool; the oscillation is a consequence of one shared status
// field representing a fundamentally per-workload fact, not a bug in either
// decision. This test locks in that documented behavior (see docs/secrets.md) rather
// than asserting it as a defect - a real fix needs a per-workload status
// representation, out of scope here; it is tracked as backlog.
func TestGlobalStatusOscillatesForAPipelineSelectedByTwoWorkloads(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Q and P together overflow on workload A; P alone fits comfortably on workload B.
	q := testVPForOscillation("team-q", "q", t0)
	p := testVPForOscillation("team-p", "p", t0.Add(time.Hour))
	q.Status.ConfigCheckResult = boolPtr(true)
	q.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	p.Status.ConfigCheckResult = boolPtr(true)
	p.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)

	c := newFakeClient(q, p)
	getter := staticGetter(map[string]*corev1.Secret{
		"team-q/creds": {Data: map[string][]byte{"cert": bigValue(700000)}},
		"team-p/creds": {Data: map[string][]byte{"cert": bigValue(700000)}},
	})

	// Workload A selects both Q and P (they do not fit together - Q wins, P loses).
	resultA, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "workload-a", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, resultA, 1)
	assert.Equal(t, "q", resultA[0].GetName())

	gotP := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(p), gotP))
	assert.False(t, gotP.IsValid(), "P loses to Q on workload A's pool")

	// Workload B's selector never matches Q at all (a different team's pipeline), so
	// its pool is just P - simulated here with a separate fake client scoped to
	// exactly what workload B would actually see (the fake client backing A's own
	// pool has no Namespace-based filtering to lean on for Scope:AllPipelines, so a
	// second client is the simplest faithful stand-in for "a different selector").
	cB := newFakeClient(p)
	resultB, _, err := resolveWorkloadPipelines(context.Background(), cB, getter, agentFilter(), "Vector", "default", "workload-b", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, resultB, 1)
	assert.Equal(t, "p", resultB[0].GetName(), "P alone fits comfortably on a pool that does not include Q")

	// Finalize workload B's reinstatement, as createOrUpdateVector would after a
	// successful publish - P becomes globally valid. Both fake clients wrap the same
	// underlying pipeline objects' names/namespaces, but reinstatePipelines must
	// write through the client A's own subsequent Get below reads from, so use c.
	pFromC := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(p), pFromC))
	require.NoError(t, reinstatePipelines(context.Background(), c, []pipeline.Pipeline{pFromC}))
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(p), gotP))
	assert.True(t, gotP.IsValid(), "P is now globally valid, thanks to workload B's reconcile")

	// Workload A reconciles again. P now enters A's pool via the plain IsValid()
	// branch (not as a retry candidate, since its status is currently valid), and
	// A's own pool still does not have room for it alongside Q.
	resultA2, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "workload-a", testAssetsPrototype())
	require.NoError(t, err)
	require.Len(t, resultA2, 1)
	assert.Equal(t, "q", resultA2[0].GetName())

	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(p), gotP))
	assert.False(t, gotP.IsValid(),
		"P flips back to invalid: workload A's next reconcile re-discovers the same conflict with Q and overwrites the "+
			"global status B just set, with nothing in P's own spec ever having changed")
}

// Regression guard: a pipeline whose failure CLASS switches between rounds (here:
// collision -> size exclusion) must get the NEW, accurate reason, not keep
// displaying stale text from the class it no longer fails under - nothing in the
// pipeline's own spec changes to trigger a fresh per-pipeline reconcile that would
// otherwise overwrite it.
func TestAttributionReasonUpdatesWhenFailureClassChanges(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older, younger := collidingVPs(t0) // older="team/a-x", younger="team-a/x", colliding flat keys
	older.Status.ConfigCheckResult = boolPtr(true)
	older.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	younger.Status.ConfigCheckResult = boolPtr(true)
	younger.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)

	c := newFakeClient(older, younger)
	getter := staticGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	// Round 1: younger loses to older on the flat-key collision.
	_, _, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)

	gotYounger := &v1alpha1.VectorPipeline{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	require.False(t, gotYounger.IsValid())
	require.Contains(t, *gotYounger.Status.Reason, secretCollisionReasonPrefix)

	// Round 2: the collision is resolved (older deleted), but a NEW, older, large
	// pipeline appears that younger now loses to on SIZE instead - a completely
	// different failure class, with younger's own spec never touched.
	require.NoError(t, c.Delete(context.Background(), older))
	bigOlder := testVPForOscillation("team-big", "big", t0.Add(-time.Hour)) // older than younger
	bigOlder.Status.ConfigCheckResult = boolPtr(true)
	bigOlder.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
	require.NoError(t, c.Create(context.Background(), bigOlder))

	bigGetter := staticGetter(map[string]*corev1.Secret{
		"team-big/creds": {Data: map[string][]byte{"cert": bigValue(700000)}},
		"team-a/creds":   {Data: map[string][]byte{"username": bigValue(700000)}},
	})

	result, _, err := resolveWorkloadPipelines(context.Background(), c, bigGetter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
	require.NoError(t, err)
	names := make([]string, 0, len(result))
	for _, p := range result {
		names = append(names, p.GetName())
	}
	assert.Equal(t, []string{"big"}, names, "bigOlder wins on age; younger now loses on size, not collision")

	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
	assert.False(t, gotYounger.IsValid())
	require.NotNil(t, gotYounger.Status.Reason)
	assert.Contains(t, *gotYounger.Status.Reason, secretSizeExclusionReasonPrefix,
		"the reason must now reflect the CURRENT failure (size), not the stale collision text from round 1")
	assert.NotContains(t, *gotYounger.Status.Reason, secretCollisionReasonPrefix)
}

func testVPForOscillation(ns, name string, created time.Time) *v1alpha1.VectorPipeline {
	return &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, CreationTimestamp: metav1.NewTime(created)},
		Spec: v1alpha1.VectorPipelineSpec{
			Secret: map[string]v1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
			)},
		},
	}
}
