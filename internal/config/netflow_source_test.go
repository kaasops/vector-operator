package config

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func TestExtractProtocol(t *testing.T) {
	cases := []struct {
		name       string
		sourceType string
		opts       map[string]any
		want       corev1.Protocol
	}{
		{"netflow is UDP without a mode option", NetflowType, map[string]any{"address": "0.0.0.0:2055"}, corev1.ProtocolUDP},
		{"syslog in udp mode", SyslogType, map[string]any{"mode": "udp"}, corev1.ProtocolUDP},
		{"syslog in tcp mode", SyslogType, map[string]any{"mode": "tcp"}, corev1.ProtocolTCP},
		{"vector source has no mode", VectorType, map[string]any{"address": "0.0.0.0:9000"}, corev1.ProtocolTCP},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractProtocol(c.sourceType, c.opts); got != c.want {
				t.Fatalf("extractProtocol(%q, %v) = %s, want %s", c.sourceType, c.opts, got, c.want)
			}
		})
	}
}

func TestNetflowPipelineIsAnAggregatorPipeline(t *testing.T) {
	p := &PipelineConfig{Sources: map[string]*Source{"nf": {Type: NetflowType}}}
	role, err := p.VectorRole()
	if err != nil {
		t.Fatalf("VectorRole() returned an error: %v", err)
	}
	if *role != vectorv1alpha1.VectorPipelineRoleAggregator {
		t.Fatalf("VectorRole() = %s, want %s", *role, vectorv1alpha1.VectorPipelineRoleAggregator)
	}
}
