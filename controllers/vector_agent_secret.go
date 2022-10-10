package controllers

import (
	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

var vectorAgentConfig = `
data_dir: /vector-data-dir
api:
  enabled: true
  address: 127.0.0.1:8686
  playground: false
sources:
  kubernetes_logs:
    type: kubernetes_logs
sinks:
  stdout:
    type: console
    inputs: [kubernetes_logs]
    encoding:
      codec: json
  vector:
    type: vector
    inputs: [kubernetes_logs]
    address: vector-sample-aggregator:6000
`

func (r *VectorReconciler) createVectorAgentSecret(v *vectorv1alpha1.Vector) *corev1.Secret {
	labels := labelsForVectorAgent(v.Name)

	config := map[string][]byte{
		"agent.yaml": []byte(vectorAgentConfig),
	}

	secret := &corev1.Secret{
		ObjectMeta: objectMetaVectorAgent(v, labels),
		Data:       config,
	}
	return secret
}
