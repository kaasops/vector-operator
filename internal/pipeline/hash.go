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

package pipeline

import (
	"encoding/json"
	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/utils/hash"
)

type tmp struct {
	Spec        v1alpha1.VectorPipelineSpec
	Labels      map[string]string
	ServiceName string
}

func GegPipelineHash(pipeline Pipeline) (*uint32, error) {
	a, err := json.Marshal(tmp{
		Spec:        pipeline.GetSpec(),
		Labels:      pipeline.GetLabels(),
		ServiceName: pipeline.GetAnnotations()[common.AnnotationServiceName],
	})
	if err != nil {
		return nil, err
	}
	hash := hash.Get(a)
	return &hash, nil
}

// IsPipelineChanged returns true, if hash in .status.lastAppliedPipelineHash matches with spec Hash
func IsPipelineChanged(pipeline Pipeline) (bool, error) {
	hash, err := GegPipelineHash(pipeline)
	if err != nil {
		return false, err
	}

	if pipeline.GetLastAppliedPipeline() != nil && *hash == *pipeline.GetLastAppliedPipeline() {
		return true, nil
	}

	return false, nil
}
