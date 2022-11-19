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
	"errors"

	corev1 "k8s.io/api/core/v1"

	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/utils/hash"
)

func (ctrl *Controller) createVectorAgentConfig(ctx context.Context) (*corev1.Secret, error) {
	cfgHash := hash.Get(ctrl.Config)

	configCheck := configcheck.New(
		ctrl.Config,
		ctrl.Client,
		ctrl.ClientSet,
		ctrl.Vector.Name,
		ctrl.Vector.Namespace,
		ctrl.Vector.Spec.Agent.Image,
		ctrl.Vector.Spec.Agent.Env,
	)

	if ctrl.Vector.Status.LastAppliedConfigHash == nil || *ctrl.Vector.Status.LastAppliedConfigHash != cfgHash {
		err := configCheck.Run(ctx)
		if errors.Is(err, configcheck.ValidationError) {
			if err := ctrl.SetFailedStatus(ctx, err); err != nil {
				return nil, err
			}
			return nil, err
		}
		if err != nil {
			return nil, err
		}

		if err := ctrl.SetLastAppliedPipelineStatus(ctx, &cfgHash); err != nil {
			return nil, err
		}

		if err := ctrl.SetSucceesStatus(ctx); err != nil {
			return nil, err
		}

	}

	labels := ctrl.labelsForVectorAgent()
	config := map[string][]byte{
		"agent.json": ctrl.Config,
	}

	secret := &corev1.Secret{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels),
		Data:       config,
	}

	return secret, nil
}
