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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// TestSecretValueSafeForJSONText pins the exact predicate: valid UTF-8 with no '"',
// '\', or 0x00-0x1F control byte passes; everything else with one of those is
// rejected; an empty value always passes (that is vector's own contract to enforce,
// not this guard's - see the function's own doc comment for the
// src/secrets/directory.rs citation).
func TestSecretValueSafeForJSONText(t *testing.T) {
	cases := []struct {
		name  string
		value []byte
		safe  bool
	}{
		{"empty value", []byte(""), true},
		{"double quote", []byte(`has "a" quote`), false},
		{"backslash alone", []byte(`back\slash`), false},
		{"backslash-b escape", []byte("x\\by"), false},
		{"backslash-n escape", []byte("x\\ny"), false},
		{"NUL byte", []byte{0x00}, false},
		{"tab byte 0x09", []byte{0x09}, false},
		{"newline byte 0x0a", []byte{0x0a}, false},
		{"unit separator 0x1f", []byte{0x1f}, false},
		{"space 0x20 passes - first byte NOT rejected", []byte{0x20}, true},
		{"ordinary ASCII", []byte("plain-ascii_value123"), true},
		{"forward slash", []byte("a/b/c"), true},
		{"DEL 0x7f passes - only 0x00-0x1F is rejected", []byte{0x7f}, true},
		{"Unicode text", []byte("café日本語"), true},
		{"invalid UTF-8", []byte{0xff, 0xfe, 0xfd}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.safe, secretValueSafeForJSONText(tc.value))
		})
	}
}

// TestBuildAgentConfigRejectsUnsafeSecretValue pins the guard on the final-assembly
// path (resolvePendingSecrets): a value containing a double quote must fail
// BuildAgentConfig with *SecretValueUnsafeError, naming the Secret/key but never
// repeating the value itself.
func TestBuildAgentConfigRejectsUnsafeSecretValue(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	// A distinctive marker, not sharing any word with the guard's own fixed rule
	// description ("double quote, backslash, or control byte"), so a substring
	// check below cannot be confused by the generic wording of the message itself.
	const secretMarker = "xk7Q-p0nyZ4rd"
	unsafe := secretMarker + `"` + secretMarker
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte(unsafe)}},
	})

	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.Error(t, err)

	var unsafeErr *SecretValueUnsafeError
	require.True(t, errors.As(err, &unsafeErr), "must be a *SecretValueUnsafeError: %v", err)
	assert.Equal(t, "team-a", unsafeErr.SecretNamespace)
	assert.Equal(t, "creds", unsafeErr.SecretName)
	assert.Equal(t, "username", unsafeErr.Key)

	assert.Contains(t, err.Error(), "creds")
	assert.Contains(t, err.Error(), "username")
	assert.NotContains(t, err.Error(), unsafe, "the error must never repeat the unsafe value")
	assert.NotContains(t, err.Error(), secretMarker, "the error must not leak even a fragment of the value's content")
}

// TestBuildAgentConfigRejectsBackslashSecretValue covers both fates a lone backslash
// can have, depending entirely on the character right after it in the generated
// config - this guard cannot tell them apart in advance, so it rejects a backslash
// unconditionally:
//   - `pass\word`: '\w' is not a JSON escape sequence at all, so the result is
//     invalid JSON - the same rejected-config outcome the quote case gets, and
//     configcheck (when enabled) would catch it.
//   - `pass\bword`: '\b' IS a valid JSON escape (backspace), so vector and
//     `vector validate` both accept it - it decodes to different bytes than what the
//     Secret actually stores, silently. This is the case that cannot be caught
//     downstream and the reason this guard exists at all, rather than leaving it to
//     configcheck.
func TestBuildAgentConfigRejectsBackslashSecretValue(t *testing.T) {
	for name, value := range map[string]string{
		"invalid JSON escape ('\\w')":                  `pass\word`,
		"valid JSON escape, silent corruption ('\\b')": `pass\bword`,
	} {
		t.Run(name, func(t *testing.T) {
			vp := testVPWithSecret("team-a", "app-logs",
				map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
				`{"logs": {"type": "kubernetes_logs"}}`,
				`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
			)
			getter := staticSecretGetter(map[string]*corev1.Secret{
				"team-a/creds": {Data: map[string][]byte{"username": []byte(value)}},
			})

			_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
			require.Error(t, err)
			var unsafeErr *SecretValueUnsafeError
			require.True(t, errors.As(err, &unsafeErr))
		})
	}
}

// TestBuildAgentConfigRejectsControlByteSecretValue covers four control bytes:
// 0x00, 0x09 (tab), 0x0a (newline), and 0x1f, the boundary right below the first
// byte that is legal (0x20, space).
func TestBuildAgentConfigRejectsControlByteSecretValue(t *testing.T) {
	for _, b := range []byte{0x00, 0x09, 0x0a, 0x1f} {
		value := append([]byte("prefix-"), b)
		vp := testVPWithSecret("team-a", "app-logs",
			map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
			`{"logs": {"type": "kubernetes_logs"}}`,
			`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
		)
		getter := staticSecretGetter(map[string]*corev1.Secret{
			"team-a/creds": {Data: map[string][]byte{"username": value}},
		})
		_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
		require.Errorf(t, err, "control byte 0x%02x must be rejected", b)
		var unsafeErr *SecretValueUnsafeError
		require.Truef(t, errors.As(err, &unsafeErr), "control byte 0x%02x: %v", b, err)
	}
}

// TestBuildAgentConfigRejectsInvalidUTF8SecretValue: vector's directory backend reads
// the mounted file with tokio::fs::read_to_string, which requires UTF-8 - invalid
// bytes must be caught here, with an attributed reason, rather than surfacing as an
// unattributed vector-side runtime failure.
func TestBuildAgentConfigRejectsInvalidUTF8SecretValue(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": {0xff, 0xfe, 0xfd}}},
	})
	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.Error(t, err)
	var unsafeErr *SecretValueUnsafeError
	require.True(t, errors.As(err, &unsafeErr))
}

