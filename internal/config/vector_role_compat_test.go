package config

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// Source type lists as of v0.6.0, kept separate from the current lists in
// vector_source_types.go. A change to the current lists must not change the role of a
// pipeline that has no spec.role and uses only these types.
var (
	v060AgentTypes = []string{
		"apache_metrics", "demo_logs", "dnstap", "docker_logs", "eventstoredb_metrics", "file",
		"host_metrics", "http_client", "internal_logs", "internal_metrics", "journald",
		"kubernetes_logs", "mongodb_metrics", "nginx_metrics", "opentelemetry",
		"postgresql_metrics", "prometheus_pushgateway", "prometheus_remote_write", "prometheus_scrape",
	}
	v060AggregatorTypes = []string{
		"amqp", "aws_kinesis_firehose", "aws_s3", "aws_sqs", "datadog_agent", "fluent",
		"gcp_pubsub", "heroku_logs", "http_client", "http_server", "internal_logs",
		"internal_metrics", "kafka", "kubernetes_events", "logstash", "nats", "opentelemetry",
		"pulsar", "redis", "socket", "splunk_hec", "statsd", "syslog", "vector",
	}
)

// v060Role is the v0.6.0 inference: agent if every source has an agent type, otherwise
// aggregator, and invalid without sources or with an unknown type.
func v060Role(types []string) string {
	if len(types) == 0 {
		return "invalid"
	}
	role := vectorv1alpha1.VectorPipelineRoleAgent
	for _, t := range types {
		switch {
		case slices.Contains(v060AgentTypes, t):
		case slices.Contains(v060AggregatorTypes, t):
			role = vectorv1alpha1.VectorPipelineRoleAggregator
		default:
			return "invalid"
		}
	}
	return string(role)
}

// Without spec.role, every set of up to three source types resolves as on v0.6.0. The
// types come from the v0.6.0 lists, plus one unknown type.
func TestVectorRoleWithoutPinMatchesV060(t *testing.T) {
	types := slices.Concat(v060AgentTypes, v060AggregatorTypes, []string{"not_a_source"})
	slices.Sort(types)
	types = slices.Compact(types)

	check := func(set ...string) {
		got := "invalid"
		if r, err := (&PipelineConfig{Sources: sources(set...)}).VectorRole(nil); err == nil {
			got = string(*r)
		}
		require.Equal(t, v060Role(set), got, "source types %v", set)
	}
	check()
	for i := range types {
		check(types[i])
		for j := i + 1; j < len(types); j++ {
			check(types[i], types[j])
			for k := j + 1; k < len(types); k++ {
				check(types[i], types[j], types[k])
			}
		}
	}
}
