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
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func bytesOfLen(n int) []byte {
	return bytes.Repeat([]byte("a"), n)
}

func TestDetectSecretSizeOverflowOldestSurvives(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := testVPWithSecretCreated("team-a", "older", t0,
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	younger := testVPWithSecretCreated("team-b", "younger", t0.Add(time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)

	// 600000 + 600000 = 1200000 > corev1.MaxSecretSize (1048576), but neither alone
	// exceeds it - reproduces the live cross-tenant repro: two individually-valid
	// tenants whose combined values overflow the shared Secret.
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
		"team-b/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), older, younger)
	require.NoError(t, err)
	require.Len(t, exclusions, 1)
	assert.Same(t, younger, exclusions[0].Victim)
	assert.Equal(t, 600000, exclusions[0].AcceptedTotal)
	assert.Equal(t, 600000, exclusions[0].PipelineBytes)

	// Order of arguments must not matter: attribution follows CreationTimestamp.
	reversed, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), younger, older)
	require.NoError(t, err)
	require.Len(t, reversed, 1)
	assert.Same(t, younger, reversed[0].Victim)
}

func TestDetectSecretSizeOverflowTieBreakByPipelineID(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := testVPWithSecretCreated("z-ns", "p", t0,
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	b := testVPWithSecretCreated("a-ns", "p", t0, // "a-ns/p" < "z-ns/p" lexicographically
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"z-ns/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
		"a-ns/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), a, b)
	require.NoError(t, err)
	require.Len(t, exclusions, 1)
	assert.Same(t, a, exclusions[0].Victim, "a-ns/p sorts before z-ns/p, so it is accepted first and z-ns/p (a) is the excluded one")

	reversed, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), b, a)
	require.NoError(t, err)
	require.Len(t, reversed, 1)
	assert.Same(t, a, reversed[0].Victim)
}

func TestDetectSecretSizeOverflowNoOverflowReturnsNil(t *testing.T) {
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

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), vp1, vp2)
	require.NoError(t, err)
	assert.Nil(t, exclusions)
}

// A pipeline whose own secret values alone exceed the limit must be rejected outright
// - there is no smaller workload where it would ever fit.
func TestDetectSecretSizeOverflowSinglePipelineAloneExceedsLimit(t *testing.T) {
	vp := testVPWithSecret("team-a", "huge",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bytesOfLen(corev1.MaxSecretSize + 1)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), vp)
	require.NoError(t, err)
	require.Len(t, exclusions, 1)
	assert.Same(t, vp, exclusions[0].Victim)
	assert.Equal(t, 0, exclusions[0].AcceptedTotal)
	assert.Equal(t, corev1.MaxSecretSize+1, exclusions[0].PipelineBytes)
}

