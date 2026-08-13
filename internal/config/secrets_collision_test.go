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

package config

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// collidingVP builds a VectorPipeline reproducing the "team"+"a-x" / "team-a"+"x"
// flat-key ambiguity from TestSecretFlatKeyCollisionAcrossPipelines, with an explicit
// CreationTimestamp so tests can control attribution ordering.
func collidingVP(namespace, name string, created time.Time) *vectorv1alpha1.VectorPipeline {
	return &vectorv1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Secret: map[string]vectorv1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
			)},
		},
	}
}

func TestDetectSecretCollisionsOldestSurvives(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := collidingVP("team", "a-x", t0)
	younger := collidingVP("team-a", "x", t0.Add(time.Hour))

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	collisions, err := DetectSecretCollisions(getter, older, younger)
	require.NoError(t, err)
	require.Len(t, collisions, 1)
	assert.Equal(t, "team_a_x_es_username", collisions[0].FlatKey)
	assert.Equal(t, "team/a-x", collisions[0].Survivor)
	assert.Same(t, younger, collisions[0].Victim)

	// Order of arguments must not matter: the outcome is derived from
	// CreationTimestamp, not from pipeline list position.
	collisionsReversed, err := DetectSecretCollisions(getter, younger, older)
	require.NoError(t, err)
	require.Len(t, collisionsReversed, 1)
	assert.Same(t, younger, collisionsReversed[0].Victim)
}

func TestDetectSecretCollisionsTieBreakByPipelineID(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := collidingVP("team", "a-x", t0)
	b := collidingVP("team-a", "x", t0)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	collisions, err := DetectSecretCollisions(getter, a, b)
	require.NoError(t, err)
	require.Len(t, collisions, 1)
	// Equal timestamps: "team-a/x" < "team/a-x" lexicographically ('-' < '/'), so
	// "team-a/x" survives and "team/a-x" is excluded, deterministically regardless of
	// argument order.
	assert.Same(t, a, collisions[0].Victim)
	assert.Equal(t, "team-a/x", collisions[0].Survivor)

	collisionsReversed, err := DetectSecretCollisions(getter, b, a)
	require.NoError(t, err)
	require.Len(t, collisionsReversed, 1)
	assert.Same(t, a, collisionsReversed[0].Victim)
}

func TestDetectSecretCollisionsNoCollisionReturnsNil(t *testing.T) {
	vp1 := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	vp2 := testVPWithSecret("team-b", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1")}},
		"team-b/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	collisions, err := DetectSecretCollisions(getter, vp1, vp2)
	require.NoError(t, err)
	assert.Nil(t, collisions)
}

// Two different pipelines whose flat key happens to collide but resolve to the
// identical (namespace, secret, key) tuple are not a real collision - both correctly
// read the same value, nothing to attribute.
func TestDetectSecretCollisionsSameTupleAcrossPipelinesNotAVictim(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Both declare the backend namespace explicitly (ClusterVectorPipeline shape) so
	// they can share the exact same (namespace, secretName, key) tuple while still
	// being two distinct pipelines with colliding flat keys.
	older := &vectorv1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "a-x", CreationTimestamp: metav1.NewTime(t0)},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Secret: map[string]vectorv1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "shared"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
			)},
		},
	}
	younger := &vectorv1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "a_x", CreationTimestamp: metav1.NewTime(t0.Add(time.Hour))},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Secret: map[string]vectorv1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "shared"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6001"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
			)},
		},
	}
	// Sanity: "a-x" and "a_x" both sanitize (via flatKey's "-"->"_") to the same flat
	// key, same as the namespace-boundary ambiguity used elsewhere in this package.
	require.Equal(t, flatKey("", "a-x", "es", "username"), flatKey("", "a_x", "es", "username"))

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"shared/creds": {Data: map[string][]byte{"username": []byte("u1")}},
	})

	collisions, err := DetectSecretCollisions(getter, older, younger)
	require.NoError(t, err)
	assert.Nil(t, collisions)
}

