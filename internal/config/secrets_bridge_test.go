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

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

func TestBridgeAssetsNoBridgeNeededWhenEverythingFits(t *testing.T) {
	vp := testVPWithSecret("team-a", "app",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": []byte("small")}},
	})

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), nil, []pipeline.Pipeline{vp})
	require.NoError(t, err)
	assert.Empty(t, waiting, "no bridge round needed when the target comfortably fits")
	assert.Equal(t, map[string][]byte{"team_a_app_es_cert": []byte("small")}, bridgeData)
}

func TestBridgeAssetsExcludesPipelineThatWouldOverflowAlongsideExisting(t *testing.T) {
	vp := testVPWithSecret("team-b", "younger",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-b/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
	})
	existing := map[string][]byte{"stale_older_key": bytesOfLen(600000)}

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), existing, []pipeline.Pipeline{vp})
	require.NoError(t, err)
	require.Len(t, waiting, 1)
	assert.Same(t, vp, waiting[0])
	assert.Equal(t, existing, bridgeData, "bridgeData must be untouched - nothing already staged may be dropped by this call")
}

func TestBridgeAssetsSameValueAlreadyStagedIsFree(t *testing.T) {
	vp := testVPWithSecret("team-a", "app",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	value := bytesOfLen(900000)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": value}},
	})
	// Already staged with the identical value - re-accepting it must cost nothing,
	// even though 900000 alone is close to the ceiling.
	existing := map[string][]byte{"team_a_app_es_cert": value}

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), existing, []pipeline.Pipeline{vp})
	require.NoError(t, err)
	assert.Empty(t, waiting)
	assert.Equal(t, existing, bridgeData)
}

// A value rotation on an already-staged key must be charged by its actual net
// change, not treated as a brand new addition - otherwise a same-round config
// change plus a value rotation of an existing key would be double-counted.
func TestBridgeAssetsValueRotationChargedByNetDelta(t *testing.T) {
	vp := testVPWithSecret("team-a", "app",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	oldValue := bytesOfLen(100000)
	newValue := bytesOfLen(150000) // rotated to a slightly larger value
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": newValue}},
	})
	existing := map[string][]byte{"team_a_app_es_cert": oldValue}

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), existing, []pipeline.Pipeline{vp})
	require.NoError(t, err)
	assert.Empty(t, waiting, "the net delta (+50000) fits comfortably")
	assert.Equal(t, newValue, bridgeData["team_a_app_es_cert"])

	// Now rotate to a value whose delta alone, on top of unrelated stale data,
	// overflows - proving the charge is the real total, not zero just because the
	// key already existed.
	hugeValue := bytesOfLen(1000000)
	getterHuge := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": hugeValue}},
	})
	existingWithNeighbor := map[string][]byte{
		"team_a_app_es_cert": oldValue,
		"unrelated_stale":    bytesOfLen(100000),
	}
	bridgeData2, waiting2, err := BridgeAssets(context.Background(), getterHuge, testAssetsPrototype(), existingWithNeighbor, []pipeline.Pipeline{vp})
	require.NoError(t, err)
	require.Len(t, waiting2, 1, "1000000 + 100000 > MaxSecretSize even though the key already existed")
	assert.Equal(t, existingWithNeighbor, bridgeData2, "rolled back exactly - the pre-existing (smaller) value must survive untouched")
}

// Rollback must restore bridgeData exactly - including a key that did NOT exist
// before this pipeline's tentative application, which must be removed entirely
// rather than left present with a stale/zero value.
func TestBridgeAssetsRollbackRemovesKeysThatDidNotExistBefore(t *testing.T) {
	vp := testVPWithSecret("team-c", "app",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-c/creds": {Data: map[string][]byte{"cert": bytesOfLen(600000)}},
	})
	existing := map[string][]byte{"stale": bytesOfLen(600000)}

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), existing, []pipeline.Pipeline{vp})
	require.NoError(t, err)
	require.Len(t, waiting, 1)
	_, present := bridgeData["team_c_app_es_cert"]
	assert.False(t, present, "a key that did not exist before a rejected pipeline's tentative application must not linger")
	assert.Len(t, bridgeData, 1)
}

// Oldest-first among the pipelines actually being staged: if two new pipelines
// together would overflow existing headroom, the older one must be preferred -
// mirrors DetectSecretSizeOverflow's own policy, applied here to the bridge step.
func TestBridgeAssetsPrefersOlderAmongCandidates(t *testing.T) {
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
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"cert": bytesOfLen(700000)}},
		"team-b/creds": {Data: map[string][]byte{"cert": bytesOfLen(700000)}},
	})

	bridgeData, waiting, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), nil, []pipeline.Pipeline{older, younger})
	require.NoError(t, err)
	require.Len(t, waiting, 1)
	assert.Same(t, younger, waiting[0])
	assert.Contains(t, bridgeData, "team_a_older_es_cert")
	assert.NotContains(t, bridgeData, "team_b_younger_es_cert")
}

func TestBridgeAssetsPropagatesDataError(t *testing.T) {
	vp := testVPWithSecret("team-a", "app",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
	)
	getter := staticSecretGetter(nil) // creds Secret does not exist

	_, _, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), nil, []pipeline.Pipeline{vp})
	require.Error(t, err)
}
