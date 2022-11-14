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

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"
	"github.com/mitchellh/mapstructure"
)

var (
	KubernetesSourceType = "kubernetes_logs"
	sourceDefault        = &Source{
		Name: "defaultSource",
		Type: KubernetesSourceType,
	}
	sinkDefault = &Sink{
		Name:   "defaultSink",
		Type:   "blackhole",
		Inputs: []string{"defaultSource"},
		Options: map[string]interface{}{
			"rate":                100,
			"print_interval_secs": 60,
		},
	}
)

var (
	PipelineTypeError  error = errors.New("type kubernetes_logs only allowed")
	PipelineScopeError error = errors.New("logs from external namespace not allowed")
)

type Builder struct {
	Name      string
	vaCtrl    *vectoragent.Controller
	Pipelines []pipeline.Pipeline
}

func NewBuilder(vaCtrl *vectoragent.Controller, pipelines ...pipeline.Pipeline) *Builder {
	return &Builder{
		vaCtrl:    vaCtrl,
		Pipelines: pipelines,
	}
}

func (b *Builder) GetByteConfig() ([]byte, error) {
	config, err := b.generateVectorConfig()
	if err != nil {
		return nil, err
	}

	data, err := vectorConfigToByte(config)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (b *Builder) GetByteConfigWithValidate() ([]byte, error) {
	config, err := b.generateVectorConfig()
	if err != nil {
		return nil, err
	}
	if len(b.Pipelines) != 0 {
		for _, pipeline := range b.Pipelines {
			for _, source := range config.Sources {
				if pipeline.Type() != vectorv1alpha1.ClusterPipelineKind {
					if source.Type != KubernetesSourceType {
						return nil, PipelineTypeError
					}
					if source.ExtraNamespaceLabelSelector != "" {
						if source.ExtraNamespaceLabelSelector != k8s.NamespaceNameToLabel(pipeline.GetNamespace()) {
							return nil, PipelineScopeError
						}
					}
				}
			}
		}
	}
	data, err := vectorConfigToByte(config)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (b *Builder) generateVectorConfig() (*VectorConfig, error) {
	vectorConfig := New(b.vaCtrl.Vector)

	sources, transforms, sinks, err := b.getComponents()
	if err != nil {
		return nil, err
	}

	if len(sources) == 0 {
		sources = []*Source{sourceDefault}
	}
	if len(sinks) == 0 {
		sinks = []*Sink{sinkDefault}
	}

	vectorConfig.Sinks = sinks
	vectorConfig.Sources = sources
	vectorConfig.Transforms = transforms

	return vectorConfig, nil
}

func (b *Builder) getComponents() (sources []*Source, transforms []*Transform, sinks []*Sink, err error) {

	for _, pipeline := range b.Pipelines {
		pipelineSources, err := getSources(pipeline, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, source := range pipelineSources {
			if err != nil {
				return nil, nil, nil, err
			}
			if source.Type == KubernetesSourceType {
				if pipeline.Type() != vectorv1alpha1.ClusterPipelineKind && source.ExtraNamespaceLabelSelector == "" {
					source.ExtraNamespaceLabelSelector = k8s.NamespaceNameToLabel(pipeline.GetNamespace())
				}
			}
			sources = append(sources, source)
		}
		pipelineTransforms, err := getTransforms(pipeline)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, transform := range pipelineTransforms {
			if err != nil {
				return nil, nil, nil, err
			}
			transforms = append(transforms, transform)
		}
		pipelineSinks, err := getSinks(pipeline)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, sink := range pipelineSinks {
			if err != nil {
				return nil, nil, nil, err
			}
			sinks = append(sinks, sink)
		}
	}
	return sources, transforms, sinks, nil
}

func vectorConfigToByte(config *VectorConfig) ([]byte, error) {
	cfgMap, err := cfgToMap(config)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(cfgMap)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func getSources(pipeline pipeline.Pipeline, filter []string) ([]*Source, error) {
	var sources []*Source
	sourcesMap, err := decodeRaw(pipeline.GetSpec().Sources.Raw)
	if err != nil {
		return nil, err
	}
	for k, v := range sourcesMap {
		if len(filter) != 0 {
			if !contains(filter, k) {
				continue
			}
		}
		var source *Source
		if err := mapstructure.Decode(v, &source); err != nil {
			return nil, err
		}
		source.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
		sources = append(sources, source)
	}
	return sources, nil
}

func getTransforms(pipeline pipeline.Pipeline) ([]*Transform, error) {
	if pipeline.GetSpec().Transforms == nil {
		return nil, nil
	}
	transformsMap, err := decodeRaw(pipeline.GetSpec().Transforms.Raw)
	if err != nil {
		return nil, err
	}
	var transforms []*Transform
	if err := json.Unmarshal(pipeline.GetSpec().Transforms.Raw, &transformsMap); err != nil {
		return nil, err
	}
	for k, v := range transformsMap {
		var transform *Transform
		if err := mapstructure.Decode(v, &transform); err != nil {
			return nil, err
		}
		transform.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
		for i, inputName := range transform.Inputs {
			transform.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
		}
		transforms = append(transforms, transform)
	}
	return transforms, nil
}

func getSinks(pipeline pipeline.Pipeline) ([]*Sink, error) {
	sinksMap, err := decodeRaw(pipeline.GetSpec().Sinks.Raw)
	if err != nil {
		return nil, err
	}
	var sinks []*Sink
	for k, v := range sinksMap {
		var sink *Sink
		if err := mapstructure.Decode(v, &sink); err != nil {
			return nil, err
		}
		sink.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
		for i, inputName := range sink.Inputs {
			sink.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
		}
		sinks = append(sinks, sink)
	}
	return sinks, nil
}

func cfgToMap(config *VectorConfig) (cfgMap map[string]interface{}, err error) {
	sources := make(map[string]interface{})
	transforms := make(map[string]interface{})
	sinks := make(map[string]interface{})
	for _, source := range config.Sources {
		spec, err := Mapper(source)
		if err != nil {
			return nil, err
		}
		sources[source.Name] = spec
	}
	for _, transform := range config.Transforms {
		spec, err := Mapper(transform)
		if err != nil {
			return nil, err
		}
		transforms[transform.Name] = spec
	}
	for _, sink := range config.Sinks {
		spec, err := Mapper(sink)
		if err != nil {
			return nil, err
		}
		sinks[sink.Name] = spec
	}

	err = mapstructure.Decode(config, &cfgMap)
	if err != nil {
		return nil, err
	}
	// TODO: remove hardcoded map keys
	cfgMap["sources"] = sources
	cfgMap["transforms"] = transforms
	cfgMap["sinks"] = sinks

	return cfgMap, nil
}