// TestBuildAgentConfigAcceptsJSONSafeSecretValue is the anti-test: space (0x20, the
// byte immediately above the rejected 0x00-0x1F range), ordinary ASCII, '/', Unicode,
// and 0x7F and above must all pass byte for byte, unmodified - this guard exists to
// keep the JSON substitution safe, not to restrict what a credential may contain
// beyond that.
func TestBuildAgentConfigAcceptsJSONSafeSecretValue(t *testing.T) {
	value := []byte("  plain/ascii_日本語\x7f-value  ")
	require.True(t, secretValueSafeForJSONText(value), "sanity: the fixture itself must be judged safe")

	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": value}},
	})

	cfg, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)
	assert.Equal(t, value, cfg.SecretAssets()["team_a_app_logs_es_username"], "the value must reach the assets map byte for byte, unmodified")
}

// TestBuildAgentConfigAcceptsEmptySecretValue: an empty value is JSON-safe on its own
// (there is nothing in it to corrupt anything with) and this guard leaves it alone -
// whether vector's own directory backend accepts an empty secret file is a separate
// contract (it does not - see secretValueSafeForJSONText's doc comment), not this
// guard's to enforce.
func TestBuildAgentConfigAcceptsEmptySecretValue(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("")}},
	})

	cfg, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)
	assert.Equal(t, []byte(""), cfg.SecretAssets()["team_a_app_logs_es_username"])
}

// TestDetectSecretSizeOverflowRejectsUnsafeValue pins the guard on the size pre-pass:
// the SAME predicate must reject there too, wrapped in *SecretSizeDataError so it is
// treated as this one pipeline's own data problem (see DetectSecretSizeOverflow's doc
// comment) rather than aborting size attribution for the whole pool.
func TestDetectSecretSizeOverflowRejectsUnsafeValue(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte(`bad"value`)}},
	})

	_, err := DetectSecretSizeOverflow(context.Background(), getter, testAssetsPrototype(), vp)
	require.Error(t, err)
	var dataErr *SecretSizeDataError
	require.True(t, errors.As(err, &dataErr), "must be a *SecretSizeDataError: %v", err)
	var unsafeErr *SecretValueUnsafeError
	require.True(t, errors.As(err, &unsafeErr), "must unwrap to *SecretValueUnsafeError: %v", err)
}

// TestBridgeAssetsRejectsUnsafeValue pins the guard on the bridge path, mirroring
// TestDetectSecretSizeOverflowRejectsUnsafeValue's expectations exactly - the same
// *SecretSizeDataError wrapping planSecretAssetsBridge already special-cases.
func TestBridgeAssetsRejectsUnsafeValue(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte(`bad"value`)}},
	})

	_, _, err := BridgeAssets(context.Background(), getter, testAssetsPrototype(), nil, []pipeline.Pipeline{vp})
	require.Error(t, err)
	var dataErr *SecretSizeDataError
	require.True(t, errors.As(err, &dataErr), "must be a *SecretSizeDataError: %v", err)
	var unsafeErr *SecretValueUnsafeError
	require.True(t, errors.As(err, &unsafeErr), "must unwrap to *SecretValueUnsafeError: %v", err)
}
