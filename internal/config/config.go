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
	"net"
	"strconv"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/evcollector"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
	goyaml "sigs.k8s.io/yaml"
)

var (
	ErrNotAllowedSourceType   = errors.New("type kubernetes_logs only allowed")
	ErrClusterScopeNotAllowed = errors.New("logs from external namespace not allowed")
)

type VectorConfigParams struct {
	AggregatorName    string
	ApiEnabled        bool
	PlaygroundEnabled bool
	UseApiServerCache bool
	InternalMetrics   bool
	ExpireMetricsSecs *int
}

func newVectorConfig(p VectorConfigParams) *VectorConfig {
	sources := make(map[string]*Source)
	transforms := make(map[string]*Transform)
	sinks := make(map[string]*Sink)

	api := &ApiSpec{
		Address:    net.JoinHostPort("0.0.0.0", strconv.Itoa(AgentApiPort)),
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
		globalOptions: globalOptions{
			ExpireMetricsSecs: p.ExpireMetricsSecs,
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
		case isAgentAndAggregator(s.Type):
			agentCount++
			aggregatorCount++
		case isAgent(s.Type):
			agentCount++
		case isAggregator(s.Type):
			aggregatorCount++
		default:
			return nil, fmt.Errorf("unsupported source type: %s", s.Type)
		}
	}
	switch {
	case agentCount > 0 && aggregatorCount > 0:
		role := vectorv1alpha1.VectorPipelineRoleMixed
		return &role, nil
	case agentCount > 0:
		role := vectorv1alpha1.VectorPipelineRoleAgent
		return &role, nil
	case aggregatorCount > 0:
		role := vectorv1alpha1.VectorPipelineRoleAggregator
		return &role, nil
	}
	return nil, fmt.Errorf("unknown vector role")
}

type SPGroup struct {
	PipelineName string
	Namespace    string
	ServiceName  string
}

func (c *VectorConfig) GetSourcesServicePorts() map[SPGroup][]*ServicePort {
	m := make(map[SPGroup][]*ServicePort)
	for _, s := range c.internal.servicePort {
		spg := SPGroup{
			PipelineName: s.PipelineName,
			Namespace:    s.Namespace,
			ServiceName:  s.ServiceName,
		}
		m[spg] = append(m[spg], s)
	}
	return m
}

func (c *VectorConfig) GetEventCollectorConfig(namespace string) *evcollector.Config {
	items := make([]*evcollector.ReceiverParams, 0)
	for _, s := range c.internal.servicePort {
		if s.IsKubernetesEvents {
			items = append(items, &evcollector.ReceiverParams{
				ServiceNamespace: namespace,
				ServiceName:      s.ServiceName,
				WatchedNamespace: s.Namespace,
				Port:             strconv.Itoa(int(s.Port)),
			})
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &evcollector.Config{
		Receivers: items,
	}
}
