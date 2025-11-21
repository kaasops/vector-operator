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

package k8s

const (
	// The following labels are recommended by kubernetes https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/

	// ManagedByLabelKey is Kubernetes recommended label key, it represents the tool being used to manage the operation of an application
	// For resources managed by SeaweedFS Operator, its value is always seaweedfs-operator
	ManagedByLabelKey string = "app.kubernetes.io/managed-by"
	// ComponentLabelKey is Kubernetes recommended label key, it represents the component within the architecture
	ComponentLabelKey string = "app.kubernetes.io/component"
	// NameLabelKey is Kubernetes recommended label key, it represents the name of the application
	NameLabelKey string = "app.kubernetes.io/name"
	// InstanceLabelKey is Kubernetes recommended label key, it represents a unique name identifying the instance of an application
	// It's set by helm when installing a release
	InstanceLabelKey string = "app.kubernetes.io/instance"
	// VersionLabelKey is Kubernetes recommended label key, it represents the version of the app
	VersionLabelKey    string = "app.kubernetes.io/version"
	VectorExcludeLabel string = "vector.dev/exclude"

	// PodName is to select pod by name
	// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#pod-selector
	PodName string = "statefulset.kubernetes.io/pod-name"
)

// MergeLabels merges two sets of Kubernetes labels, with the source (src) labels
// being merged into the destination (dst) labels. If a key exists in both maps,
// the destination value is preserved.
func MergeLabels(dst, src map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string)
	}

	if src == nil {
		return dst
	}

	for k, v := range src {
		if _, ok := dst[k]; !ok {
			dst[k] = v
		}
	}
	return dst
}
