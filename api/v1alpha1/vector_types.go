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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VectorSpec defines the desired state of Vector
type VectorSpec struct {
	// DisableAggregation
	// DisableAggregation bool `json:"disableAggregation,omitempty"`
	// Vector Agent
	Agent *VectorAgent `json:"agent,omitempty"`
	// Vector Aggregator
	// Aggregator *VectorAggregator `json:"aggregator,omitempty"`
}

// VectorStatus defines the observed state of Vector
type VectorStatus struct {
	ConfigCheckResult *bool   `json:"configCheckResult,omitempty"`
	Reason            *string `json:"reason,omitempty"`
}

// VectorAgent is the Schema for the Vector Agent
type VectorAgent struct {
	// +kubebuilder:default:="timberio/vector:0.24.0-distroless-libc"
	Image       string          `json:"image,omitempty"`
	DataDir     string          `json:"dataDir,omitempty"`
	ApiEnabled  bool            `json:"ApiEnabled,omitempty"`
	Service     bool            `json:"service,omitempty"`
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
}

// VectorAggregator is the Schema for the Vector Aggregator
type VectorAggregator struct {
	Enable bool `json:"enable,omitempty"`
	// +kubebuilder:default:="timberio/vector:0.24.0-distroless-libc"
	Image    string `json:"image,omitempty"`
	Replicas int    `json:"replicas,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
