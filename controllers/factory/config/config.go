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
	"strconv"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"
	"github.com/mitchellh/mapstructure"
)

type ApiSpec struct {
	Address    string `json:"address,omitempty"`
	Enabled    bool   `json:"enabled,omitempty"`
	Playground bool   `json:"playground,omitempty"`
}

func New(vector *vectorv1alpha1.Vector) *VectorConfig {
	sources := []*Source{}
	sinks := []*Sink{}

	api := &ApiSpec{
		Address:    "0.0.0.0:" + strconv.Itoa(vectoragent.ApiPort),
		Enabled:    vector.Spec.Agent.Api.Enabled,
		Playground: vector.Spec.Agent.Api.Playground,
	}

	return &VectorConfig{
		DataDir: vector.Spec.Agent.DataDir,
		Api:     api,
		Sources: sources,
		Sinks:   sinks,
	}
}

func Mapper(c ConfigComponent) (map[string]interface{}, error) {
	spec := c.GetOptions()
	config := &mapstructure.DecoderConfig{
		Result:               &spec,
		ZeroFields:           false,
		TagName:              "mapper",
		IgnoreUntaggedFields: true,
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, err
	}
	err = decoder.Decode(c)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func (t Source) GetOptions() map[string]interface{} {
	return t.Options
}

func (t Transform) GetOptions() map[string]interface{} {
	return t.Options
}

func (t Sink) GetOptions() map[string]interface{} {
	return t.Options
}
