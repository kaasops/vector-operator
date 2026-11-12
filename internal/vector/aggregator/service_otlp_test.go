package aggregator

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

func TestOTLPServicePortNames(t *testing.T) {
	for _, tt := range []struct {
		name    string
		sources string
		want    map[int32]string
	}{
		{
			"short",
			`{"otel":{"type":"opentelemetry","grpc":{"address":":4317"},"http":{"address":":4318"}}}`,
			map[int32]string{4317: "otel-grpc", 4318: "otel-http"},
		},
		{
			"opentelemetry",
			`{"opentelemetry":{"type":"opentelemetry","grpc":{"address":":4317"},"http":{"address":":4318"}}}`,
			map[int32]string{4317: "opentelemetry-grpc", 4318: "opentelemetry-http"},
		},
		{
			"long names with a shared prefix",
			`{"very_long_source_a":{"type":"opentelemetry","grpc":{"address":":4317"},"http":{"address":":4318"}},"very_long_source_b":{"type":"opentelemetry","grpc":{"address":":5317"},"http":{"address":":5318"}}}`,
			map[int32]string{4317: "very-long-source-a-grpc", 4318: "very-long-source-a-http", 5317: "very-long-source-b-grpc", 5318: "very-long-source-b-http"},
		},
		{
			"63 character names",
			fmt.Sprintf(`{"%s":{"type":"opentelemetry","grpc":{"address":":4317"},"http":{"address":":4318"}}}`, strings.Repeat("a", 58)),
			map[int32]string{4317: strings.Repeat("a", 58) + "-grpc", 4318: strings.Repeat("a", 58) + "-http"},
		},
		{
			"64 character names",
			fmt.Sprintf(`{"%s":{"type":"opentelemetry","grpc":{"address":":4317"},"http":{"address":":4318"}}}`, strings.Repeat("a", 59)),
			map[int32]string{4317: "grpc-4317", 4318: "http-4318"},
		},
		{
			"numeric source",
			`{"123":{"type":"opentelemetry","grpc":{"address":":65535"},"http":{"address":":4318"}}}`,
			map[int32]string{65535: "123-grpc", 4318: "123-http"},
		},
		{
			"invalid port name characters",
			`{"otel.input":{"type":"opentelemetry","grpc":{"address":":4317"}},"otel--input":{"type":"opentelemetry","http":{"address":":4318"}}}`,
			map[int32]string{4317: "grpc-4317", 4318: "otel-input-http"},
		},
		{
			"same normalized name",
			`{"otel_in":{"type":"opentelemetry","grpc":{"address":":4317"}},"otel-in":{"type":"opentelemetry","grpc":{"address":":5317"}}}`,
			map[int32]string{4317: "otel-in-grpc", 5317: "grpc-5317"},
		},
		{
			"keep existing names",
			`{"otel":{"type":"opentelemetry","grpc":{"address":":4317"}},"otel_grpc":{"type":"socket","address":":6000"},"grpc_4317":{"type":"socket","address":":6001"},"grpc_4317_1":{"type":"socket","address":":6002"}}`,
			map[int32]string{4317: "grpc-4317-2", 6000: "otel-grpc", 6001: "grpc-4317", 6002: "grpc-4317-1"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			p := &vectorv1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "logs", Namespace: "test"},
				Spec: vectorv1alpha1.VectorPipelineSpec{
					Role:    ptr.To(vectorv1alpha1.VectorPipelineRoleAggregator),
					Sources: &runtime.RawExtension{Raw: []byte(tt.sources)},
				},
			}
			cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{AggregatorName: "test"}, p)
			g.Expect(err).To(Succeed())
			ctrl := NewController(&vectorv1alpha1.VectorAggregator{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
			}, nil, nil)
			ctrl.Config = cfg
			services, err := ctrl.createVectorAggregatorServices()
			g.Expect(err).To(Succeed())
			g.Expect(services).To(HaveLen(1))
			g.Expect(services[0].Name).To(Equal("test-aggregator-logs"))
			got := make(map[int32]string)
			used := make(map[string]bool)
			for _, p := range services[0].Spec.Ports {
				g.Expect(validation.IsDNS1123Label(p.Name)).To(BeEmpty())
				g.Expect(used[p.Name]).To(BeFalse(), "duplicate name %s", p.Name)
				used[p.Name] = true
				g.Expect(p.Protocol).To(Equal(corev1.ProtocolTCP))
				g.Expect(p.TargetPort).To(Equal(intstr.FromInt32(p.Port)))
				got[p.Port] = p.Name
			}
			g.Expect(got).To(Equal(tt.want))

			// Service metadata must not change the Vector source options.
			var original map[string]map[string]any
			g.Expect(json.Unmarshal([]byte(tt.sources), &original)).To(Succeed())
			for name, source := range original {
				delete(source, "type")
				g.Expect(cfg.Sources["test-logs-"+name].Options).To(Equal(source))
			}
		})
	}
}

