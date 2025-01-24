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
	"fmt"
	"github.com/kaasops/vector-operator/internal/utils/hash"

	corev1 "k8s.io/api/core/v1"
)

type globalOptions struct {
	ExpireMetricsSecs *int `yaml:"expire_metrics_secs,omitempty"`
}

type VectorConfig struct {
	DataDir           string   `yaml:"data_dir"`
	ExpireMetricsSecs *int     `yaml:"expire_metrics_secs,omitempty"`
	Api               *ApiSpec `yaml:"api"`
	PipelineConfig    `yaml:",inline"`
	internal          internalConfig `yaml:"-"`
}

type PipelineConfig struct {
	Sources    map[string]*Source    `yaml:"sources"`
	Transforms map[string]*Transform `yaml:"transforms"`
	Sinks      map[string]*Sink      `yaml:"sinks"`
}

type ApiSpec struct {
	Address    string `yaml:"address,omitempty"`
	Enabled    bool   `yaml:"enabled,omitempty"`
	Playground bool   `yaml:"playground,omitempty"`
}

type Source struct {
	Name                        string         `yaml:"-"`
	Type                        string         `mapstructure:"type" yaml:"type"`
	ExtraNamespaceLabelSelector string         `mapstructure:"extra_namespace_label_selector" yaml:"extra_namespace_label_selector,omitempty"`
	ExtraLabelSelector          string         `mapstructure:"extra_label_selector" yaml:"extra_label_selector,omitempty"`
	ExtraFieldSelector          string         `mapstructure:"extra_field_selector" yaml:"extra_field_selector,omitempty"`
	UseApiServerCache           bool           `mapstructure:"use_apiserver_cache" yaml:"use_apiserver_cache,omitempty"`
	Options                     map[string]any `mapstructure:",remain" yaml:",inline,omitempty"`
}

type Transform struct {
	Name    string                 `yaml:"-"`
	Type    string                 `mapstructure:"type" yaml:"type"`
	Inputs  []string               `mapstructure:"inputs" yaml:"inputs"`
	Options map[string]interface{} `mapstructure:",remain" yaml:",inline,omitempty"`
}

type Sink struct {
	Name    string                 `yaml:"-"`
	Type    string                 `mapstructure:"type" yaml:"type"`
	Inputs  []string               `mapstructure:"inputs" yaml:"inputs"`
	Options map[string]interface{} `mapstructure:",remain" yaml:",inline,omitempty"`
}

// Used to unmarshal pipeline config from kubernetes object and pass to mapstructure.
type pipelineConfig_ struct {
	Sources    map[string]interface{}
	Transforms map[string]interface{}
	Sinks      map[string]interface{}
}

type ServicePort struct {
	IsKubernetesEvents bool
	PipelineName       string
	SourceName         string
	Namespace          string
	Port               int32
	Protocol           corev1.Protocol
	ServiceName        string
}

type internalConfig struct {
	servicePort map[string]*ServicePort
}

func (c *internalConfig) addServicePort(port *ServicePort) error {
	key := fmt.Sprintf("%d/%s", port.Port, port.Protocol)
	if v, ok := c.servicePort[key]; !ok {
		c.servicePort[key] = port
	} else {
		return fmt.Errorf("duplicate port %s in %s and %s", key, v.PipelineName, port.PipelineName)
	}
	return nil
}

func (c *VectorConfig) GetGlobalConfigHash() *uint32 {
	bytes, _ := json.Marshal(globalOptions{
		ExpireMetricsSecs: c.ExpireMetricsSecs,
	})
	gHash := hash.Get(bytes)
	return &gHash
}
