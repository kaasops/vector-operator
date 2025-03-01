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
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Controller struct {
	client.Client
	Vector *vectorv1alpha1.Vector

	ByteConfig []byte
	Config     *config.VectorConfig
	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	ClientSet *kubernetes.Clientset
}

func NewController(v *vectorv1alpha1.Vector, c client.Client, cs *kubernetes.Clientset) *Controller {
	ctrl := &Controller{
		Client:    c,
		Vector:    v,
		ClientSet: cs,
	}
	ctrl.SetDefault()
	return ctrl
}

func (ctrl *Controller) SetSuccessStatus(ctx context.Context, cfgHash, globCfgHash *uint32) error {
	var status = true
	ctrl.Vector.Status.ConfigCheckResult = &status
	ctrl.Vector.Status.Reason = nil
	ctrl.Vector.Status.LastAppliedConfigHash = cfgHash
	ctrl.Vector.Status.LastAppliedGlobalConfigHash = globCfgHash

	return k8s.UpdateStatus(ctx, ctrl.Vector, ctrl.Client)
}

func (ctrl *Controller) SetFailedStatus(ctx context.Context, reason string) error {
	var status = false
	ctrl.Vector.Status.ConfigCheckResult = &status
	ctrl.Vector.Status.Reason = &reason

	return k8s.UpdateStatus(ctx, ctrl.Vector, ctrl.Client)
}
