package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

// A pipeline without spec.role keeps its v0.6.0 hash. Otherwise every existing pipeline
// looks changed after an upgrade and is validated again.
func TestGetPipelineHashWithoutRoleMatchesV060(t *testing.T) {
	spec := v1alpha1.VectorPipelineSpec{
		Sources: &runtime.RawExtension{Raw: []byte(`{"logs":{"type":"kubernetes_logs"}}`)},
		Sinks:   &runtime.RawExtension{Raw: []byte(`{"out":{"inputs":["logs"],"type":"blackhole"}}`)},
	}
	tests := []struct {
		name     string
		pipeline Pipeline
		want     int64
	}{
		{
			name: "VectorPipeline",
			pipeline: &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "n"},
				Spec:       spec,
			},
			want: 456156848,
		},
		{
			name: "ClusterVectorPipeline with labels and a service name",
			pipeline: &v1alpha1.ClusterVectorPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "a",
					Labels:      map[string]string{"t": "x"},
					Annotations: map[string]string{"observability.kaasops.io/service-name": "s"},
				},
				Spec: spec,
			},
			want: 2241026221,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := GetPipelineHash(tt.pipeline)
			require.NoError(t, err)
			assert.Equal(t, tt.want, *h)
		})
	}
}
