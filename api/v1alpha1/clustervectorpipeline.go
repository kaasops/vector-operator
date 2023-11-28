package v1alpha1

import (
	"context"

	"github.com/kaasops/vector-operator/pkg/utils/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ClusterPipelineKind = "ClusterVectorPipeline"
)

func (vp *ClusterVectorPipeline) GetSpec() VectorPipelineSpec {
	return vp.Spec
}

func (vp *ClusterVectorPipeline) IsValid() bool {
	if vp.Status.ConfigCheckResult != nil {
		return *vp.Status.ConfigCheckResult
	}
	return false
}

func (vp *ClusterVectorPipeline) IsDeleted() bool {
	return !vp.DeletionTimestamp.IsZero()
}

func (vp *ClusterVectorPipeline) SetConfigCheck(value bool) {
	vp.Status.ConfigCheckResult = &value
}

func (vp *ClusterVectorPipeline) GetConfigCheckResult() *bool {
	return vp.Status.ConfigCheckResult
}

func (vp *ClusterVectorPipeline) SetReason(reason *string) {
	vp.Status.Reason = reason
}

func (vp *ClusterVectorPipeline) GetLastAppliedPipeline() *uint32 {
	return vp.Status.LastAppliedPipelineHash
}

func (vp *ClusterVectorPipeline) SetLastAppliedPipeline(hash *uint32) {
	vp.Status.LastAppliedPipelineHash = hash
}

func (vp *ClusterVectorPipeline) UpdateStatus(ctx context.Context, c client.Client) error {
	return k8s.UpdateStatus(ctx, vp, c)
}
