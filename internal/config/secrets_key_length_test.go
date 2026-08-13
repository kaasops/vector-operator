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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// keyOfLength returns a source key of exactly n characters, valid under
// keyCharsetRegex on its own - the point of these tests is that a perfectly valid
// user-supplied key can still become an invalid *generated* key once the operator's
// namespace/pipeline/alias prefix is added.
func keyOfLength(n int) string {
	return strings.Repeat("x", n)
}

// With ns="k", name="p", alias="a", flatKey = "k_p_a_" + key (6 fixed characters
// before the key), so key length n yields a flat key of exactly n+6 characters.
func TestSecretKeyTooLong_RejectsFlatKeyOverLimit(t *testing.T) {
	key := keyOfLength(248) // flat key: 6 + 248 = 254 characters, one over the limit
	vp := testVPWithSecret("k", "p",
		map[string]vectorv1alpha1.PipelineSecretBackend{"a": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[a.`+key+`]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"k/creds": {Data: map[string][]byte{key: []byte("v")}},
	})

	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.Error(t, err)

	var tooLong *SecretKeyTooLongError
	require.True(t, errors.As(err, &tooLong), "error must be a *SecretKeyTooLongError, not a raw string: got %v", err)
	assert.Equal(t, "a", tooLong.Alias)
	assert.Equal(t, key, tooLong.Key)
	assert.Len(t, tooLong.FlatKey, 254)
	assert.Contains(t, err.Error(), "254")
	assert.Contains(t, err.Error(), "253")
}

func TestSecretKeyLength_AtLimitSucceeds(t *testing.T) {
	key := keyOfLength(247) // flat key: 6 + 247 = 253 characters, exactly at the limit
	vp := testVPWithSecret("k", "p",
		map[string]vectorv1alpha1.PipelineSecretBackend{"a": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[a.`+key+`]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"k/creds": {Data: map[string][]byte{key: []byte("v")}},
	})

	cfg, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)
	assets := cfg.SecretAssets()
	require.Len(t, assets, 1)
	for flat := range assets {
		assert.Len(t, flat, 253)
	}
}
