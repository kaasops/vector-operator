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

package pipeline

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/utils/hash"
)

// Regression for #232: the CRC32 config hash is a uint32 and routinely exceeds the
// int32 ceiling (2147483647). The status field must be a signed 64-bit int so such a
// value is stored verbatim; a uint32->int32 schema (or cast) rejects it at the
// apiserver and wedges the reconcile loop with configCheckResult=false forever.
func TestLastAppliedPipelineHashHoldsFullUint32Range(t *testing.T) {
	// crc32("test") == 3632233996, which is > math.MaxInt32 — the exact class of value
	// that used to be rejected.
	raw := hash.Get([]byte("test"))
	require.Greater(t, raw, uint32(math.MaxInt32), "test input must hash above int32 max")

	stored := int64(raw)
	vp := &v1alpha1.VectorPipeline{}
	vp.SetLastAppliedPipeline(&stored)

	require.NotNil(t, vp.GetLastAppliedPipeline())
	assert.Equal(t, int64(raw), *vp.GetLastAppliedPipeline())
	assert.Positive(t, *vp.GetLastAppliedPipeline(), "hash must not overflow into a negative value")
	assert.LessOrEqual(t, *vp.GetLastAppliedPipeline(), int64(math.MaxUint32))
}

// Toggling the per-pipeline config-optimization opt-out annotation must change the
// pipeline hash so the reconcile propagates it to an agent config rebuild.
func TestGetPipelineHashTracksConfigOptimizationAnnotation(t *testing.T) {
	base := &v1alpha1.VectorPipeline{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
	h1, err := GetPipelineHash(base)
	require.NoError(t, err)

	opted := &v1alpha1.VectorPipeline{ObjectMeta: metav1.ObjectMeta{
		Name:        "p",
		Namespace:   "ns",
		Annotations: map[string]string{common.AnnotationConfigOptimization: "disabled"},
	}}
	h2, err := GetPipelineHash(opted)
	require.NoError(t, err)

	assert.NotEqual(t, *h1, *h2)
}

// The same for a ClusterVectorPipeline (cluster-scoped opt-out path).
func TestGetPipelineHashTracksConfigOptimizationAnnotationCVP(t *testing.T) {
	base := &v1alpha1.ClusterVectorPipeline{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	h1, err := GetPipelineHash(base)
	require.NoError(t, err)

	opted := &v1alpha1.ClusterVectorPipeline{ObjectMeta: metav1.ObjectMeta{
		Name:        "p",
		Annotations: map[string]string{common.AnnotationConfigOptimization: common.AnnotationValueDisabled},
	}}
	h2, err := GetPipelineHash(opted)
	require.NoError(t, err)

	assert.NotEqual(t, *h1, *h2)
}

// Without the annotation the hash must match the pre-field struct, so an operator
// upgrade does not re-hash and re-validate every pipeline.
func TestGetPipelineHashStableWithoutAnnotation(t *testing.T) {
	p := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: v1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(`{"logs":{"type":"kubernetes_logs"}}`)},
		},
	}
	h, err := GetPipelineHash(p)
	require.NoError(t, err)

	legacy, err := json.Marshal(struct {
		Spec        v1alpha1.VectorPipelineSpec
		Labels      map[string]string
		ServiceName string
	}{Spec: p.Spec})
	require.NoError(t, err)
	assert.Equal(t, int64(hash.Get(legacy)), *h)
}

// Toggling the annotation must read as "changed" (IsPipelineChanged == false) so the
// pipeline reconcile propagates to an agent rebuild.
func TestIsPipelineChangedDetectsConfigOptimizationToggle(t *testing.T) {
	base := &v1alpha1.VectorPipeline{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
	h, err := GetPipelineHash(base)
	require.NoError(t, err)

	annotated := &v1alpha1.VectorPipeline{ObjectMeta: metav1.ObjectMeta{
		Name:        "p",
		Namespace:   "ns",
		Annotations: map[string]string{common.AnnotationConfigOptimization: common.AnnotationValueDisabled},
	}}
	annotated.SetLastAppliedPipeline(h) // the no-annotation hash was last applied

	unchanged, err := IsPipelineChanged(annotated)
	require.NoError(t, err)
	assert.False(t, unchanged)
}
