package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// A component given as null decodes to a nil pointer; it must fail the pipeline with a
// reason instead of reaching code that reads its type.
func TestUnmarshalJsonRejectsNullComponent(t *testing.T) {
	for name, spec := range map[string]vectorv1alpha1.VectorPipelineSpec{
		"source": {
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": null}`)},
			Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)},
		},
		"transform": {
			Sources:    &runtime.RawExtension{Raw: []byte(`{"in": {"type": "demo_logs"}}`)},
			Transforms: &runtime.RawExtension{Raw: []byte(`{"t": null}`)},
			Sinks:      &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)},
		},
		"sink": {
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "demo_logs"}}`)},
			Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": null}`)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := UnmarshalJson(spec, &PipelineConfig{})
			require.ErrorContains(t, err, "is empty")
		})
	}
}
