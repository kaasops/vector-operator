package v1alpha1

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
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

func (vp *ClusterVectorPipeline) GetRole() VectorPipelineRole {
	if vp.Status.Role == nil {
		return VectorPipelineRoleUnknown
	}
	return *vp.Status.Role
}

func (vp *ClusterVectorPipeline) SetRole(role *VectorPipelineRole) {
	vp.Status.Role = role
}

func (vp *ClusterVectorPipeline) GetTypeMeta() metav1.TypeMeta {
	return vp.TypeMeta
}
