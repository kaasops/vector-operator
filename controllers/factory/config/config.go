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

	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"
)

type Config struct {
	Name string

	Ctx    context.Context
	vaCtrl *vectoragent.Controller
	pCtrls []pipeline.Controller

	VectorConfig *vector.VectorConfig
}

func New(ctx context.Context, vaCtrl *vectoragent.Controller) (*Config, error) {
	cfg := &Config{
		Ctx:    ctx,
		vaCtrl: vaCtrl,
	}

	return cfg, nil
}

func (cfg *Config) FillForVectorAgent() error {
	cfg.Name = cfg.vaCtrl.Vector.Name

	pCtrl := pipeline.NewController(cfg.Ctx, cfg.vaCtrl.Client, nil)
	pCtrls, err := pCtrl.SelectSucceesed()
	if err != nil {
		return err
	}
	cfg.pCtrls = pCtrls

	if err := cfg.GenerateVectorConfig(); err != nil {
		return err
	}

	return nil
}

func (cfg *Config) FillForVectorPipeline(vCtrl *pipeline.Controller) error {
	cfg.Name = vCtrl.Pipeline.Name()

	pCtrls := make([]pipeline.Controller, 1)
	pCtrls[0] = *vCtrl

	cfg.pCtrls = pCtrls

	if err := cfg.GenerateVectorConfig(); err != nil {
		return err
	}

	return nil
}

func (cfg *Config) GetByteConfig() ([]byte, error) {
	data, err := json.Marshal(cfg.VectorConfig)
	if err != nil {
		return nil, err
	}

	return data, nil
}
