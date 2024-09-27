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
)

// VectorSpec defines the desired state of Vector
type VectorSpec struct {
	// Vector Agent
	Agent *VectorAgent `json:"agent,omitempty"`
	// Determines if requests to the kube-apiserver can be served by a cache.
	UseApiServerCache bool `json:"useApiServerCache,omitempty"`
	// Defines a filter for the Vector Pipeline and Cluster Vector Pipeline by labels.
	// If not specified, all pipelines will be selected.
	Selector *VectorSelectorSpec `json:"selector,omitempty"`
}

// VectorStatus defines the observed state of Vector
type VectorStatus struct {
	VectorCommonStatus `json:",inline"`
}

// VectorAgent is the Schema for the Vector Agent
type VectorAgent struct {
	VectorCommon `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="Valid",type="boolean",JSONPath=".status.configCheckResult"

// Vector is the Schema for the vectors API
type Vector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VectorSpec   `json:"spec,omitempty"`
	Status VectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VectorList contains a list of Vector
type VectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Vector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Vector{}, &VectorList{})
}