// Two pipelines that share the exact identical (namespace, secretName, key) tuple
// through the same ns/name dash-ambiguity DetectSecretCollisions exercises land on
// the SAME flat key - the aggregated Secret stores that value once. The size guard
// must not charge it twice: naively summing both pipelines' contributions would
// overflow (600000*2 > limit) even though the real aggregated Secret would hold the
// value only once (600000, well under it).
func TestDetectSecretSizeOverflowDedupsSharedFlatKey(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := collidingVP("team", "a-x", t0)
	sameTuple := &vectorv1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a-x", CreationTimestamp: metav1.NewTime(t0.Add(time.Hour))},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Secret: map[string]vectorv1alpha1.PipelineSecretBackend{
				"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "team"},
			},
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6010"}}`)},
			Sinks: &runtime.RawExtension{Raw: []byte(
				`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
			)},
		},
	}
	require.Equal(t, flatKey("team", "a-x", "es", "username"), flatKey("", "team-a-x", "es", "username"))

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team/creds": {Data: map[string][]byte{"username": bytesOfLen(600000)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), older, sameTuple)
	require.NoError(t, err)
	assert.Nil(t, exclusions, "the shared value must be counted once, not once per pipeline referencing it")
}

// A per-pipeline data problem (the declared key vanished after the pipeline was
// written, but its spec is otherwise fine) must come back wrapped in
// *SecretSizeDataError, distinct from a structural spec/shape error - callers treat
// the two very differently (see resolveWorkloadPipelines, which must not strip every
// retry candidate from the pool just because one pipeline's Secret key disappeared).
func TestDetectSecretSizeOverflowWrapsMissingKeyAsDataError(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	// The Secret exists, but the declared key does not.
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"other": []byte("x")}},
	})

	_, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), vp)
	require.Error(t, err)
	var dataErr *SecretSizeDataError
	assert.True(t, errors.As(err, &dataErr), "a missing-key failure must be a *SecretSizeDataError: got %T", err)
}

// DetectSecretSizeOverflow must not fold "detection aborted" into "fits" - the two
// mean very different things to a caller deciding whether it is safe to reinstate a
// previously-excluded pipeline (see the identical rule for DetectSecretCollisions).
// A structural spec error (as opposed to a per-pipeline data error, see the test
// above) must NOT be a *SecretSizeDataError.
func TestDetectSecretSizeOverflowReturnsErrorOnBrokenPipeline(t *testing.T) {
	vp1 := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	broken := testCVPWithSecret("broken-cvp",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"in": {"type": "vector", "address": "0.0.0.0:6011"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1")}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), vp1, broken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
	assert.Nil(t, exclusions)
	var dataErr *SecretSizeDataError
	assert.False(t, errors.As(err, &dataErr), "a structural spec error must not be a *SecretSizeDataError")
}

// The docs/secrets.md example: A=600 KiB, B=600 KiB, C=10 KiB, oldest to youngest.
// This is greedy-continue, not "trim the youngest until it fits": B is skipped for
// not fitting next to A, but C - younger than B - is still evaluated afterwards and
// kept, because skipping B does not remove it from the running total (it was never
// added) and does not disqualify anyone who comes after it.
func TestDetectSecretSizeOverflowGreedyContinuePastASkippedPipeline(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := testVPWithSecretCreated("team-a", "a", t0,
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	b := testVPWithSecretCreated("team-b", "b", t0.Add(time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	c := testVPWithSecretCreated("team-c", "c", t0.Add(2*time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
		"team-b/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
		"team-c/creds": {Data: map[string][]byte{"cert": bytesOfLen(10000)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), a, b, c)
	require.NoError(t, err)
	require.Len(t, exclusions, 1, "only B must be excluded - C must survive despite being younger than the excluded B")
	assert.Same(t, b, exclusions[0].Victim)
}

// Two pipelines can both overflow in the same pass (e.g. A survives, B and D both
// individually overflow against the running total) - each must get its own,
// independently correct reason, not share one or silently drop one.
func TestDetectSecretSizeOverflowMultipleSimultaneousVictimsGetOwnReasons(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := testVPWithSecretCreated("team-a", "a", t0,
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	b := testVPWithSecretCreated("team-b", "b", t0.Add(time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	d := testVPWithSecretCreated("team-d", "d", t0.Add(2*time.Hour),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
		"team-b/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
		"team-d/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), a, b, d)
	require.NoError(t, err)
	require.Len(t, exclusions, 2, "both B and D individually overflow against A's running total and must each be reported")

	victims := map[string]SecretSizeExclusion{}
	for _, e := range exclusions {
		victims[pipelineID(e.Victim)] = e
	}
	require.Contains(t, victims, "team-b/b")
	require.Contains(t, victims, "team-d/d")
	assert.Equal(t, 600000, victims["team-b/b"].AcceptedTotal, "B's own reason must reflect the total at the point B was considered")
	assert.Equal(t, 600000, victims["team-d/d"].AcceptedTotal, "D's own reason must reflect the same total - B never joined accepted, so D sees the identical baseline")
}

// The API server compares strictly greater-than (k8s.io/api/core/v1 MaxSecretSize),
// so a total exactly at the limit must be accepted, not excluded.
func TestDetectSecretSizeOverflowAtExactLimitIsAccepted(t *testing.T) {
	vp := testVPWithSecret("team-a", "app",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bytesOfLen(corev1.MaxSecretSize)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), vp)
	require.NoError(t, err)
	assert.Nil(t, exclusions, "exactly MaxSecretSize bytes must be accepted, not excluded")
}
