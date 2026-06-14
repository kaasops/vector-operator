package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func TestApplyComponentTemplatesRendersSink(t *testing.T) {
	t.Parallel()

	p := &PipelineConfig{
		Sinks: map[string]*Sink{
			"stdout": {
				Inputs: []string{"filter"},
				Options: map[string]any{
					sinkTemplateKey: map[string]any{
						"name": "json-console",
						"params": map[string]any{
							"codec": "json",
						},
					},
				},
			},
		},
	}
	tpls := &ComponentTemplatesConfig{
		Sinks: map[string]string{
			"json-console": `type: console
encoding:
  codec: "{{ .codec }}"`,
		},
	}

	require.NoError(t, applyComponentTemplates(p, tpls))

	sink := p.Sinks["stdout"]
	require.NotNil(t, sink)
	require.Equal(t, "console", sink.Type)
	require.Equal(t, []string{"filter"}, sink.Inputs)
	require.Equal(t, "json", sink.Options["encoding"].(map[string]any)["codec"])
	_, hasTemplateRef := sink.Options[sinkTemplateKey]
	require.False(t, hasTemplateRef)
}

func TestApplyComponentTemplatesRendersOpenTelemetryHTTPSink(t *testing.T) {
	t.Parallel()

	p := &PipelineConfig{
		Sinks: map[string]*Sink{
			"otel-http": {
				Inputs: []string{"transform"},
				Options: map[string]any{
					sinkTemplateKey: map[string]any{
						"name": "otel-http",
						"params": map[string]any{
							"uri":       "http://localhost:8889/write",
							"maxEvents": 100,
						},
					},
				},
			},
		},
	}
	tpls := &ComponentTemplatesConfig{
		Sinks: map[string]string{
			"otel-http": `type: opentelemetry
protocol: http
uri: "{{ .uri }}"
method: post
encoding:
  codec: json
batch:
  max_events: {{ .maxEvents }}`,
		},
	}

	require.NoError(t, applyComponentTemplates(p, tpls))

	sink := p.Sinks["otel-http"]
	require.NotNil(t, sink)
	require.Equal(t, "opentelemetry", sink.Type)
	require.Equal(t, []string{"transform"}, sink.Inputs)
	require.Equal(t, "http", sink.Options["protocol"])
	require.Equal(t, "http://localhost:8889/write", sink.Options["uri"])
	require.Equal(t, "post", sink.Options["method"])
	require.Equal(t, "json", sink.Options["encoding"].(map[string]any)["codec"])
	require.Equal(t, float64(100), sink.Options["batch"].(map[string]any)["max_events"])
	_, hasTemplateRef := sink.Options[sinkTemplateKey]
	require.False(t, hasTemplateRef)
}

