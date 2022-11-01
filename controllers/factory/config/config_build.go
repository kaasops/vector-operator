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
	"github.com/kaasops/vector-operator/controllers/factory/pipeline/clustervectorpipeline"
	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
)

var (
	sourceDefault = &vector.Source{
		Name: "defaultSource",
		Type: "kubernetes_logs",
	}
	sinkDefault = &vector.Sink{
		Name:   "defaultSink",
		Type:   "blackhole",
		Inputs: []string{"defaultSource"},
		Options: map[string]interface{}{
			"rate":                100,
			"print_interval_secs": 60,
		},
	}
)

func (cfg *Config) GenerateVectorConfig() error {
	vectorConfig := vector.New(cfg.vaCtrl.Vector)

	sources, transforms, sinks, err := cfg.getComponents()
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		sources = []*vector.Source{sourceDefault}
	}
	if len(sinks) == 0 {
		sinks = []*vector.Sink{sinkDefault}
	}

	vectorConfig.Sinks = sinks
	vectorConfig.Sources = sources
	vectorConfig.Transforms = transforms

	cfg.VectorConfig = vectorConfig

	return nil
}

func (cfg *Config) getComponents() (sources []*vector.Source, transforms []*vector.Transform, sinks []*vector.Sink, err error) {

	for _, vCtrl := range cfg.pCtrls {
		pipelineSources, err := vCtrl.GetSources(nil)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, source := range pipelineSources {
			if err != nil {
				return nil, nil, nil, err
			}
			if vCtrl.Pipeline.Type() != clustervectorpipeline.Type && source.Type == "kubernetes_logs" {
				source.ExtraNamespaceLabelSelector = k8s.NamespaceNameToLabel(vCtrl.Pipeline.Namespace())
			}
			sources = append(sources, source)
		}
		pipelineTransforms, err := vCtrl.GetTransforms()
		if err != nil {
			return nil, nil, nil, err
		}
		for _, transform := range pipelineTransforms {
			if err != nil {
				return nil, nil, nil, err
			}
			transforms = append(transforms, transform)
		}
		pipelineSinks, err := vCtrl.GetSinks()
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
