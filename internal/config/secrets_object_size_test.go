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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// testAssetsPrototype stands in for what a workload controller's builder produces: the
// assets Secret with its real metadata and no data. Kept minimal on purpose - the specs
// that care about metadata weight build their own.
func testAssetsPrototype() *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "vector-agent-secret-assets"}}
}

// The running object total is only valid because Data entries contribute
// independently to the generated Size(). That is an upstream implementation detail
// this code leans on, so it is pinned against the real Size() rather than assumed: if
// k8s.io/api ever makes entry costs interdependent, this fails instead of the budget
// silently drifting.
func TestSecretObjectSizeModelIsAdditive(t *testing.T) {
	data := map[string][]byte{}
	for i := 0; i < 500; i++ {
		data[fmt.Sprintf("team-%d_pipeline-%d_alias_key-name-that-is-fairly-long-%d", i, i, i)] = bytesOfLen(i % 97)
	}
	// A value long enough to cross protobuf's varint length boundaries, so the model
	// is not only checked on short ones.
	data["one-big"] = bytesOfLen(200000)

	prototype := testAssetsPrototype()
	withData := prototype.DeepCopy()
	withData.Data = data
	actual := withData.Size()

	assert.Equal(t, actual, secretObjectSize(prototype, data),
		"the modelled size must equal what the generated protobuf Size() reports for the same object")
}

// The anti-test for a model that charged base64 rather than raw bytes: a single value
// of exactly corev1.MaxSecretSize is legal (the values budget admits it - there is a
// dedicated test for that boundary), so the object budget must admit it too. A base64
// model would have inflated 1 MiB to ~1.33 MiB, exceeded the object budget, and
// evicted a perfectly valid pipeline for a limit it never actually breached.
func TestDetectSecretSizeOverflowObjectBudgetAdmitsOneMaxSizeValue(t *testing.T) {
	vp := testVPWithSecret("team-a", "big",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bytesOfLen(corev1.MaxSecretSize)}},
	})

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), vp)
	require.NoError(t, err)
	assert.Empty(t, exclusions,
		"a single value at exactly the Kubernetes limit fits both budgets - raw bytes are what etcd stores, not base64")
}

// The case the values-only budget cannot see at all: tiny values, but so many long
// flat keys that the object itself is what overflows. This is the shape measured on
// kind (~65-character keys, 24-byte values), the one that used to reach the API
// server and come back as a raw `etcdserver: request is too large`.
func TestDetectSecretSizeOverflowObjectBudgetCatchesManyLongKeys(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Long namespace/pipeline names are what make the generated flat keys long, the
	// same way a real per-pipeline key does.
	longNS := strings.Repeat("n", 40)
	sources := `{"logs": {"type": "kubernetes_logs"}}`

	// Each pipeline references many keys of one Secret; values are tiny.
	backends := map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}}
	sinkRefs := func(n int) string {
		var b strings.Builder
		b.WriteString(`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {`)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, `"opt%d": "SECRET[es.key-name-long-enough-to-matter-%d]"`, i, i)
		}
		b.WriteString(`}}}`)
		return b.String()
	}
	secretData := map[string][]byte{}
	for i := 0; i < 5000; i++ {
		secretData[fmt.Sprintf("key-name-long-enough-to-matter-%d", i)] = bytesOfLen(24)
	}
	getter := staticSecretGetter(map[string]*corev1.Secret{
		longNS + "/creds": {Data: secretData},
	})

	older := testVPWithSecretCreated(longNS, strings.Repeat("o", 30), t0, backends, sources, sinkRefs(5000))
	younger := testVPWithSecretCreated(longNS, strings.Repeat("y", 30), t0.Add(time.Hour), backends, sources, sinkRefs(5000))

	// Sanity: the values alone are nowhere near the values budget, so anything found
	// here is the object budget doing work the old guard could not do.
	assert.Less(t, 2*5000*24, corev1.MaxSecretSize)

	exclusions, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), older, younger)
	require.NoError(t, err)
	require.Len(t, exclusions, 1, "the pool overflows the object budget, so exactly one pipeline must be excluded")

	assert.Same(t, younger, exclusions[0].Victim, "attribution stays oldest-first, the same policy as the values budget")
	assert.True(t, exclusions[0].ObjectBudget, "the exclusion must be attributed to the OBJECT budget, not the values limit")
	assert.Greater(t, exclusions[0].PipelineObjectBytes, 0)
	assert.Greater(t, exclusions[0].AcceptedObjectBytes, 0)
	assert.LessOrEqual(t, exclusions[0].AcceptedObjectBytes+exclusions[0].PipelineObjectBytes, 2*SecretAssetsObjectBudget,
		"the reported numbers must be the modelled object sizes, not values")

	// And the survivor really is published: excluding the younger one has to leave a
	// pool that fits, otherwise the guard would just be trading one API error for a
	// permanently stuck workload.
	remaining, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), older)
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

