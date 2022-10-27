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

type VectorConfig struct {
	DataDir    string                 `json:"data_dir,omitempty"`
	Api        *ApiSpec               `json:"api,omitempty"`
	Sources    map[string]interface{} `json:"sources,omitempty"`
	Transforms map[string]interface{} `json:"transforms,omitempty"`
	Sinks      map[string]interface{} `json:"sinks,omitempty"`
}

type ApiSpec struct {
	Enabled    *bool   `json:"enabled,omitempty"`
	Address    *string `json:"address,omitempty"`
	Playground *bool   `json:"playground,omitempty"`
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
