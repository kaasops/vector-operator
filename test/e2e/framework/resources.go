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

package framework

import (
	"github.com/kaasops/vector-operator/test/e2e/framework/assertions"
)

// Pipeline returns a pipeline resource wrapper for custom matchers
func (f *Framework) Pipeline(name string) *assertions.PipelineResource {
	return assertions.NewPipelineResource(f.namespace, name)
}

// ClusterPipeline returns a cluster-scoped pipeline resource wrapper for custom matchers
func (f *Framework) ClusterPipeline(name string) *assertions.PipelineResource {
	return assertions.NewPipelineResource("", name)
}

// Service returns a service resource wrapper for custom matchers
func (f *Framework) Service(name string) *assertions.ServiceResource {
	return assertions.NewServiceResource(f.namespace, name)
}
