package v1alpha1

import v1 "k8s.io/api/core/v1"

type VectorCommonStatus struct {
	ConfigCheckResult     *bool   `json:"configCheckResult,omitempty"`
	Reason                *string `json:"reason,omitempty"`
	LastAppliedConfigHash *uint32 `json:"LastAppliedConfigHash,omitempty"`
}

type VectorCommon struct {
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
	// Vector will expire internal metrics that haven’t been emitted/updated in the configured interval (default 300 seconds).
	// https://vector.dev/docs/reference/configuration/global-options/#expire_metrics_secs
	ExpireMetricsSecs int `json:"expireMetricsSecs,omitempty"`
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
	// If probe enabled via VectorAgent or VectorAggregator, this setting will be ignored for that probe.
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

type VectorSelectorSpec struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

type VectorAggregatorCommon struct {
	VectorCommon `json:",inline"`
	Replicas     int32 `json:"replicas,omitempty"`
	// Selector defines a filter for the Vector Pipeline and Cluster Vector Pipeline by labels.
	// If not specified, all pipelines will be selected.
	Selector       *VectorSelectorSpec `json:"selector,omitempty"`
	EventCollector EventCollector      `json:"eventCollector,omitempty"`
}

type EventCollector struct {
	Image           string        `json:"image,omitempty"`
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	MaxBatchSize    int32         `json:"maxBatchSize,omitempty"`
}
