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
			name:    "pin rejects an unclassified type",
			sources: sources("exec"),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAggregator),
			wantErr: "unsupported source type: exec",
		},
		{
			name:    "pin to agent rejects an aggregator only type",
			sources: sources(kubernetesEventsType),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAgent),
			wantErr: "source a has type kubernetes_events, which does not run in the agent role",
		},
		{
			name:    "pin to aggregator rejects a host source",
			sources: sources(KubernetesLogsType),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAggregator),
			wantErr: "source a has type kubernetes_logs, which does not run in the aggregator role",
		},
		{
			name:    "pin to aggregator accepts a network scraper",
			sources: sources(PostgreSQLMetricsType),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAggregator),
			want:    vectorv1alpha1.VectorPipelineRoleAggregator,
		},
		{
			name:    "pin reports the first bad source by name",
			sources: sources(KafkaType, SyslogType, AMQPType),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAgent),
			wantErr: "source a has type kafka, which does not run in the agent role",
		},
		{
			name:    "pin to agent rejects a mix with an aggregator only type",
			sources: sources(KubernetesLogsType, KafkaType),
			pinned:  role(vectorv1alpha1.VectorPipelineRoleAgent),
			wantErr: "source b has type kafka, which does not run in the agent role",
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

// The host sources are the only agent types an aggregator cannot run, which is the rule the
// docs state for a pinned aggregator.
func TestAgentOnlyTypesAreHostSources(t *testing.T) {
	var agentOnly []string
	for name := range agentTypes {
		if !isAggregator(name) {
			agentOnly = append(agentOnly, name)
		}
	}
	assert.ElementsMatch(t, []string{DockerLogsType, FileType, HostMetricsType, JournaldType, KubernetesLogsType}, agentOnly)
}
