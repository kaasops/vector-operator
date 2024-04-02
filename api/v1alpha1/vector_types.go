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
	// Determines if requests to the kube-apiserver can be served by a cache.
	UseApiServerCache bool `json:"useApiServerCache,omitempty"`
	// Vector Aggregator
	// Aggregator *VectorAggregator `json:"aggregator,omitempty"`
}

// VectorStatus defines the observed state of Vector
type VectorStatus struct {
	ConfigCheckResult     *bool   `json:"configCheckResult,omitempty"`
	Reason                *string `json:"reason,omitempty"`
	LastAppliedConfigHash *uint32 `json:"LastAppliedConfigHash,omitempty"`
}

// VectorAgent is the Schema for the Vector Agent
type VectorAgent struct {
	// Image - docker image settings for Vector Agent
	// if no specified operator uses default config version
	// +optional
	Image string `json:"image,omitempty"`
	// ImagePullSecrets An optional list of references to secrets in the same namespace
	// to use for pulling images from registries
	// see http://kubernetes.io/docs/user-guide/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// ImagePullPolicy of pods
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Annotations is an unstructured key value map stored with a resource that may be set by external tools to store and retrieve arbitrary metadata.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Resources container resource request and limits, https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// if not specified - default setting will be used
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resources",xDescriptors="urn:alm:descriptor:com.tectonic.ui:resourceRequirements"
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Affinity If specified, the pod's scheduling constraints.
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`
	// Tolerations If specified, the pod's tolerations.
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// SecurityContext holds pod-level security attributes and common container settings.
	// This defaults to the default PodSecurityContext.
	// +optional
	SecurityContext *v1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	// SecurityContext holds security configuration that will be applied to a container.
	// Some fields are present in both SecurityContext and PodSecurityContext.
	// When both are set, the values in SecurityContext take precedence.
	// +optional
	ContainerSecurityContext *v1.SecurityContext `json:"containerSecurityContext,omitempty"`
	// SchedulerName - defines kubernetes scheduler name
	// +optional
	SchedulerName string `json:"schedulerName,omitempty"`
	// RuntimeClassName - defines runtime class for kubernetes pod.
	// https://kubernetes.io/docs/concepts/containers/runtime-class/
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`
	// HostAliases provides mapping between ip and hostnames,
	// that would be propagated to pod,
	// cannot be used with HostNetwork.
	// +optional
	HostAliases []v1.HostAlias `json:"host_aliases,omitempty"`
	// PodSecurityPolicyName - defines name for podSecurityPolicy
	// in case of empty value, prefixedName will be used.
	// +optional
	PodSecurityPolicyName string `json:"podSecurityPolicyName,omitempty"`
	// PriorityClassName assigned to the Pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// HostNetwork controls whether the pod may use the node network namespace
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// Env that will be added to Vector pod
	Env []v1.EnvVar `json:"env,omitempty"`
	// The directory used for persisting Vector state, such as on-disk buffers, file checkpoints, and more. Please make sure the Vector project has write permissions to this directory.
	// https://vector.dev/docs/reference/configuration/global-options/#data_dir
	DataDir string `json:"dataDir,omitempty"`
	// Vector API params. Allows to interact with a running Vector instance.
	// https://vector.dev/docs/reference/api/
	Api ApiSpec `json:"api,omitempty"`
	// Enable internal metrics exporter
	// +optional
	InternalMetrics bool `json:"internalMetrics,omitempty"`
	// List of volumes that can be mounted by containers belonging to the pod.
	// +optional
	Volumes []v1.Volume `json:"volumes,omitempty"`
	// Periodic probe of container service readiness. Container will be removed from service endpoints if the probe fails.
	// +optional
	ReadinessProbe *v1.Probe `json:"readinessProbe,omitempty"`
	// Periodic probe of container liveness. Container will be restarted if the probe fails.
	// +optional
	LivenessProbe *v1.Probe `json:"livenessProbe,omitempty"`
	// Pod volumes to mount into the container's filesystem.
	// +optional
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`
	// Control params for ConfigCheck pods
	ConfigCheck ConfigCheck `json:"configCheck,omitempty"`
	// Compress config file to fix: metadata.annotations: Too long: must have at most 262144 characters
	CompressConfigFile      bool                    `json:"compressConfigFile,omitempty"`
	ConfigReloaderImage     string                  `json:"configReloaderImage,omitempty"`
	ConfigReloaderResources v1.ResourceRequirements `json:"configReloaderResources,omitempty"`
}

// ApiSpec is the Schema for the Vector Agent GraphQL API - https://vector.dev/docs/reference/api/
type ApiSpec struct {
	Enabled    bool `json:"enabled,omitempty"`
	Playground bool `json:"playground,omitempty"`
	// Enable ReadinessProbe and LivenessProbe via api /health endpoint.
	// If probe enabled via VectorAgent, this setting will be ignored for that probe.
	// +optional
	Healthcheck bool `json:"healthcheck,omitempty"`
}

// ConfigCheck is the Schema for control params for ConfigCheck pods
type ConfigCheck struct {
	Disabled bool `json:"disabled,omitempty"`
	// Image - docker image settings for Vector Agent
	// if no specified operator uses default config version
	// +optional
	Image *string `json:"image,omitempty"`
	// Resources container resource request and limits, https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// if not specified - default setting will be used
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resources",xDescriptors="urn:alm:descriptor:com.tectonic.ui:resourceRequirements"
	// +optional
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Affinity If specified, the pod's scheduling constraints.
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`
	// Tolerations If specified, the pod's tolerations.
	// +optional
	Tolerations *[]v1.Toleration `json:"tolerations,omitempty"`
	// Annotations is an unstructured key value map stored with a resource that may be set by external tools to store and retrieve arbitrary metadata.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
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
