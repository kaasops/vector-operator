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
	"context"
	"encoding/json"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func Get(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client) ([]byte, error) {
	vCtrl := pipeline.NewController(ctx, c, nil)
	pipelines, err := vCtrl.SelectSucceesed()
	if err != nil {
		return nil, err
	}

	cfg, err := GenerateVectorConfig(v, pipelines)
	if err != nil {
		return nil, err
	}

	return VectorConfigToJson(cfg)
}

func GenerateVectorConfig(
	v *vectorv1alpha1.Vector,
	vCtrls []pipeline.Controller,
) (*vector.VectorConfig, error) {
	cfg := vector.New(v.Spec.Agent.DataDir, v.Spec.Agent.ApiEnabled)
	sources, transforms, sinks, err := getComponents(vCtrls)
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		sources = sourceDefault
	}
	if len(sinks) == 0 {
		sinks = sinkDefault
	}

	cfg.Sinks = sinks
	cfg.Sources = sources
	cfg.Transforms = transforms

	return cfg, nil
}

func getComponents(vCtrls []pipeline.Controller) (map[string]interface{}, map[string]interface{}, map[string]interface{}, error) {
	sourcesMap := make(map[string]interface{})
	transformsMap := make(map[string]interface{})
	sinksMap := make(map[string]interface{})

	for _, vCtrl := range vCtrls {
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

func VectorConfigToJson(conf *vector.VectorConfig) ([]byte, error) {
	data, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	return data, nil
}
