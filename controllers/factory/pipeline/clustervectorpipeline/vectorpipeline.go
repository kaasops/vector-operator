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

package clustervectorpipeline

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Type = "cluster"
)

type Controller struct {
	ClusterVectorPipeline *vectorv1alpha1.ClusterVectorPipeline
}

func (ctrl *Controller) Spec() vectorv1alpha1.VectorPipelineSpec {
	return ctrl.ClusterVectorPipeline.Spec
}

func (ctrl *Controller) Name() string {
	return ctrl.ClusterVectorPipeline.Name
}

func (ctrl *Controller) Namespace() string {
	return ctrl.ClusterVectorPipeline.Namespace
}

func (ctrl *Controller) Type() string {
	return Type
}

func (ctrl *Controller) SetConfigCheck(value bool) {
	ctrl.ClusterVectorPipeline.Status.ConfigCheckResult = &value
}

func (ctrl *Controller) SetReason(reason *string) {
	ctrl.ClusterVectorPipeline.Status.Reason = reason
}

func (ctrl *Controller) GetLastAppliedPipeline() *uint32 {
	return ctrl.ClusterVectorPipeline.Status.LastAppliedPipelineHash
}

func (ctrl *Controller) SetLastAppliedPipeline(hash *uint32) {
	ctrl.ClusterVectorPipeline.Status.LastAppliedPipelineHash = hash
}

func (ctrl *Controller) UpdateStatus(ctx context.Context, c client.Client) error {
	return k8sutils.UpdateStatus(ctx, ctrl.ClusterVectorPipeline, c)
}

func NewController(cvp *vectorv1alpha1.ClusterVectorPipeline) *Controller {
	return &Controller{
		ClusterVectorPipeline: cvp,
	}
}
