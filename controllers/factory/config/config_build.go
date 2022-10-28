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
	"github.com/kaasops/vector-operator/controllers/factory/vector"
)

var (
	sourceDefault = map[string]interface{}{
		"defaultSource": map[string]string{
			"type": "kubernetes_logs",
		},
	}

	rate        int32 = 100
	sinkDefault       = map[string]interface{}{
		"defaultSink": map[string]interface{}{
			"type":                "blackhole",
			"inputs":              []string{"defaultSource"},
			"rate":                rate,
			"print_interval_secs": 60,
		},
	}
)

func (cfg *Config) GenerateVectorConfig() error {
	vectorConfig := vector.New(cfg.vaCtrl.Vector.Spec.Agent.DataDir, cfg.vaCtrl.Vector.Spec.Agent.ApiEnabled)

	sources, transforms, sinks, err := cfg.getComponents()
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		sources = sourceDefault
	}
	if len(sinks) == 0 {
		sinks = sinkDefault
	}

	vectorConfig.Sinks = sinks
	vectorConfig.Sources = sources
	vectorConfig.Transforms = transforms

	cfg.VectorConfig = vectorConfig

	return nil
}

func (cfg *Config) getComponents() (map[string]interface{}, map[string]interface{}, map[string]interface{}, error) {
	sourcesMap := make(map[string]interface{})
	transformsMap := make(map[string]interface{})
	sinksMap := make(map[string]interface{})

	for _, vCtrl := range cfg.pCtrls {
		sources, err := vCtrl.GetSources(nil)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, source := range sources {
			spec, err := vector.Mapper(source)
			if err != nil {
				return nil, nil, nil, err
			}
			sourcesMap[source.Name] = spec
		}
		transforms, err := vCtrl.GetTransforms()
		if err != nil {
			return nil, nil, nil, err
		}
		for _, transform := range transforms {
			spec, err := vector.Mapper(transform)
			if err != nil {
				return nil, nil, nil, err
			}
			transformsMap[transform.Name] = spec
		}
		sinks, err := vCtrl.GetSinks()
		if err != nil {
			return nil, nil, nil, err
		}
		for _, sink := range sinks {
			spec, err := vector.Mapper(sink)
			if err != nil {
				return nil, nil, nil, err
			}
			sinksMap[sink.Name] = spec
		}
	}
	return sourcesMap, transformsMap, sinksMap, nil
}
