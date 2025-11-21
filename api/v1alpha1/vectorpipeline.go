package v1alpha1

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VectorPipelineRole string

const (
	VectorPipelineRoleUnknown    VectorPipelineRole = "unknown"
	VectorPipelineRoleAgent      VectorPipelineRole = "agent"
	VectorPipelineRoleAggregator VectorPipelineRole = "aggregator"
	VectorPipelineRoleMixed      VectorPipelineRole = "mixed"
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

func (vp *VectorPipeline) GetRole() VectorPipelineRole {
	if vp.Status.Role == nil {
		return VectorPipelineRoleUnknown
	}
	return *vp.Status.Role
}

func (vp *VectorPipeline) SetRole(role *VectorPipelineRole) {
	vp.Status.Role = role
}

func (vp *VectorPipeline) GetTypeMeta() metav1.TypeMeta {
	return vp.TypeMeta
}

func (vp *VectorPipeline) SkipPrefix() bool {
	return vp.GetSpec().SkipPrefix
}
