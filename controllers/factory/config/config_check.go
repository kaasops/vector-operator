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

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Check(ctx context.Context, v *vectorv1alpha1.Vector, vCtrl *pipeline.Controller, c client.Client, cs *kubernetes.Clientset) error {

	log := log.FromContext(context.TODO()).WithValues("ConfigCheck Vector Pipeline", vCtrl.Pipeline.Name())

	var vCtrls []pipeline.Controller
	vCtrls = append(vCtrls, *vCtrl)

	cfg, err := GenerateVectorConfig(v, vCtrls)
	if err != nil {
		return err
	}

	// if p.Type() == vectorpipeline.Type {
	// 	log.Info("It's VectorPipeline")
	// }

	cfgJson, err := VectorConfigToJson(cfg)
	if err != nil {
		return err
	}

	err = configcheck.Run(cfgJson, c, cs, v.Name, v.Namespace, v.Spec.Agent.Image)
	if _, ok := err.(*configcheck.ErrConfigCheck); ok {
		if err := vCtrl.SetFailedStatus(err); err != nil {
			return err
		}
		log.Error(err, "Vector Config has error")
		return nil
	}
	if err != nil {
		return err
	}

	return vCtrl.SetSucceesStatus()
}
