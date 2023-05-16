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

package configcheck

import (
	"context"

	"github.com/kaasops/vector-operator/controllers/factory/utils/compression"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (cc *ConfigCheck) createVectorConfigCheckConfig(ctx context.Context) (*corev1.Secret, error) {
	log := log.FromContext(ctx).WithValues("Vector ConfigCheck", cc.Initiator)
	labels := labelsForVectorConfigCheck()
	var data []byte = cc.Config

	if cc.CompressedConfig {
		data = compression.Compress(cc.Config, log)
	}

	config := map[string][]byte{
		"agent.json": data,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cc.getNameVectorConfigCheck(),
			Namespace: cc.Namespace,
			Labels:    labels,
		},
		Data: config,
	}

	return secret, nil
}
