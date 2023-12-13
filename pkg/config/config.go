/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/pkg/pipeline"
	"github.com/kaasops/vector-operator/pkg/utils/k8s"
	"github.com/kaasops/vector-operator/pkg/vector/vectoragent"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/labels"
	goyaml "sigs.k8s.io/yaml"
)

var (
	ErrNotAllowedSourceType   error = errors.New("type kubernetes_logs only allowed")
	ErrClusterScopeNotAllowed error = errors.New("logs from external namespace not allowed")
	ErrInvalidSelector        error = errors.New("invalid selector")
)

func New(vector *vectorv1alpha1.Vector) *VectorConfig {
	sources := make(map[string]*Source)
	transforms := make(map[string]*Transform)
	sinks := make(map[string]*Sink)

	api := &ApiSpec{
		Address:    "0.0.0.0:" + strconv.Itoa(vectoragent.ApiPort),
		Enabled:    vector.Spec.Agent.Api.Enabled,
		Playground: vector.Spec.Agent.Api.Playground,
	}

	return &VectorConfig{
		DataDir: "/vector-data-dir",
		Api:     api,
		PipelineConfig: PipelineConfig{
			Sources:    sources,
			Transforms: transforms,
			Sinks:      sinks,
		},
	}
}

func BuildByteConfig(vaCtrl *vectoragent.Controller, pipelines ...pipeline.Pipeline) ([]byte, error) {
	config, err := BuildConfig(vaCtrl, pipelines...)
	if err != nil {
		return nil, err
	}
	yaml_byte, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	json_byte, err := goyaml.YAMLToJSON(yaml_byte)
	if err != nil {
		return nil, err
	}
	return json_byte, nil
}

func BuildConfig(vaCtrl *vectoragent.Controller, pipelines ...pipeline.Pipeline) (*VectorConfig, error) {
	config := New(vaCtrl.Vector)

	for _, pipeline := range pipelines {
		p := &PipelineConfig{}
		if err := UnmarshalJson(pipeline.GetSpec(), p); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pipeline %s: %w", pipeline.GetName(), err)
		}
		for k, v := range p.Sources {
			// Validate source
			if _, ok := pipeline.(*vectorv1alpha1.VectorPipeline); ok {
				if v.Type != KubernetesSourceType {
					return nil, ErrNotAllowedSourceType
				}
				_, err := labels.Parse(v.ExtraLabelSelector)
				if err != nil {
					return nil, ErrInvalidSelector
				}
				_, err = labels.Parse(v.ExtraNamespaceLabelSelector)
				if err != nil {
					return nil, ErrInvalidSelector
				}
				if v.ExtraNamespaceLabelSelector == "" {
					v.ExtraNamespaceLabelSelector = k8s.NamespaceNameToLabel(pipeline.GetNamespace())
				}
				if v.ExtraNamespaceLabelSelector != k8s.NamespaceNameToLabel(pipeline.GetNamespace()) {
					return nil, ErrClusterScopeNotAllowed
				}
			}
			if v.Type == KubernetesSourceType && vaCtrl.Vector.Spec.UseApiServerCache {
				v.UseApiServerCache = true
			}
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
			config.Sources[v.Name] = v
		}
		for k, v := range p.Transforms {
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
			for i, inputName := range v.Inputs {
				v.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
			}
			config.Transforms[v.Name] = v
		}
		for k, v := range p.Sinks {
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
			for i, inputName := range v.Inputs {
				v.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
			}
			config.Sinks[v.Name] = v
		}
	}

	// Add exporter pipeline
	if vaCtrl.Vector.Spec.Agent.InternalMetrics && !isExporterSinkExists(config.Sinks) {
		config.Sources[DefaultInternalMetricsSourceName] = defaultInternalMetricsSource
		config.Sinks[DefaultInternalMetricsSinkName] = defaultInternalMetricsSink
	}

	if len(config.Sources) == 0 && len(config.Sinks) == 0 {
		config.PipelineConfig = defaultPipelineConfig
	}

	return config, nil
}

func UnmarshalJson(spec vectorv1alpha1.VectorPipelineSpec, p *PipelineConfig) error {
	b, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	pipeline_ := &pipelineConfig_{}
	if err := json.Unmarshal(b, pipeline_); err != nil {
		return err
	}
	if err := mapstructure.Decode(pipeline_.Sources, &p.Sources); err != nil {
		return err
	}
	if err := mapstructure.Decode(pipeline_.Transforms, &p.Transforms); err != nil {
		return err
	}
	if err := mapstructure.Decode(pipeline_.Sinks, &p.Sinks); err != nil {
		return err
	}
	return nil
}

func isExporterSinkExists(sinks map[string]*Sink) bool {
	for _, sink := range sinks {
		if sink.Type == InternalMetricsSinkType {
			return true
		}
	}
	return false
}
