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

package vectorpipeline

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Type = "noCluster"
)

type Controller struct {
	VectorPipeline *vectorv1alpha1.VectorPipeline
}

func (ctrl *Controller) Spec() vectorv1alpha1.VectorPipelineSpec {
	return ctrl.VectorPipeline.Spec
}

func (ctrl *Controller) Name() string {
	return ctrl.VectorPipeline.Name
}

func (ctrl *Controller) Namespace() string {
	return ctrl.VectorPipeline.Namespace
}

func (ctrl *Controller) Type() string {
	return Type
}

func (ctrl *Controller) SetConfigCheck(value bool) {
	ctrl.VectorPipeline.Status.ConfigCheckResult = &value
}

func (ctrl *Controller) SetReason(reason *string) {
	ctrl.VectorPipeline.Status.Reason = reason
}

func (ctrl *Controller) GetLastAppliedPipeline() *uint32 {
	return ctrl.VectorPipeline.Status.LastAppliedPipelineHash
}

func (ctrl *Controller) SetLastAppliedPipeline(hash *uint32) {
	ctrl.VectorPipeline.Status.LastAppliedPipelineHash = hash
}

func (ctrl *Controller) UpdateStatus(ctx context.Context, c client.Client) error {
	return k8sutils.UpdateStatus(ctx, ctrl.VectorPipeline, c)
}

func NewController(vp *vectorv1alpha1.VectorPipeline) *Controller {
	return &Controller{
		VectorPipeline: vp,
	}
}
