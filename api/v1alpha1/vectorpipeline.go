package v1alpha1

import (
	"context"

	"github.com/kaasops/vector-operator/pkg/utils/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	LocalPipelineKind = "VectorPipeline"
)

func (vp *VectorPipeline) GetSpec() VectorPipelineSpec {
	return vp.Spec
}

func (vp *VectorPipeline) IsValid() bool {
	if vp.Status.ConfigCheckResult != nil {
		return *vp.Status.ConfigCheckResult
	}
	return false
}

func (vp *VectorPipeline) IsDeleted() bool {
	return !vp.DeletionTimestamp.IsZero()
}

func (vp *VectorPipeline) SetConfigCheck(value bool) {
	vp.Status.ConfigCheckResult = &value
}

func (vp *VectorPipeline) GetConfigCheckResult() *bool {
	return vp.Status.ConfigCheckResult
}

func (vp *VectorPipeline) SetReason(reason *string) {
	vp.Status.Reason = reason
}

func (vp *VectorPipeline) GetLastAppliedPipeline() *uint32 {
	return vp.Status.LastAppliedPipelineHash
}

func (vp *VectorPipeline) SetLastAppliedPipeline(hash *uint32) {
	vp.Status.LastAppliedPipelineHash = hash
}

func (vp *VectorPipeline) UpdateStatus(ctx context.Context, c client.Client) error {
	return k8s.UpdateStatus(ctx, vp, c)
}
