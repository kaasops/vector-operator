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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PipelineSecretBackend declares a named secret backend for a pipeline.
type PipelineSecretBackend struct {
	// +kubebuilder:validation:Enum=kubernetes_secret
	Type string `json:"type"`
	// Name of the Kubernetes Secret. For VectorPipeline it is always resolved
	// from the pipeline's own namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace of the Secret. Required in ClusterVectorPipeline, forbidden in
	// VectorPipeline (enforced at reconcile time; the spec type is shared).
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// VectorPipelineSpec defines the desired state of VectorPipeline
type VectorPipelineSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	Sources *runtime.RawExtension `json:"sources,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Transforms *runtime.RawExtension `json:"transforms,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Sinks *runtime.RawExtension `json:"sinks,omitempty"`
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(k, k.matches('^[A-Za-z0-9_]+$'))",message="secret backend alias must match ^[A-Za-z0-9_]+$"
	Secret map[string]PipelineSecretBackend `json:"secret,omitempty"`
}

// VectorPipelineStatus defines the observed state of VectorPipeline
type VectorPipelineStatus struct {
	Role              *VectorPipelineRole `json:"role,omitempty"`
	ConfigCheckResult *bool               `json:"configCheckResult,omitempty"`
	Reason            *string             `json:"reason,omitempty"`
	// LastAppliedPipelineHash holds the CRC32 (uint32) hash of the last successfully
	// applied pipeline config. It is stored as an int64 because a uint32 can exceed the
	// int32 upper bound (2147483647); an int32 field would reject roughly half of all
	// hash values and leave the pipeline stuck with configCheckResult=false. See #232.
	LastAppliedPipelineHash *int64 `json:"LastAppliedPipelineHash,omitempty"`
	// RelatedSecretsHash holds an int64-folded sha256 over the identities (namespace,
	// name, UID, resourceVersion) of every Secret the pipeline's SECRET[] references
	// actually use - never over secret data, so the published value cannot fingerprint
	// or brute-force secret contents. Any payload change bumps the Secret's
	// resourceVersion, so the hash still changes whenever a referenced Secret rotates
	// even though the pipeline spec itself did not, letting the reconciler detect that
	// drift. Absent (nil) for pipelines whose config references no secrets.
	RelatedSecretsHash *int64 `json:"relatedSecretsHash,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=vp,categories=all
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="Valid",type="boolean",JSONPath=".status.configCheckResult"
//+kubebuilder:printcolumn:name="Role",type="string",JSONPath=".status.role"

// VectorPipeline is the Schema for the vectorpipelines API
type VectorPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VectorPipelineSpec   `json:"spec,omitempty"`
	Status VectorPipelineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VectorPipelineList contains a list of VectorPipeline
type VectorPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VectorPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VectorPipeline{}, &VectorPipelineList{})
}