func TestApplyComponentTemplatesMissingTemplateName(t *testing.T) {
	t.Parallel()

	p := &PipelineConfig{
		Sinks: map[string]*Sink{
			"console": {
				Options: map[string]any{
					sinkTemplateKey: map[string]any{
						"name": "missing",
					},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		err := applyComponentTemplates(p, nil)
		require.ErrorIs(t, err, ErrComponentTemplatesNotConfigured)
	})
}

func TestApplyComponentTemplatesUnknownTemplateName(t *testing.T) {
	t.Parallel()

	p := &PipelineConfig{
		Sinks: map[string]*Sink{
			"console": {
				Options: map[string]any{
					sinkTemplateKey: map[string]any{
						"name": "missing",
					},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		err := applyComponentTemplates(p, &ComponentTemplatesConfig{})
		require.ErrorIs(t, err, ErrComponentTemplateNotFound)
	})
}

func TestApplyComponentTemplatesReferenceWithoutConfig(t *testing.T) {
	t.Parallel()

	t.Run("sink", func(t *testing.T) {
		t.Parallel()

		p := &PipelineConfig{
			Sinks: map[string]*Sink{
				"stdout": {
					Options: map[string]any{
						sinkTemplateKey: map[string]any{
							"name": "json-console",
						},
					},
				},
			},
		}

		err := applyComponentTemplates(p, nil)
		require.ErrorIs(t, err, ErrComponentTemplatesNotConfigured)
	})

	t.Run("source", func(t *testing.T) {
		t.Parallel()

		p := &PipelineConfig{
			Sources: map[string]*Source{
				"logs": {
					Options: map[string]any{
						sourceTemplateKey: map[string]any{
							"name": "k8s-logs",
						},
					},
				},
			},
		}

		err := applyComponentTemplates(p, nil)
		require.ErrorIs(t, err, ErrComponentTemplatesNotConfigured)
	})

	t.Run("transform", func(t *testing.T) {
		t.Parallel()

		p := &PipelineConfig{
			Transforms: map[string]*Transform{
				"filter": {
					Options: map[string]any{
						transformTemplateKey: map[string]any{
							"name": "drop-debug",
						},
					},
				},
			},
		}

		err := applyComponentTemplates(p, nil)
		require.ErrorIs(t, err, ErrComponentTemplatesNotConfigured)
	})
}

func TestBuildAgentConfigTemplateReferenceWithoutConfig(t *testing.T) {
	t.Parallel()

	pipeline := &vectorv1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "logs",
			Namespace: "team-a",
		},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Sinks: rawTemplateExtension(t, `{
				"stdout": {
					"inputs": ["kubernetes_logs"],
					"sinkTemplate": {
						"name": "json-console"
					}
				}
			}`),
		},
	}

	_, _, err := BuildAgentConfig(AgentConfigParamsFromVector(&vectorv1alpha1.Vector{
		Spec: vectorv1alpha1.VectorSpec{
			Agent: &vectorv1alpha1.VectorAgent{},
		},
	}), pipeline)
	require.ErrorIs(t, err, ErrComponentTemplatesNotConfigured)
}

func TestApplyComponentTemplatesMissingParam(t *testing.T) {
	t.Parallel()

	p := &PipelineConfig{
		Sinks: map[string]*Sink{
			"console": {
				Options: map[string]any{
					sinkTemplateKey: map[string]any{
						"name": "json-console",
					},
				},
			},
		},
	}

	err := applyComponentTemplates(p, &ComponentTemplatesConfig{
		Sinks: map[string]string{
			"json-console": `type: console
encoding:
  codec: "{{ .codec }}"`,
		},
	})
	require.ErrorIs(t, err, ErrComponentTemplateRenderFailed)
}

func TestApplyComponentTemplatesTypeAndReferenceConflict(t *testing.T) {
	t.Parallel()

	p := &PipelineConfig{
		Sinks: map[string]*Sink{
			"console": {
				Type: "console",
				Options: map[string]any{
					sinkTemplateKey: map[string]any{
						"name": "json-console",
					},
				},
			},
		},
	}

	err := applyComponentTemplates(p, &ComponentTemplatesConfig{Sinks: map[string]string{"json-console": "type: console"}})
	require.ErrorIs(t, err, ErrComponentTemplateTypeConflict)
}

func TestApplyComponentTemplatesNoReferenceNoOp(t *testing.T) {
	t.Parallel()

	p := &PipelineConfig{
		Sources: map[string]*Source{
			"logs": {Type: KubernetesLogsType},
		},
		Sinks: map[string]*Sink{
			"console": {
				Type:   "console",
				Inputs: []string{"logs"},
				Options: map[string]any{
					"encoding": map[string]any{"codec": "json"},
				},
			},
		},
	}
	original := p.Sinks["console"]

	require.NoError(t, applyComponentTemplates(p, nil))
	require.Same(t, original, p.Sinks["console"])
}

func TestBuildAgentConfigAppliesTemplatedSource(t *testing.T) {
	t.Parallel()

	pipeline := &vectorv1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "logs",
			Namespace: "team-a",
		},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Sources: rawTemplateExtension(t, `{
				"kubernetes_logs": {
					"sourceTemplate": {
						"name": "k8s-logs",
						"params": {
							"readFrom": "end"
						}
					}
				}
			}`),
		},
	}

	cfg, _, err := BuildAgentConfig(AgentConfigParamsFromVector(&vectorv1alpha1.Vector{
		Spec: vectorv1alpha1.VectorSpec{
			Agent: &vectorv1alpha1.VectorAgent{
				VectorCommon: vectorv1alpha1.VectorCommon{
					ComponentTemplates: &vectorv1alpha1.ComponentTemplates{
						Sources: map[string]string{
							"k8s-logs": `type: kubernetes_logs
read_from: "{{ .readFrom }}"`,
						},
					},
				},
			},
		},
	}), pipeline)
	require.NoError(t, err)

	source := cfg.Sources["team-a-logs-kubernetes_logs"]
	require.NotNil(t, source)
	require.Equal(t, KubernetesLogsType, source.Type)
	require.Equal(t, "end", source.Options["read_from"])
	require.Equal(t, "kubernetes.io/metadata.name=team-a", source.ExtraNamespaceLabelSelector)
}

func rawTemplateExtension(t *testing.T, raw string) *runtime.RawExtension {
	t.Helper()
	return &runtime.RawExtension{Raw: []byte(raw)}
}
