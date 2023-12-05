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

type VectorConfig struct {
	DataDir        string   `yaml:"data_dir"`
	Api            *ApiSpec `yaml:"api"`
	PipelineConfig `yaml:",inline"`
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
	Name                        string                 `yaml:"-"`
	Type                        string                 `mapstructure:"type" yaml:"type"`
	ExtraNamespaceLabelSelector string                 `mapstructure:"extra_namespace_label_selector" yaml:"extra_namespace_label_selector,omitempty"`
	ExtraLabelSelector          string                 `mapstructure:"extra_label_selector" yaml:"extra_label_selector,omitempty"`
	ExtraFieldSelector          string                 `mapstructure:"extra_field_selector" yaml:"extra_field_selector,omitempty"`
	UseApiServerCache           bool                   `mapstructure:"use_apiserver_cache" yaml:"use_apiserver_cache,omitempty"`
	Options                     map[string]interface{} `mapstructure:",remain" yaml:",inline,omitempty"`
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
