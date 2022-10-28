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

	"github.com/kaasops/vector-operator/controllers/factory/utils/hash"
)

func (ctrl *Controller) GetSpecHash() (*uint32, error) {
	a, err := json.Marshal(ctrl.Pipeline.Spec())
	if err != nil {
		return nil, err
	}
	hash := hash.Get(a)
	return &hash, nil
}

// CheckHash returns true, if hash in .status.lastAppliedPipelineHash matches with spec Hash
func (ctrl *Controller) CheckHash() (bool, error) {
	hash, err := ctrl.GetSpecHash()
	if err != nil {
		return false, err
	}

	if ctrl.Pipeline.GetLastAppliedPipeline() != nil && *hash == *ctrl.Pipeline.GetLastAppliedPipeline() {
		return true, nil
	}

	return false, nil
}
