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

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// pipelineStatusBase returns the pipeline as the API server currently holds it, which is
// the base a status patch has to be computed against: a base taken from the in-memory
// object AFTER the test mutated it (setting the role, say) would leave that mutation out
// of the patch, since a merge patch only carries what differs from its base.
func pipelineStatusBase(p pipeline.Pipeline) pipeline.Pipeline {
	// Read into a FRESH object rather than a copy of p: a copy would arrive at the Get
	// carrying whatever the test already set locally, and whether those fields survive
	// the decode is a detail of the client's serializer, not something this helper's
	// contract should rest on.
	var base pipeline.Pipeline
	switch p.(type) {
	case *v1alpha1.VectorPipeline:
		base = &v1alpha1.VectorPipeline{}
	case *v1alpha1.ClusterVectorPipeline:
		base = &v1alpha1.ClusterVectorPipeline{}
	default:
		ExpectWithOffset(1, p).To(BeNil(), fmt.Sprintf("pipelineStatusBase: unsupported pipeline type %T", p))
	}
	ExpectWithOffset(1, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(p), base)).To(Succeed())
	return base
}
