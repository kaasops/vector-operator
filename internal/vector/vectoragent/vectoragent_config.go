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

	"github.com/kaasops/vector-operator/internal/utils/compression"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (ctrl *Controller) createVectorAgentConfig(ctx context.Context) (*corev1.Secret, error) {
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", ctrl.Vector.Name)
	labels := ctrl.labelsForVectorAgent()
	annotations := ctrl.annotationsForVectorAgent()
	var data []byte = ctrl.Config

	if ctrl.Vector.Spec.Agent.CompressConfigFile {
		data = compression.Compress(ctrl.Config, log)
	}
	config := map[string][]byte{
		"agent.json": data,
	}
	secret := &corev1.Secret{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels, annotations, ctrl.Vector.Namespace),
		Data:       config,
	}

	return secret, nil
}
