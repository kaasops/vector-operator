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

package vectoragent

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/utils"
)

func (vr *VectorAgentReconciler) createVectorAgentConfig(ctx context.Context) (*corev1.Secret, error) {
	cfg, err := config.Get(ctx, vr.Vector, vr.Client)
	if err != nil {
		return nil, err
	}

	cfgHash := utils.GetHash(cfg)

	if vr.Vector.Status.LastAppliedConfigHash == nil || *vr.Vector.Status.LastAppliedConfigHash != cfgHash {
		err = configcheck.Run(cfg, vr.Client, vr.Clientset, vr.Vector.Name, vr.Vector.Namespace, vr.Vector.Spec.Agent.Image)
		if _, ok := err.(*configcheck.ErrConfigCheck); ok {
			if err := setFailedStatus(ctx, vr.Vector, vr.Client, err); err != nil {
				return nil, err
			}
			return nil, err
		}
		if err != nil {
			return nil, err
		}

		if err := SetLastAppliedPipelineStatus(ctx, vr.Vector, vr.Client, &cfgHash); err != nil {
			return nil, err
		}

		if err := setSucceesStatus(ctx, vr.Vector, vr.Client); err != nil {
			return nil, err
		}

	}

	labels := vr.labelsForVectorAgent()
	config := map[string][]byte{
		"agent.json": cfg,
	}

	secret := &corev1.Secret{
		ObjectMeta: vr.objectMetaVectorAgent(labels),
		Data:       config,
	}

	return secret, nil
}
