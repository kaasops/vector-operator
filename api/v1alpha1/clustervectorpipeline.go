package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (vp *ClusterVectorPipeline) GetReason() *string {
	return vp.Status.Reason
}

func (vp *ClusterVectorPipeline) GetLastAppliedPipeline() *int64 {
	return vp.Status.LastAppliedPipelineHash
}

func (vp *ClusterVectorPipeline) SetLastAppliedPipeline(hash *int64) {
	vp.Status.LastAppliedPipelineHash = hash
}

func (vp *ClusterVectorPipeline) GetRelatedSecretsHash() *int64 {
	return vp.Status.RelatedSecretsHash
}

func (vp *ClusterVectorPipeline) SetRelatedSecretsHash(hash *int64) {
	vp.Status.RelatedSecretsHash = hash
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
