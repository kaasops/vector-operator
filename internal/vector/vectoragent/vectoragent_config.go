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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/compression"
)

func (ctrl *Controller) createVectorAgentConfig(ctx context.Context, name string, data []byte) (*corev1.Secret, error) {
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", ctrl.Vector.Name)
	labels := ctrl.labelsForVectorAgent()
	annotations := ctrl.annotationsForVectorAgent()

	if ctrl.Vector.Spec.Agent.CompressConfigFile {
		data = compression.Compress(data, log)
	}
	config := map[string][]byte{
		"agent.json": data,
	}
	meta := ctrl.objectMetaVectorAgent(labels, annotations, ctrl.Vector.Namespace)
	meta.Name = name
	secret := &corev1.Secret{
		ObjectMeta: meta,
		Data:       config,
	}

	return secret, nil
}

// deleteAgentConfigSecret best-effort removes a config Secret by name. Used to
// clean up the standby (-opt) Secret once checkpoint migration is disabled, so
// the feature gate leaves nothing behind.
func (ctrl *Controller) deleteAgentConfigSecret(ctx context.Context, name string) error {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ctrl.Vector.Namespace}}
	if err := ctrl.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return client.IgnoreNotFound(err)
	}
	return nil
}
