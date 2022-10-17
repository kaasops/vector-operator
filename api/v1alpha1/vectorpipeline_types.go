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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VectorPipelineSpec defines the desired state of VectorPipeline
type VectorPipelineSpec struct {
	Source     map[string]SourceSpec    `json:"source,omitempty"`
	Transforms map[string]TransformSpec `json:"transforms,omitempty"`
	Sink       map[string]SinkSpec      `json:"sinks,omitempty"`
}

type TransformSpec struct {
	Type      string     `json:"type,omitempty"`
	Inputs    []string   `json:"inputs,omitempty"`
	Condition *Condition `json:"condition,omitempty"`
}

type Condition struct {
	Type   string `json:"type,omitempty"`
	Source string `json:"source,omitempty"`
}

type SourceSpec struct {
	Type               string `json:"type,omitempty"`
	ExtraLabelSelector string `json:"extra_label_selector,omitempty"`
	ExtraFieldSelector string `json:"extra_field_selector,omitempty"`
}

type SinkSpec struct {
	Type              string    `json:"type,omitempty"`
	Address           string    `json:"address,omitempty"`
	Inputs            []string  `json:"inputs,omitempty"`
	Encoding          *Encoding `json:"encoding,omitempty"`
	Rate              *int32    `json:"rate,omitempty"`
	PrintIntervalSecs int32     `json:"print_interval_secs,omitempty"`
}

type Encoding struct {
	Codec string `json:"codec,omitempty"`
}

// VectorPipelineStatus defines the observed state of VectorPipeline
type VectorPipelineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
