/*
Copyright 2022.

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
	"strings"
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

func testVPWithSecret(namespace, name string, secret map[string]vectorv1alpha1.PipelineSecretBackend, sources, sinks string) pipeline.Pipeline {
	return &vectorv1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(sources)},
			Sinks:   &runtime.RawExtension{Raw: []byte(sinks)},
			Secret:  secret,
		},
	}
}

func testCVPWithSecret(name string, secret map[string]vectorv1alpha1.PipelineSecretBackend, sources, sinks string) pipeline.Pipeline {
	return &vectorv1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(sources)},
			Sinks:   &runtime.RawExtension{Raw: []byte(sinks)},
			Secret:  secret,
		},
	}
}

// testVPWithSecretCreated/testCVPWithSecretCreated are testVPWithSecret/testCVPWithSecret
// with an explicit CreationTimestamp, for tests that need to control
// DetectSecretCollisions' age-based attribution.
func testVPWithSecretCreated(namespace, name string, created time.Time, secret map[string]vectorv1alpha1.PipelineSecretBackend, sources, sinks string) pipeline.Pipeline {
	p := testVPWithSecret(namespace, name, secret, sources, sinks).(*vectorv1alpha1.VectorPipeline)
	p.CreationTimestamp = metav1.NewTime(created)
	return p
}

func testCVPWithSecretCreated(name string, created time.Time, secret map[string]vectorv1alpha1.PipelineSecretBackend, sources, sinks string) pipeline.Pipeline {
	p := testCVPWithSecret(name, secret, sources, sinks).(*vectorv1alpha1.ClusterVectorPipeline)
	p.CreationTimestamp = metav1.NewTime(created)
	return p
}

func staticSecretGetter(secrets map[string]*corev1.Secret) func(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return func(_ context.Context, namespace, name string) (*corev1.Secret, error) {
		key := namespace + "/" + name
		if s, ok := secrets[key]; ok {
			return s, nil
		}
		return nil, assertNotFoundErr(key)
	}
}

type secretNotFoundErr string

func (e secretNotFoundErr) Error() string { return "secret " + string(e) + " not found" }

func assertNotFoundErr(key string) error { return secretNotFoundErr(key) }

func TestAgentConfigWithSecrets(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1")}},
	})

	cfg, jsonBytes, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)

	assert.Contains(t, string(jsonBytes), `"secret":{"k8s":{"path":"/etc/vector/secrets","type":"directory"}}`)

	flat := "team_a_app_logs_es_username"
	assert.Contains(t, string(jsonBytes), `SECRET[k8s.`+flat+`]`)
	assert.NotContains(t, string(jsonBytes), "u1")

	assets := cfg.SecretAssets()
	require.Equal(t, map[string][]byte{flat: []byte("u1")}, assets)
}

func TestAgentConfigZeroChurnWithoutSecrets(t *testing.T) {
	vp := testPipeline("ns-a", "pipe",
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "blackhole", "inputs": ["logs"]}}`)

	_, withoutGetter, err := BuildAgentConfig(VectorConfigParams{}, vp)
	require.NoError(t, err)

	getter := staticSecretGetter(nil)
	cfgWithGetter, withGetter, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)

	assert.Equal(t, string(withoutGetter), string(withGetter))
	assert.NotContains(t, string(withGetter), `"secret"`)
	assert.Empty(t, cfgWithGetter.SecretAssets())
}

func TestAgentConfigSecretMissingKey(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.password]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1")}},
	})

	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "password"), "error should mention the missing key: %v", err)
}

func TestVPNamespaceFieldForbidden(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds", Namespace: "other"},
		},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "blackhole", "inputs": ["logs"]}}`,
	)

	getter := staticSecretGetter(nil)
	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is not allowed in VectorPipeline")
}

// BuildAggregatorConfig gets the same integration as BuildAgentConfig, so a smoke
// test covers it symmetrically.
func TestAggregatorConfigWithSecrets(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1")}},
	})

	cfg, err := BuildAggregatorConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)

	jsonBytes, err := cfg.MarshalJSON()
	require.NoError(t, err)

	flat := "team_a_app_logs_es_username"
	assert.Contains(t, string(jsonBytes), `"secret":{"k8s":{"path":"/etc/vector/secrets","type":"directory"}}`)
	assert.Contains(t, string(jsonBytes), `SECRET[k8s.`+flat+`]`)
	assert.NotContains(t, string(jsonBytes), "u1")
	require.Equal(t, map[string][]byte{flat: []byte("u1")}, cfg.SecretAssets())
}

// The flat key is generateName(ns, name)+"-"+alias+"-"+key with every "-" replaced
// by "_", so ns "team"+pipeline "a-x" and ns "team-a"+pipeline "x" produce the
// identical flat key for the same alias/key. Without a collision guard, whichever
// pipeline resolves second silently overwrites the other's secret value.
func TestSecretFlatKeyCollisionAcrossPipelines(t *testing.T) {
	vp1 := testVPWithSecret("team", "a-x",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]"}}}`,
	)
	vp2 := testVPWithSecret("team-a", "x",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"logs2": {"type": "kubernetes_logs"}}`,
		`{"out2": {"type": "elasticsearch", "inputs": ["logs2"], "auth": {"user": "SECRET[es.username]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team/creds":   {Data: map[string][]byte{"username": []byte("u1")}},
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u2")}},
	})

	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp1, vp2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team_a_x_es_username")
	assert.Contains(t, err.Error(), "team/a-x")
	assert.Contains(t, err.Error(), "team-a/x")
}

// The same flat key resolving to the same (namespace, secret, key) tuple must stay
// legal: a pipeline referencing the same alias.key twice is not a collision.
func TestSecretFlatKeySameTupleTwiceSucceeds(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]", "user2": "SECRET[es.username]"}}}`,
	)

	getter := staticSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1")}},
	})

	cfg, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)

	flat := "team_a_app_logs_es_username"
	assert.Equal(t, map[string][]byte{flat: []byte("u1")}, cfg.SecretAssets())
}

// countingSecretGetter wraps staticSecretGetter and counts calls per namespace/name key,
// so a test can assert exactly how many times the API was actually read.
func countingSecretGetter(secrets map[string]*corev1.Secret) (func(ctx context.Context, namespace, name string) (*corev1.Secret, error), map[string]int) {
	counts := make(map[string]int)
	inner := staticSecretGetter(secrets)
	return func(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
		counts[namespace+"/"+name]++
		return inner(ctx, namespace, name)
	}, counts
}

// TestResolvePendingSecretsMemoizesGetPerSecret pins resolvePendingSecrets' documented
// memoization (see its doc comment: "fetched once" per namespace/name): a pipeline
// referencing two different keys of the same Secret must only cost one Get, not one per
// SECRET[] reference.
func TestResolvePendingSecretsMemoizesGetPerSecret(t *testing.T) {
	vp := testVPWithSecret("team-a", "app-logs",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.username]", "pass": "SECRET[es.password]"}}}`,
	)

	getter, counts := countingSecretGetter(map[string]*corev1.Secret{
		"team-a/creds": {Data: map[string][]byte{"username": []byte("u1"), "password": []byte("p1")}},
	})

	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, vp)
	require.NoError(t, err)
	assert.Equal(t, 1, counts["team-a/creds"], "two SECRET[] refs into the same Secret must cost exactly one Get")
}

func TestCVPNamespaceRequired(t *testing.T) {
	cvp := testCVPWithSecret("cvp1",
		map[string]vectorv1alpha1.PipelineSecretBackend{
			"es": {Type: "kubernetes_secret", Name: "creds"},
		},
		`{"logs": {"type": "kubernetes_logs", "extra_namespace_label_selector": "kubernetes.io/metadata.name=ns-x"}}`,
		`{"out": {"type": "blackhole", "inputs": ["logs"]}}`,
	)

	getter := staticSecretGetter(nil)
	_, _, err := BuildAgentConfig(VectorConfigParams{PipelineSecretGetter: getter}, cvp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
}