// A three-pipeline group where the second-oldest member shares the survivor's exact
// tuple must not be victimized just for not being oldest - only the member with a
// genuinely different tuple is a real collision.
func TestDetectSecretCollisionsSameTupleAsSurvivorNotVictimized(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldest := collidingVP("team", "a-x", t0) // tuple: team/creds/username

	// Second-oldest, same flat key, SAME tuple as oldest (explicit backend namespace
	// "team", matching oldest's implicit one) - not a collision, just another correct
	// reader of the same secret. Reuses the "team-a-x" cluster-scoped name trick from
	// the three-way test above to land on the identical flat key.
	middleSameTuple := &vectorv1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a-x", CreationTimestamp: metav1.NewTime(t0.Add(time.Hour))},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Secret: map[string]vectorv1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "team"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6003"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
			)},
		},
	}

	youngestDifferentTuple := collidingVP("team-a", "x", t0.Add(2*time.Hour)) // tuple: team-a/creds/username

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	collisions, err := DetectSecretCollisions(getter, oldest, middleSameTuple, youngestDifferentTuple)
	require.NoError(t, err)
	require.Len(t, collisions, 1, "only the member with a genuinely different tuple must be reported")
	assert.Same(t, youngestDifferentTuple, collisions[0].Victim)
	assert.Equal(t, "team/a-x", collisions[0].Survivor)
}

// DetectSecretCollisions must not fold "detection aborted" (a pipeline-level problem
// while scanning) into "no collisions found" - the two mean very different things to a
// caller deciding whether it is safe to reinstate a previously-excluded pipeline.
func TestDetectSecretCollisionsReturnsErrorOnBrokenPipeline(t *testing.T) {
	vp1 := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	// A ClusterVectorPipeline backend without the required Namespace is a permanent
	// spec shape error, same as processPipelineSecrets enforces at build time.
	broken := testCVPWithSecret("broken-cvp",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"in": {"type": "vector", "address": "0.0.0.0:6004"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1")}},
	})

	collisions, err := DetectSecretCollisions(getter, vp1, broken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
	assert.Nil(t, collisions)
}

func TestDetectSecretCollisionsThreeWayGroupExcludesAllButOldest(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldest := collidingVP("team", "a-x", t0)
	middle := collidingVP("team-a", "x", t0.Add(time.Hour))
	// A third, distinct pipeline flattening to the identical key: a cluster-scoped
	// pipeline (no namespace) named "team-a-x" - generateName("", "team-a-x") is the
	// bare name, "team-a-x", the same string "team"+"-"+"a-x" and "team-a"+"-"+"x"
	// both produce.
	youngest := &vectorv1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a-x", CreationTimestamp: metav1.NewTime(t0.Add(2 * time.Hour))},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Secret: map[string]vectorv1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "cx"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6002"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
			)},
		},
	}
	require.Equal(t, flatKey("team", "a-x", "es", "username"), flatKey("", "team-a-x", "es", "username"))

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
		"cx/creds":     {Data: map[string][]byte{"username": []byte("u3")}},
	})

	collisions, err := DetectSecretCollisions(getter, oldest, middle, youngest)
	require.NoError(t, err)
	require.Len(t, collisions, 2)

	victims := map[string]bool{}
	for _, c := range collisions {
		assert.Equal(t, "team/a-x", c.Survivor, "the oldest pipeline must be the sole survivor of the whole group")
		victims[pipelineID(c.Victim)] = true
	}
	assert.True(t, victims["team-a/x"])
	assert.True(t, victims["team-a-x"])
}