// The config hands ports over sorted; the names of OTLP ports must not depend on the
// order either, so a port never swaps its name with a sibling.
func TestOTLPServicePortNamesIgnoreOrder(t *testing.T) {
	g := NewWithT(t)
	list := []*config.ServicePort{
		{SourceName: "udp_input", Port: 4317, Protocol: corev1.ProtocolUDP},
		{SourceName: "otel", Endpoint: "http", Port: 4318, Protocol: corev1.ProtocolTCP},
		{SourceName: "tcp_input", Port: 4000, Protocol: corev1.ProtocolTCP},
		{SourceName: "otel", Endpoint: "grpc", Port: 4317, Protocol: corev1.ProtocolTCP},
		// Both kebab to "otel-a-grpc"; the lower port keeps it.
		{SourceName: "otel-a", Endpoint: "grpc", Port: 6317, Protocol: corev1.ProtocolTCP},
		{SourceName: "otel_a", Endpoint: "grpc", Port: 5317, Protocol: corev1.ProtocolTCP},
	}
	reversed := slices.Clone(list)
	slices.Reverse(reversed)
	byPort := func(ports []corev1.ServicePort) []corev1.ServicePort {
		return slices.SortedFunc(slices.Values(ports), func(a, b corev1.ServicePort) int {
			return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.Protocol, b.Protocol))
		})
	}
	want := []corev1.ServicePort{
		{Name: "tcp-input", Protocol: corev1.ProtocolTCP, Port: 4000, TargetPort: intstr.FromInt32(4000)},
		{Name: "otel-grpc", Protocol: corev1.ProtocolTCP, Port: 4317, TargetPort: intstr.FromInt32(4317)},
		{Name: "udp-input", Protocol: corev1.ProtocolUDP, Port: 4317, TargetPort: intstr.FromInt32(4317)},
		{Name: "otel-http", Protocol: corev1.ProtocolTCP, Port: 4318, TargetPort: intstr.FromInt32(4318)},
		{Name: "otel-a-grpc", Protocol: corev1.ProtocolTCP, Port: 5317, TargetPort: intstr.FromInt32(5317)},
		{Name: "grpc-6317", Protocol: corev1.ProtocolTCP, Port: 6317, TargetPort: intstr.FromInt32(6317)},
	}
	g.Expect(byPort(buildServicePorts(list))).To(Equal(want))
	g.Expect(byPort(buildServicePorts(reversed))).To(Equal(want))
}

func TestEnsureOTLPService(t *testing.T) {
	for _, cluster := range []bool{false, true} {
		name := "VectorPipeline"
		if cluster {
			name = "ClusterVectorPipeline"
		}
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			spec := vectorv1alpha1.VectorPipelineSpec{
				Role:    ptr.To(vectorv1alpha1.VectorPipelineRoleAggregator),
				Sources: &runtime.RawExtension{Raw: []byte(`{"otel":{"type":"opentelemetry","grpc":{"address":"0.0.0.0:4317"},"http":{"address":"[::]:4318"}}}`)},
			}
			var p pipeline.Pipeline = &vectorv1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "logs", Namespace: "test"}, Spec: spec,
			}
			var agg Aggregator = &vectorv1alpha1.VectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}}
			if cluster {
				p = &vectorv1alpha1.ClusterVectorPipeline{ObjectMeta: metav1.ObjectMeta{Name: "logs"}, Spec: spec}
				agg = &vectorv1alpha1.ClusterVectorAggregator{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Spec:       vectorv1alpha1.ClusterVectorAggregatorSpec{ResourceNamespace: "test"},
				}
			}
			cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{AggregatorName: "test"}, p)
			g.Expect(err).To(Succeed())
			ctrl := NewController(agg, newFakeClient(g), nil)
			ctrl.Config = cfg
			ctx := context.Background()
			g.Expect(ctrl.ensureVectorAggregatorService(ctx)).To(Succeed())
			svc := &corev1.Service{}
			key := types.NamespacedName{Name: "test-aggregator-logs", Namespace: "test"}
			g.Expect(ctrl.Get(ctx, key, svc)).To(Succeed())
			g.Expect(svc.Spec.Ports).To(Equal([]corev1.ServicePort{
				{Name: "otel-grpc", Protocol: corev1.ProtocolTCP, Port: 4317, TargetPort: intstr.FromInt32(4317)},
				{Name: "otel-http", Protocol: corev1.ProtocolTCP, Port: 4318, TargetPort: intstr.FromInt32(4318)},
			}))
		})
	}
}
