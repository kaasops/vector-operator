package config

import (
	"fmt"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/labels"
	goyaml "sigs.k8s.io/yaml"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

const (
	AgentApiPort = 8686
)

func BuildAgentConfig(p VectorConfigParams, pipelines ...pipeline.Pipeline) (*VectorConfig, []byte, error) {
	cfg, err := buildAgentConfig(p, pipelines...)
	if err != nil {
		return nil, nil, err
	}
	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, nil, err
	}
	jsonBytes, err := goyaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, nil, err
	}
	return cfg, jsonBytes, nil
}

func buildAgentConfig(params VectorConfigParams, pipelines ...pipeline.Pipeline) (*VectorConfig, error) {
	cfg := newVectorConfig(params)

	for _, pipeline := range pipelines {
		p := &PipelineConfig{}
		if err := UnmarshalJson(pipeline.GetSpec(), p); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pipeline %s: %w", pipeline.GetName(), err)
		}
		for k, v := range p.Sources {
			// Validate source
			if _, ok := pipeline.(*vectorv1alpha1.VectorPipeline); ok {
				if v.Type != KubernetesLogsType {
					return nil, ErrNotAllowedSourceType
				}
				_, err := labels.Parse(v.ExtraLabelSelector)
				if err != nil {
					return nil, fmt.Errorf("invalid pod selector for source %s: %w", k, err)
				}
				_, err = labels.Parse(v.ExtraNamespaceLabelSelector)
				if err != nil {
					return nil, fmt.Errorf("invalid namespace selector for source %s: %w", k, err)
				}
				if v.ExtraNamespaceLabelSelector == "" {
					v.ExtraNamespaceLabelSelector = k8s.NamespaceNameToLabel(pipeline.GetNamespace())
				}
				if v.ExtraNamespaceLabelSelector != k8s.NamespaceNameToLabel(pipeline.GetNamespace()) {
					return nil, ErrClusterScopeNotAllowed
				}
			}
			if v.Type == KubernetesLogsType && params.UseApiServerCache {
				v.UseApiServerCache = true
			}
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
			cfg.Sources[v.Name] = v
		}
		for k, v := range p.Transforms {
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
			for i, inputName := range v.Inputs {
				v.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
			}
			cfg.Transforms[v.Name] = v
		}
		for k, v := range p.Sinks {
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
			for i, inputName := range v.Inputs {
				v.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
			}
			cfg.Sinks[v.Name] = v
		}
	}

	// Add exporter pipeline
	if params.InternalMetrics && !isExporterSinkExists(cfg.Sinks) {
		cfg.Sources[DefaultInternalMetricsSourceName] = defaultInternalMetricsSource
		cfg.Sinks[DefaultInternalMetricsSinkName] = defaultInternalMetricsSink
	}

	if len(cfg.Sources) == 0 && len(cfg.Sinks) == 0 {
		cfg.PipelineConfig = defaultAgentPipelineConfig
	}

	return cfg, nil
}
