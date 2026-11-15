package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// PipelineRole must agree with the full decode the pipeline's own reconcile does.
func TestPipelineRoleMatchesFullDecode(t *testing.T) {
	agg := vectorv1alpha1.VectorPipelineRoleAggregator
	raw := func(s string) *runtime.RawExtension { return &runtime.RawExtension{Raw: []byte(s)} }
	sinks := raw(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)
	specs := map[string]vectorv1alpha1.VectorPipelineSpec{
		"agent":             {Sources: raw(`{"in": {"type": "kubernetes_logs"}}`), Sinks: sinks},
		"aggregator":        {Sources: raw(`{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`), Sinks: sinks},
		"dual-role":         {Sources: raw(`{"in": {"type": "internal_metrics"}}`), Sinks: sinks},
		"pinned":            {Role: &agg, Sources: raw(`{"in": {"type": "internal_metrics"}}`), Sinks: sinks},
		"pin not supported": {Role: &agg, Sources: raw(`{"in": {"type": "kubernetes_logs"}}`), Sinks: sinks},
		"mixed":             {Sources: raw(`{"a": {"type": "kubernetes_logs"}, "b": {"type": "kafka"}}`), Sinks: sinks},
		"unknown type":      {Sources: raw(`{"in": {"type": "no_such_source"}}`), Sinks: sinks},
		"no type":           {Sources: raw(`{"in": {}}`), Sinks: sinks},
		"null source":       {Sources: raw(`{"in": null}`), Sinks: sinks},
		"type not a string": {Sources: raw(`{"in": {"type": 1}}`), Sinks: sinks},
		"no sources":        {Sinks: sinks},
		"empty sources":     {Sources: raw(`{}`), Sinks: sinks},
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			var want *vectorv1alpha1.VectorPipelineRole
			p := &PipelineConfig{}
			err := UnmarshalJson(spec, p)
			if err == nil {
				want, err = p.VectorRole(spec.Role)
			}
			got, gotErr := PipelineRole(spec)
			assert.Equal(t, err != nil, gotErr != nil, "full decode error: %v, PipelineRole error: %v", err, gotErr)
			assert.Equal(t, want, got)
		})
	}
}
