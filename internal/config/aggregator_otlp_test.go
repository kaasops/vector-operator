package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/common"
)

func portTestPipeline(t *testing.T, sources map[string]any, role *vectorv1alpha1.VectorPipelineRole) *vectorv1alpha1.VectorPipeline {
	t.Helper()
	raw, err := json.Marshal(sources)
	require.NoError(t, err)
	p := &vectorv1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name: "logs", Namespace: "test",
			Annotations: map[string]string{common.AnnotationServiceName: "receivers"},
		},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Role: role, Sources: &runtime.RawExtension{Raw: raw},
		},
	}
	// Status is not the explicit role: inferred aggregators have this value too.
	p.SetRole(ptr.To(vectorv1alpha1.VectorPipelineRoleAggregator))
	return p
}

func TestOTLPServicePorts(t *testing.T) {
	for _, tt := range []struct {
		name       string
		role       *vectorv1alpha1.VectorPipelineRole
		sourceType string
		options    map[string]any
		want       []int32
	}{
		{"both", role(vectorv1alpha1.VectorPipelineRoleAggregator), "opentelemetry", map[string]any{"grpc": map[string]any{"address": "0.0.0.0:4317"}, "http": map[string]any{"address": "0.0.0.0:4318"}}, []int32{4317, 4318}},
		{"ipv6", role(vectorv1alpha1.VectorPipelineRoleAggregator), "opentelemetry", map[string]any{"grpc": map[string]any{"address": "[::]:4317"}, "http": map[string]any{"address": "[::1]:4318"}}, []int32{4317, 4318}},
		{"grpc only", role(vectorv1alpha1.VectorPipelineRoleAggregator), "opentelemetry", map[string]any{"grpc": map[string]any{"address": ":4317"}}, []int32{4317}},
		{"http only", role(vectorv1alpha1.VectorPipelineRoleAggregator), "opentelemetry", map[string]any{"http": map[string]any{"address": "localhost:4318"}}, []int32{4318}},
		{"no blocks", role(vectorv1alpha1.VectorPipelineRoleAggregator), "opentelemetry", map[string]any{}, nil},
		{"empty blocks", role(vectorv1alpha1.VectorPipelineRoleAggregator), "opentelemetry", map[string]any{"grpc": map[string]any{}, "http": nil}, nil},
		{"no role", nil, "opentelemetry", map[string]any{"grpc": map[string]any{"address": ":4317"}, "http": map[string]any{"address": ":4318"}}, nil},
		{"agent", role(vectorv1alpha1.VectorPipelineRoleAgent), "opentelemetry", map[string]any{"grpc": map[string]any{"address": ":4317"}, "http": map[string]any{"address": ":4318"}}, nil},
		{"other source", role(vectorv1alpha1.VectorPipelineRoleAggregator), "kafka", map[string]any{"grpc": map[string]any{"address": ":4317"}, "http": map[string]any{"address": ":4318"}}, nil},
		{"always tcp", role(vectorv1alpha1.VectorPipelineRoleAggregator), "opentelemetry", map[string]any{"mode": "udp", "grpc": map[string]any{"address": ":4317", "mode": "udp"}, "http": map[string]any{"address": ":4318", "mode": "udp"}}, []int32{4317, 4318}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.options["type"] = tt.sourceType
			p := portTestPipeline(t, map[string]any{"otel": tt.options}, tt.role)
			cfg, err := BuildAggregatorConfig(VectorConfigParams{AggregatorName: "test"}, p)
			require.NoError(t, err)
			groups := cfg.GetSourcesServicePorts()
			if len(tt.want) == 0 {
				require.Empty(t, groups)
				return
			}
			require.Len(t, groups, 1)
			ports := groups[SPGroup{PipelineName: "logs", Namespace: "test", ServiceName: "receivers"}]
			var got []int32
			for _, p := range ports {
				require.Equal(t, corev1.ProtocolTCP, p.Protocol)
				require.Equal(t, "otel", p.SourceName)
				got = append(got, p.Port)
			}
			require.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestOTLPAddressMatchesOrdinaryAddress(t *testing.T) {
	for _, tt := range []struct {
		address any
		port    *int32
		wantErr bool
	}{
		{"0.0.0.0:4317", ptr.To[int32](4317), false},
		{"[::]:4318", ptr.To[int32](4318), false},
		{"[fe80::1%eth0]:4317", ptr.To[int32](4317), false},
		{"localhost:4317", ptr.To[int32](4317), false},
		{":4317", ptr.To[int32](4317), false},
		{"no-port", nil, false}, {"::1:4317", nil, false}, {"", nil, false}, {nil, nil, false}, {4317, nil, false},
		{":", nil, true}, {":otlp", nil, true}, {":-1", nil, true}, {":65536", nil, true}, {":2147483648", nil, true},
		{":0", ptr.To[int32](0), false}, {":65535", ptr.To[int32](65535), false},
	} {
		label, err := json.Marshal(tt.address)
		require.NoError(t, err)
		t.Run(string(label), func(t *testing.T) {
			ordinary := portTestPipeline(t, map[string]any{"in": map[string]any{"type": "socket", "address": tt.address}}, nil)
			want, wantErr := BuildAggregatorConfig(VectorConfigParams{}, ordinary)
			if tt.wantErr {
				require.Error(t, wantErr)
			} else {
				require.NoError(t, wantErr)
				if tt.port == nil {
					require.Empty(t, want.GetSourcesServicePorts())
				} else {
					ports := want.GetSourcesServicePorts()[SPGroup{PipelineName: "logs", Namespace: "test", ServiceName: "receivers"}]
					require.Len(t, ports, 1)
					require.Equal(t, *tt.port, ports[0].Port)
				}
			}
			for _, endpoint := range []string{"grpc", "http"} {
				t.Run(endpoint, func(t *testing.T) {
					p := portTestPipeline(t, map[string]any{"in": map[string]any{
						"type": "opentelemetry", endpoint: map[string]any{"address": tt.address},
					}}, role(vectorv1alpha1.VectorPipelineRoleAggregator))
					got, err := BuildAggregatorConfig(VectorConfigParams{}, p)
					if wantErr != nil {
						require.EqualError(t, err, wantErr.Error())
						require.Nil(t, got)
						return
					}
					require.NoError(t, err)
					wantPorts, gotPorts := want.GetSourcesServicePorts(), got.GetSourcesServicePorts()
					require.Len(t, gotPorts, len(wantPorts))
					for group, ports := range wantPorts {
						require.Len(t, gotPorts[group], len(ports))
						require.Equal(t, ports[0].Port, gotPorts[group][0].Port)
						require.Equal(t, ports[0].Protocol, gotPorts[group][0].Protocol)
					}
				})
			}
		})
	}
}

func TestOTLPPortCollisionMatchesOrdinaryAddress(t *testing.T) {
	for _, endpoint := range []string{"grpc", "http"} {
		t.Run(endpoint, func(t *testing.T) {
			first := portTestPipeline(t, map[string]any{"in": map[string]any{"type": "socket", "address": ":4317"}}, nil)
			first.Name = "first"
			second := portTestPipeline(t, map[string]any{"in": map[string]any{"type": "socket", "address": ":4317"}}, nil)
			second.Name = "second"
			_, wantErr := BuildAggregatorConfig(VectorConfigParams{}, first, second)
			require.EqualError(t, wantErr, "duplicate port 4317/TCP in first and second")
			second = portTestPipeline(t, map[string]any{"in": map[string]any{
				"type": "opentelemetry", endpoint: map[string]any{"address": ":4317"},
			}}, role(vectorv1alpha1.VectorPipelineRoleAggregator))
			second.Name = "second"
			cfg, err := BuildAggregatorConfig(VectorConfigParams{}, first, second)
			require.EqualError(t, err, wantErr.Error())
			require.Nil(t, cfg)

			first.Spec.Sources.Raw = []byte(`{"in":{"type":"socket","address":":4317","mode":"udp"}}`)
			cfg, err = BuildAggregatorConfig(VectorConfigParams{}, first, second)
			require.NoError(t, err)
			require.Len(t, cfg.GetSourcesServicePorts(), 2)
		})
	}
}
