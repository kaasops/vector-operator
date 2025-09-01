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

// VectorPipelineSpec defines the desired state of VectorPipeline
type VectorPipelineSpec struct {
	// Controls whether prefix logic should be applied
	// +optional
	// +kubebuilder:default:=false
	SkipPrefix bool `json:"skipPrefix,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Sources *runtime.RawExtension `json:"sources,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Transforms *runtime.RawExtension `json:"transforms,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Sinks *runtime.RawExtension `json:"sinks,omitempty"`
}

// VectorPipelineStatus defines the observed state of VectorPipeline
type VectorPipelineStatus struct {
	Role                    *VectorPipelineRole `json:"role,omitempty"`
	ConfigCheckResult       *bool               `json:"configCheckResult,omitempty"`
	Reason                  *string             `json:"reason,omitempty"`
	LastAppliedPipelineHash *uint32             `json:"LastAppliedPipelineHash,omitempty"`
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
