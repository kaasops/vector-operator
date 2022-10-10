package controllers

import (
	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

var vectorAggregatorConfig = `
data_dir: /vector-data-dir
api:
  enabled: true
  address: 127.0.0.1:8686
  playground: false
sources:
  vector:
    address: 0.0.0.0:6000
    type: vector
    version: "2"
sinks:
  stdout:
    type: console
    inputs: [vector]
    encoding:
      codec: json
`

func (r *VectorReconciler) createVectorAggregatorSecret(v *vectorv1alpha1.Vector) *corev1.Secret {
	labels := labelsForVectorAggregator(v.Name)

	config := map[string][]byte{
		"aggregator.yaml": []byte(vectorAggregatorConfig),
	}

	secret := &corev1.Secret{
		ObjectMeta: objectMetaVectorAggregator(v, labels),
		Data:       config,
	}
	return secret
}
