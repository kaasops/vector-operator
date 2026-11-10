package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func portOrderTestConfig(t *testing.T) *VectorConfig {
	t.Helper()
	c := &VectorConfig{}
	c.internal.servicePort = make(map[string]*ServicePort)
	for _, p := range []*ServicePort{
		{SourceName: "events_b", PipelineName: "b", ServiceName: "svc", Port: 42001, Protocol: corev1.ProtocolTCP, IsKubernetesEvents: true},
		{SourceName: "metrics", PipelineName: "a", ServiceName: "svc", Port: 9090, Protocol: corev1.ProtocolTCP},
		{SourceName: "syslog_udp", PipelineName: "a", ServiceName: "svc", Port: 514, Protocol: corev1.ProtocolUDP},
		{SourceName: "syslog_tcp", PipelineName: "a", ServiceName: "svc", Port: 514, Protocol: corev1.ProtocolTCP},
		{SourceName: "events_a", PipelineName: "a", ServiceName: "svc", Port: 42000, Protocol: corev1.ProtocolTCP, IsKubernetesEvents: true},
	} {
		require.NoError(t, c.internal.addServicePort(p))
	}
	return c
}

// Map order is random, so each check repeats until an unsorted result would show.
func TestGetSourcesServicePortsIsOrdered(t *testing.T) {
	c := portOrderTestConfig(t)
	for range 100 {
		groups := c.GetSourcesServicePorts()
		require.Len(t, groups, 2)
		var names []string
		for _, sp := range groups[SPGroup{PipelineName: "a", ServiceName: "svc"}] {
			names = append(names, sp.SourceName)
		}
		assert.Equal(t, []string{"syslog_tcp", "syslog_udp", "metrics", "events_a"}, names)
	}
}

func TestGetEventCollectorConfigIsOrdered(t *testing.T) {
	c := portOrderTestConfig(t)
	for range 100 {
		cfg := c.GetEventCollectorConfig("vector")
		require.NotNil(t, cfg)
		var ports []string
		for _, r := range cfg.Receivers {
			ports = append(ports, r.Port)
		}
		assert.Equal(t, []string{"42000", "42001"}, ports)
	}
}
