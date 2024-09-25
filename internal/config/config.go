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
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/vector/vectoragent"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
	"net"
	goyaml "sigs.k8s.io/yaml"
	"strconv"
)

var (
	ErrNotAllowedSourceType   = errors.New("type kubernetes_logs only allowed")
	ErrClusterScopeNotAllowed = errors.New("logs from external namespace not allowed")
)

type VectorConfigParams struct {
	ApiEnabled        bool
	PlaygroundEnabled bool
	UseApiServerCache bool
	InternalMetrics   bool
}

func newVectorConfig(p VectorConfigParams) *VectorConfig {
	sources := make(map[string]*Source)
	transforms := make(map[string]*Transform)
	sinks := make(map[string]*Sink)

	api := &ApiSpec{
		Address:    net.JoinHostPort("0.0.0.0", strconv.Itoa(vectoragent.ApiPort)),
		Enabled:    p.ApiEnabled,
		Playground: p.PlaygroundEnabled,
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
		if sink.Type == PrometheusExporterType {
			return true
		}
	}
	return false
}

func (c *VectorConfig) MarshalJSON() ([]byte, error) {
	yamlByte, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	jsonByte, err := goyaml.YAMLToJSON(yamlByte)
	if err != nil {
		return nil, err
	}
	return jsonByte, nil
}

func (c *PipelineConfig) VectorRole() (*vectorv1alpha1.VectorPipelineRole, error) {
	if len(c.Sources) == 0 {
		return nil, fmt.Errorf("sources list is empty")
	}
	agentCount := 0
	aggregatorCount := 0
	for _, s := range c.Sources {
		switch {
		case isAgent(s.Type):
			agentCount++
			fallthrough // some types can be both an agent and an aggregator at the same time
		case isAggregator(s.Type):
			aggregatorCount++
		default:
			return nil, fmt.Errorf("unsupported source type: %s", s.Type)
		}
	}
	switch {
	case len(c.Sources) == agentCount:
		role := vectorv1alpha1.VectorPipelineRoleAgent
		return &role, nil
	case len(c.Sources) == aggregatorCount:
		role := vectorv1alpha1.VectorPipelineRoleAggregator
		return &role, nil
	}
	return nil, fmt.Errorf("unknown vector role")
}

func (c *VectorConfig) GetSourcesServicePorts() map[string][]*ServicePort {
	m := make(map[string][]*ServicePort)
	for _, s := range c.internal.servicePort {
		key := fmt.Sprintf("%s-%s", s.Namespace, s.PipelineName)
		m[key] = append(m[key], s)
	}
	return m
}
