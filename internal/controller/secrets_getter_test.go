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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// countingReader wraps a fake client.Reader and counts real Get calls per key, so
// tests can assert how many times the underlying API was actually hit.
type countingReader struct {
	client.Reader
	calls map[string]int
}

func (r *countingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	r.calls[key.String()]++
	return r.Reader.Get(ctx, key, obj, opts...)
}

func TestPipelineSecretGetterMemoizesRepeatedCalls(t *testing.T) {
	s := &corev1.Secret{}
	s.Name = "creds"
	s.Namespace = "team-a"
	s.Data = map[string][]byte{"cert": []byte("v1")}

	base := newFakeClient(s)
	counting := &countingReader{Reader: base, calls: map[string]int{}}

	getter := pipelineSecretGetter(counting, context.Background())
	require.NotNil(t, getter)

	for i := 0; i < 5; i++ {
		got, err := getter(context.Background(), "team-a", "creds")
		require.NoError(t, err)
		assert.Equal(t, s.Data, got.Data)
	}

	assert.Equal(t, 1, counting.calls["team-a/creds"],
		"5 calls for the identical Secret must result in exactly 1 real API read - DetectSecretCollisions, "+
			"DetectSecretSizeOverflow, BridgeAssets, and Build*Config's own resolvePendingSecrets can all ask for it in one reconcile")
}

func TestPipelineSecretGetterMemoizesDistinctSecretsSeparately(t *testing.T) {
	s1 := &corev1.Secret{}
	s1.Name, s1.Namespace = "creds1", "team-a"
	s2 := &corev1.Secret{}
	s2.Name, s2.Namespace = "creds2", "team-b"

	base := newFakeClient(s1, s2)
	counting := &countingReader{Reader: base, calls: map[string]int{}}
	getter := pipelineSecretGetter(counting, context.Background())

	_, err := getter(context.Background(), "team-a", "creds1")
	require.NoError(t, err)
	_, err = getter(context.Background(), "team-b", "creds2")
	require.NoError(t, err)
	_, err = getter(context.Background(), "team-a", "creds1")
	require.NoError(t, err)

	assert.Equal(t, 1, counting.calls["team-a/creds1"])
	assert.Equal(t, 1, counting.calls["team-b/creds2"])
}

// A cached Get failure (a transient API error, a genuinely missing Secret) must also
// be memoized - a repeated call within the same reconcile must not silently retry and
// possibly observe a different result mid-round.
func TestPipelineSecretGetterMemoizesErrors(t *testing.T) {
	base := newFakeClient()
	counting := &countingReader{Reader: base, calls: map[string]int{}}
	getter := pipelineSecretGetter(counting, context.Background())

	_, err1 := getter(context.Background(), "team-a", "missing")
	require.Error(t, err1)
	assert.True(t, api_errors.IsNotFound(err1))

	_, err2 := getter(context.Background(), "team-a", "missing")
	require.Error(t, err2)
	assert.True(t, errors.Is(err2, err1) || api_errors.IsNotFound(err2))

	assert.Equal(t, 1, counting.calls["team-a/missing"], "the second call must be served from the cached error, not a second real Get")
}
