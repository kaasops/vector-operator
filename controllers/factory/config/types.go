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
	DataDir    string       `mapstructure:"data_dir"`
	Api        *ApiSpec     `mapstructure:"api"`
	Sources    []*Source    `mapstructure:"sources"`
	Transforms []*Transform `mapstructure:"transforms"`
	Sinks      []*Sink      `mapstructure:"sinks"`
}

type Source struct {
	Name                        string
	Type                        string                 `mapper:"type"`
	ExtraNamespaceLabelSelector string                 `mapstructure:"extra_namespace_label_selector" mapper:"extra_namespace_label_selector,omitempty"`
	ExtraLabelSelector          string                 `mapstructure:"extra_label_selector" mapper:"extra_label_selector,omitempty"`
	ExtraFieldSelector          string                 `mapstructure:"extra_field_selector" mapper:"extra_field_selector,omitempty"`
	Options                     map[string]interface{} `mapstructure:",remain"`
}

type Transform struct {
	Name        string
	Type        string                 `mapper:"type"`
	Inputs      []string               `mapper:"inputs"`
	Condition   interface{}            `mapper:"condition,omitempty"`
	Options     map[string]interface{} `mapstructure:",remain"`
	OptionsHash uint32
	Merged      bool
}

type Sink struct {
	Name        string
	Type        string                 `mapper:"type"`
	Inputs      []string               `mapper:"inputs"`
	Options     map[string]interface{} `mapstructure:",remain"`
	OptionsHash uint32
	Merged      bool
}

type ConfigComponent interface {
	GetOptions() map[string]interface{}
}