// chainPool builds A(oldest)-B(middle)-C(youngest), all three sharing the identical
// flatKey base "team-a-x" (via the same ns/name dash-ambiguity trick used throughout
// this file: VP(team,a-x), VP(team-a,x), and CVP(team-a-x) all flatten their base to
// "team_a_x"). A and B collide on key1 ("es.username"); B and C collide on key2
// ("es2.pass") - a DIFFERENT flat key that A never references. B is older than C, so
// naive per-key-independent attribution would make B "win" key2 against C even though
// B itself is excluded from the build for losing key1 to A.
func chainPool(t *testing.T) (a, b, c pipeline.Pipeline, getter func(context.Context, string, string) (*corev1.Secret, error)) {
	t.Helper()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	a = testVPWithSecretCreated("team", "a-x", t0,
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"in": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	b = testVPWithSecretCreated("team-a", "x", t0.Add(time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es":  {Type: "kubernetes_secret", Name: "creds"},
			"es2": {Type: "kubernetes_secret", Name: "creds2"},
		},
		`{"in": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]", "password": "SECRET[es2.pass]"}}}`,
	)
	c = testCVPWithSecretCreated("team-a-x", t0.Add(2*time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es2": {Type: "kubernetes_secret", Name: "creds2", Namespace: "cx"}},
		`{"in": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"password": "SECRET[es2.pass]"}}}`,
	)

	getter = staticSecretGetter(map[string]*corev1.Secret{
		"team/creds":    {Data: map[string][]byte{"username": []byte("u-a")}},
		"team-a/creds":  {Data: map[string][]byte{"username": []byte("u-b")}},
		"team-a/creds2": {Data: map[string][]byte{"pass": []byte("p-b")}},
		"cx/creds2":     {Data: map[string][]byte{"pass": []byte("p-c")}},
	})
	return a, b, c, getter
}

// The chain case: B loses key1 to A and is excluded, but B is not a
// globally consistent "winner" of key2 - since B never makes it into the accepted set,
// C's key2 has nothing accepted to collide with and must survive.
func TestDetectSecretCollisionsChainDoesNotVictimizeUnrelatedLaterPipeline(t *testing.T) {
	a, b, c, getter := chainPool(t)

	collisions, err := DetectSecretCollisions(getter, a, b, c)
	require.NoError(t, err)
	require.Len(t, collisions, 1, "only B (loser of key1 to A) may be a victim; C must survive")
	assert.Same(t, b, collisions[0].Victim)
	assert.Equal(t, "team/a-x", collisions[0].Survivor)
	assert.Equal(t, "team_a_x_es_username", collisions[0].FlatKey)

	// Feeding the surviving pool (A and C - B excluded) into a real build must succeed
	// and must include C's reference, proving C is not just "not reported as a victim"
	// but actually usable.
	cfg, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, a, c)
	require.NoError(t, err)
	assert.Contains(t, cfg.Secret, SecretsBackendName)
	found := false
	for k := range cfg.internal.secretAssets {
		if k == "team_a_x_es2_pass" {
			found = true
		}
	}
	assert.True(t, found, "C's flat key must be resolved in the built config")
}

// Order of arguments/iteration must not change the outcome.
func TestDetectSecretCollisionsChainDeterministicAcrossRepeatedCalls(t *testing.T) {
	a, b, c, getter := chainPool(t)

	first, err := DetectSecretCollisions(getter, a, b, c)
	require.NoError(t, err)
	second, err := DetectSecretCollisions(getter, c, b, a)
	require.NoError(t, err)

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Same(t, first[0].Victim, second[0].Victim)
	assert.Equal(t, first[0].Survivor, second[0].Survivor)
	assert.Equal(t, first[0].FlatKey, second[0].FlatKey)
}

// A middle pipeline that is not itself involved in a collision must not end up named
// as the reported Survivor for a later pipeline that actually collides with the oldest
// one directly.
func TestDetectSecretCollisionsSurvivorNamesActualAcceptedPipelineNotBystander(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldest := collidingVP("team", "a-x", t0) // key: team_a_x_es_username, tuple team/creds/username

	// Middle pipeline: unrelated, no shared flat key with anyone.
	middle := testVPWithSecretCreated("unrelated-ns", "unrelated-name", t0.Add(time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	// Youngest: collides directly with oldest on the same flat key, different tuple.
	youngest := collidingVP("team-a", "x", t0.Add(2*time.Hour))

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team/creds":         {Data: map[string][]byte{"username": []byte("u1")}},
		"unrelated-ns/creds": {Data: map[string][]byte{"username": []byte("u-mid")}},
		"team-a/creds":       {Data: map[string][]byte{"username": []byte("u2")}},
	})

	collisions, err := DetectSecretCollisions(getter, oldest, middle, youngest)
	require.NoError(t, err)
	require.Len(t, collisions, 1)
	assert.Same(t, youngest, collisions[0].Victim)
	assert.Equal(t, "team/a-x", collisions[0].Survivor, "must name the pipeline it actually collides with, not the bystander")
}
