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

package vector

import (
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

type VectorConfig struct {
	DataDir    string                  `mapstructure:"data_dir"`
	Api        *vectorv1alpha1.ApiSpec `mapstructure:"api"`
	Sources    []*Source               `mapstructure:"sources"`
	Transforms []*Transform            `mapstructure:"transforms"`
	Sinks      []*Sink                 `mapstructure:"sinks"`
}

type Source struct {
	Name                        string
	Type                        string                 `mapper:"type"`
	ExtraNamespaceLabelSelector string                 `mapper:"extra_namespace_label_selector"`
	Options                     map[string]interface{} `mapstructure:",remain"`
}

type Transform struct {
	Name    string
	Type    string                 `mapper:"type"`
	Inputs  []string               `mapper:"inputs"`
	Options map[string]interface{} `mapstructure:",remain"`
}

type Sink struct {
	Name    string
	Type    string                 `mapper:"type"`
	Inputs  []string               `mapper:"inputs"`
	Options map[string]interface{} `mapstructure:",remain"`
}

type ConfigComponent interface {
	GetOptions() map[string]interface{}
}
