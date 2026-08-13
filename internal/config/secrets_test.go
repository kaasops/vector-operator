package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

func TestScanAndRewriteSecretRefs(t *testing.T) {
	declared := map[string]v1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}}
	opts := map[string]any{
		"auth":    map[string]any{"user": "SECRET[es.username]", "strategy": "basic"},
		"headers": []any{"x-token: SECRET[es.token]"},
		"plain":   "no secrets here",
	}
	refs, err := scanAndRewriteSecretRefs(opts, "team-a", "app-logs", declared)
	require.NoError(t, err)
	require.ElementsMatch(t, []secretRef{{"es", "username"}, {"es", "token"}}, refs)
	require.Equal(t, "SECRET[k8s.team_a_app_logs_es_username]",
		opts["auth"].(map[string]any)["user"])
	require.Equal(t, "x-token: SECRET[k8s.team_a_app_logs_es_token]",
		opts["headers"].([]any)[0])
}

func TestScanUndeclaredAliasFails(t *testing.T) {
	_, err := scanAndRewriteSecretRefs(map[string]any{"a": "SECRET[nope.key]"}, "ns", "p", nil)
	require.ErrorContains(t, err, "nope")
}

func TestScanInvalidKeyCharsetFails(t *testing.T) {
	declared := map[string]v1alpha1.PipelineSecretBackend{"es": {Type: "kubernetes_secret", Name: "creds"}}
	// "/" is allowed by secretRefRegex's own key group ([A-Za-z0-9_./\-]+) - so this
	// SECRET[] placeholder does get matched and reach the walk - but keyCharsetRegex
	// (^[A-Za-z0-9_.-]+$) does not allow "/", so it must still be rejected there. A
	// character rejected by both regexes (e.g. "$") would prove nothing: the string
	// just would not match secretRefRegex at all, and this charset branch would never
	// run.
	_, err := scanAndRewriteSecretRefs(map[string]any{"a": "SECRET[es.user/name]"}, "ns", "p", declared)
	require.ErrorContains(t, err, "user/name")
	require.ErrorContains(t, err, "must match")
}

func TestFlatKeySanitized(t *testing.T) {
	require.Equal(t, "team_a_app_logs_es_username", flatKey("team-a", "app-logs", "es", "username"))
	require.Equal(t, "cvp1_es_tls.crt", flatKey("", "cvp1", "es", "tls.crt")) // CVP: no ns part; dots kept
}
