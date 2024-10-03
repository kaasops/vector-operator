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

// ClusterVectorAggregatorSpec defines the desired state of ClusterVectorAggregator
type ClusterVectorAggregatorSpec struct {
	VectorAggregatorCommon `json:",inline"`
	// ResourceNamespace specifies the namespace where the related resources, such as Deployments and Services, will be deployed.
	ResourceNamespace string `json:"resourceNamespace,omitempty"`
}

// ClusterVectorAggregatorStatus defines the observed state of ClusterVectorAggregator
type ClusterVectorAggregatorStatus struct {
	VectorCommonStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Valid",type="boolean",JSONPath=".status.configCheckResult"

// ClusterVectorAggregator is the Schema for the clustervectoraggregators API
type ClusterVectorAggregator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterVectorAggregatorSpec   `json:"spec,omitempty"`
	Status ClusterVectorAggregatorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterVectorAggregatorList contains a list of ClusterVectorAggregator
type ClusterVectorAggregatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterVectorAggregator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterVectorAggregator{}, &ClusterVectorAggregatorList{})
}
