package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func role(r vectorv1alpha1.VectorPipelineRole) *vectorv1alpha1.VectorPipelineRole {
	return &r
}

func sources(types ...string) map[string]*Source {
	m := make(map[string]*Source, len(types))
	for i, t := range types {
		m[string(rune('a'+i))] = &Source{Type: t}
	}
	return m
}

func TestVectorRole(t *testing.T) {
	tests := []struct {
		name    string
		sources map[string]*Source
		pinned  *vectorv1alpha1.VectorPipelineRole
		want    vectorv1alpha1.VectorPipelineRole
		wantErr string
	}{
		{
			name:    "infers agent",
			sources: sources(KubernetesLogsType, JournaldType),
			want:    vectorv1alpha1.VectorPipelineRoleAgent,
		},
		{
			name:    "infers aggregator",
			sources: sources(KafkaType, SyslogType),
			want:    vectorv1alpha1.VectorPipelineRoleAggregator,
		},
		{
			// Every agent type also counts as an aggregator (the fallthrough in VectorRole),
			// so any mix of the two lands on aggregator.
			name:    "mixed types infer aggregator",
			sources: sources(KubernetesLogsType, KafkaType),
			want:    vectorv1alpha1.VectorPipelineRoleAggregator,
		},
		{
			name:    "rejects unclassified type",
			sources: sources("brand_new_source"),
			wantErr: "unsupported source type: brand_new_source",
		},
		{
			name:    "pin wins over inference",
			sources: sources(PrometheusRemoteWriteType),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAggregator),
			want:    vectorv1alpha1.VectorPipelineRoleAggregator,
		},
		{
			name:    "pin accepts an unclassified type",
			sources: sources("brand_new_source"),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAggregator),
			want:    vectorv1alpha1.VectorPipelineRoleAggregator,
		},
		{
			name:    "pin wins over a mixed inference",
			sources: sources(KubernetesLogsType, KafkaType),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAgent),
			want:    vectorv1alpha1.VectorPipelineRoleAgent,
		},
		{
			name:    "pin still needs a source",
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAggregator),
			wantErr: "sources list is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &PipelineConfig{Sources: tt.sources}
			got, err := c.VectorRole(tt.pinned)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestValidateAggregatorSources(t *testing.T) {
	tests := []struct {
		name    string
		sources map[string]*Source
		wantErr bool
	}{
		{name: "network source", sources: sources(KafkaType, SyslogType)},
		{
			// The motivating case from #218: an agent-classified listener that a namespaced
			// pipeline must still be able to pin to an aggregator.
			name:    "prometheus listeners",
			sources: sources(PrometheusRemoteWriteType, PrometheusPushgatewayType, PrometheusScrapeType),
		},
		{name: "kubernetes events", sources: sources(kubernetesEventsType)},
		{name: "unclassified source", sources: sources("brand_new_source")},
		{name: "kubernetes logs", sources: sources(KubernetesLogsType), wantErr: true},
		{name: "file", sources: sources(FileType), wantErr: true},
		{name: "journald", sources: sources(JournaldType), wantErr: true},
		{name: "docker logs", sources: sources(DockerLogsType), wantErr: true},
		{name: "host metrics", sources: sources(HostMetricsType), wantErr: true},
		{name: "host source mixed with a network source", sources: sources(KafkaType, KubernetesLogsType), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&PipelineConfig{Sources: tt.sources}).ValidateAggregatorSources()
			if tt.wantErr {
				require.ErrorIs(t, err, ErrHostSourceNotAllowed)
				return
			}
			require.NoError(t, err)
		})
	}
}

// A source type in both maps resolves to agent, so an aggregator can only ever receive it
// through an explicit pin.
func TestVectorRolePinReachesAggregatorForDualRoleSource(t *testing.T) {
	c := &PipelineConfig{Sources: sources(OpenTelemetryType)}

	inferred, err := c.VectorRole(nil)
	require.NoError(t, err)
	assert.Equal(t, vectorv1alpha1.VectorPipelineRoleAgent, *inferred)

	pinned, err := c.VectorRole(role(vectorv1alpha1.VectorPipelineRoleAggregator))
	require.NoError(t, err)
	assert.Equal(t, vectorv1alpha1.VectorPipelineRoleAggregator, *pinned)
}