// BridgeAssets stages against both budgets for the same reason the pre-pass checks
// both: a union that fits by values can still be an object the API server refuses.
func TestBridgeAssetsRespectsObjectBudget(t *testing.T) {
	longNS := strings.Repeat("n", 40)
	secretData := map[string][]byte{}
	for i := 0; i < 5000; i++ {
		secretData[fmt.Sprintf("key-name-long-enough-to-matter-%d", i)] = bytesOfLen(24)
	}
	getter := staticSecretGetter(map[string]*corev1.Secret{
		longNS + "/creds": {Data: secretData},
	})

	var b strings.Builder
	b.WriteString(`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {`)
	for i := 0; i < 3000; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, `"opt%d": "SECRET[es.key-name-long-enough-to-matter-%d]"`, i, i)
	}
	b.WriteString(`}}}`)

	vp := testVPWithSecret(longNS, strings.Repeat("p", 30),
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		b.String(),
	)

	// The assets Secret already holds enough stale key mass that adding this pipeline
	// would take the OBJECT over budget, while the values stay far under 1 MiB.
	existing := map[string][]byte{}
	for i := 0; i < 14000; i++ {
		existing[fmt.Sprintf("stale-namespace-name_stale-pipeline-name_alias_key-%d", i)] = bytesOfLen(24)
	}
	assert.Less(t, secretDataSize(existing), corev1.MaxSecretSize)

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), existing, []pipeline.Pipeline{vp})
	require.NoError(t, err)
	require.Len(t, waiting, 1, "the pipeline must wait for room rather than be staged into an object that cannot be written")
	assert.Same(t, vp, waiting[0])
	assert.LessOrEqual(t, secretObjectSize(testAssetsPrototype(), bridgeData), SecretAssetsObjectBudget,
		"whatever BridgeAssets hands back must itself be writable")
}

// The bug this budget used to have: the model charged only a name and a
// namespace, while the object the builder actually writes carries labels, an
// ownerReference, and annotations set on the CR's own spec (Vector.Spec.Agent.Annotations
// or the aggregator equivalent), sized entirely at the user's discretion up to
// Kubernetes' 256 KiB annotation ceiling. A candidate could therefore pass the model and
// still be rejected by etcd, which is the exact failure the budget exists to prevent.
// Modelling the builder's own object closes it: the same pool that fits next to bare
// metadata no longer fits next to heavy annotations.
func TestDetectSecretSizeOverflowChargesPrototypeMetadata(t *testing.T) {
	backends := map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}}
	sources := `{"logs": {"type": "kubernetes_logs"}}`
	longNS := strings.Repeat("n", 40)

	secretData := map[string][]byte{}
	var sink strings.Builder
	sink.WriteString(`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {`)
	for i := 0; i < 8000; i++ {
		secretData[fmt.Sprintf("key-name-long-enough-to-matter-%d", i)] = bytesOfLen(24)
		if i > 0 {
			sink.WriteString(", ")
		}
		fmt.Fprintf(&sink, `"opt%d": "SECRET[es.key-name-long-enough-to-matter-%d]"`, i, i)
	}
	sink.WriteString(`}}}`)
	getter := staticSecretGetter(map[string]*corev1.Secret{longNS + "/creds": {Data: secretData}})

	vp := testVPWithSecret(longNS, strings.Repeat("p", 30), backends, sources, sink.String())

	bare := testAssetsPrototype()
	accepted, err := DetectSecretSizeOverflow(context.Background(), getter, bare, vp)
	require.NoError(t, err)
	require.Empty(t, accepted, "this pool is sized to fit when only name and namespace are charged")

	// The same prototype as the builder would hand over, carrying a user-set spec
	// annotation of a size Kubernetes permits.
	heavy := bare.DeepCopy()
	heavy.Annotations = map[string]string{
		"team.example.com/pipeline-note": strings.Repeat("x", 220*1024),
	}
	heavy.Labels = map[string]string{"app.kubernetes.io/managed-by": "vector-operator"}

	excluded, err := DetectSecretSizeOverflow(context.Background(), getter, heavy, vp)
	require.NoError(t, err)
	require.Len(t, excluded, 1,
		"charged against the object the write actually carries, the same pool no longer fits and must be attributed rather than sent to the API server")
	assert.True(t, excluded[0].ObjectBudget)
	assert.Greater(t, excluded[0].AcceptedObjectBytes, 200*1024,
		"the metadata the builder attaches is part of what was already committed before any pipeline was considered")
}

