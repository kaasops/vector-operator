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

package pipeline

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// SecretIndex is a thread-safe, bidirectional index of which pipelines declare which
// Kubernetes Secrets. PipelineReconciler keeps it current on every reconcile of a
// pipeline that has spec.secret; the Secret watch (a structural watch that reuses
// the workload reconcilers' existing Secret informer, see pipeline_controller.go
// SetupWithManager) uses PipelinesFor to resolve which pipelines to requeue when a
// Secret changes - it only needs the Secret's identity, never its payload.
//
// For a cluster-scoped ClusterVectorPipeline, callers key the pipeline side with
// Namespace: "" (matching reconcile.Request.NamespacedName for cluster-scoped
// resources).
type SecretIndex struct {
	mu                sync.RWMutex
	pipelineToSecrets map[types.NamespacedName]map[types.NamespacedName]struct{}
	secretToPipelines map[types.NamespacedName]map[types.NamespacedName]struct{}
}

func NewSecretIndex() *SecretIndex {
	return &SecretIndex{
		pipelineToSecrets: make(map[types.NamespacedName]map[types.NamespacedName]struct{}),
		secretToPipelines: make(map[types.NamespacedName]map[types.NamespacedName]struct{}),
	}
}

// Set replaces the full set of secrets a pipeline declares. It is a full-replace, not
// an incremental Add/Delete: any previously linked secret absent from secrets is
// unlinked, and a nil/empty secrets removes the pipeline from the index entirely.
func (si *SecretIndex) Set(pipeline types.NamespacedName, secrets []types.NamespacedName) {
	si.mu.Lock()
	defer si.mu.Unlock()

	newSet := make(map[types.NamespacedName]struct{}, len(secrets))
	for _, s := range secrets {
		newSet[s] = struct{}{}
	}

	for old := range si.pipelineToSecrets[pipeline] {
		if _, keep := newSet[old]; keep {
			continue
		}
		delete(si.secretToPipelines[old], pipeline)
		if len(si.secretToPipelines[old]) == 0 {
			delete(si.secretToPipelines, old)
		}
	}

	if len(newSet) == 0 {
		delete(si.pipelineToSecrets, pipeline)
		return
	}

	si.pipelineToSecrets[pipeline] = newSet
	for s := range newSet {
		if si.secretToPipelines[s] == nil {
			si.secretToPipelines[s] = make(map[types.NamespacedName]struct{})
		}
		si.secretToPipelines[s][pipeline] = struct{}{}
	}
}

// PipelinesFor returns the pipelines currently declaring secret.
func (si *SecretIndex) PipelinesFor(secret types.NamespacedName) []types.NamespacedName {
	si.mu.RLock()
	defer si.mu.RUnlock()

	pipelines := si.secretToPipelines[secret]
	list := make([]types.NamespacedName, 0, len(pipelines))
	for p := range pipelines {
		list = append(list, p)
	}
	return list
}
