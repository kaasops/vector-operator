/*
Copyright 2024.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VectorAggregatorSpec defines the desired state of VectorAggregator
type VectorAggregatorSpec struct {
	VectorCommon `json:",inline"`
	Replicas     int32 `json:"replicas,omitempty"`
	// Defines a filter for the Vector Pipeline and Cluster Vector Pipeline by labels.
	// If not specified, all pipelines will be selected.
	Selector VectorSelectorSpec `json:"selector,omitempty"`
}

// VectorAggregatorStatus defines the observed state of VectorAggregator
type VectorAggregatorStatus struct {
	VectorCommonStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Valid",type="boolean",JSONPath=".status.configCheckResult"

// VectorAggregator is the Schema for the vectoraggregators API
type VectorAggregator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VectorAggregatorSpec   `json:"spec,omitempty"`
	Status VectorAggregatorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VectorAggregatorList contains a list of VectorAggregator
type VectorAggregatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VectorAggregator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VectorAggregator{}, &VectorAggregatorList{})
}