// The running totals replaced a from-scratch measurement per pipeline, so the delta
// arithmetic now carries both budgets. This pins it through the decisions it drives,
// on a round containing all three kinds of movement at once: an ACCEPTED pipeline that
// merely rotates a key already staged (net cost zero - charging the new entry on top of
// the old instead of the difference would show up here), a REJECTED one that is rolled
// back (its cost must not stay in the totals), and a small ACCEPTED one after it (which
// only fits if neither of the previous two left the totals inflated).
func TestBridgeAssetsRunningTotalsMatchAFreshMeasurement(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := strings.Repeat("n", 40)
	prototype := testAssetsPrototype()
	backends := map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}}
	sources := `{"logs": {"type": "kubernetes_logs"}}`
	sinkFor := func(keys ...string) string {
		var b strings.Builder
		b.WriteString(`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {`)
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, `"opt%d": "SECRET[es.%s]"`, i, k)
		}
		b.WriteString(`}}}`)
		return b.String()
	}

	rotator := testVPWithSecretCreated(ns, strings.Repeat("a", 30), t0, backends, sources, sinkFor("rotating"))
	tooBig := testVPWithSecretCreated(ns, strings.Repeat("b", 30), t0.Add(time.Hour), backends, sources, sinkFor("bulky"))
	small := testVPWithSecretCreated(ns, strings.Repeat("c", 30), t0.Add(2*time.Hour), backends, sources, sinkFor("tiny"))

	getter := staticSecretGetter(map[string]*corev1.Secret{
		ns + "/creds": {Data: map[string][]byte{
			"rotating": bytesOfLen(200000), // same size as what is already staged: a pure rotation
			"bulky":    bytesOfLen(300000),
			"tiny":     bytesOfLen(1024),
		}},
	})

	// Fill the assets Secret close to the object budget, then stage the rotator's key at
	// its pre-rotation value.
	existing := map[string][]byte{}
	for i := 0; i < 11500; i++ {
		existing[fmt.Sprintf("stale-namespace-name_stale-pipeline-name_alias_key-%d", i)] = bytesOfLen(24)
	}
	existing[flatKey(ns, strings.Repeat("a", 30), "es", "rotating")] = bytesOfLen(200000)

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, prototype, existing, []pipeline.Pipeline{rotator, tooBig, small})
	require.NoError(t, err)

	// The remaining slack after the rotation is deliberately SMALLER than the rotated
	// value: charging that value twice (new entry without subtracting the old) would
	// push the rotator itself over and make it wait, which is what this asserts against.
	require.Len(t, waiting, 1, "only the pipeline that genuinely does not fit may wait")
	assert.Same(t, tooBig, waiting[0])

	assert.Equal(t, bytesOfLen(200000), bridgeData[flatKey(ns, strings.Repeat("a", 30), "es", "rotating")],
		"the rotation was accepted, so the staged value is the new one")
	assert.Contains(t, bridgeData, flatKey(ns, strings.Repeat("c", 30), "es", "tiny"),
		"the small pipeline after the rejected one still fits - a rollback that left its cost in the totals would have starved it")
	assert.NotContains(t, bridgeData, flatKey(ns, strings.Repeat("b", 30), "es", "bulky"))

	// And the decisions agree with what a from-scratch measurement says about the result.
	assert.LessOrEqual(t, secretObjectSize(prototype, bridgeData), SecretAssetsObjectBudget)
	assert.LessOrEqual(t, secretDataSize(bridgeData), corev1.MaxSecretSize)
}
