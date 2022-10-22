package configcheck

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createVectorConfigCheckConfig(cfg []byte, name, ns, hash string) (*corev1.Secret, error) {
	labels := labelsForVectorConfigCheck()

	config := map[string][]byte{
		"agent.json": cfg,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getNameVectorConfigCheck(name, hash),
			Namespace: ns,
			Labels:    labels,
		},
		Data: config,
	}

	return secret, nil
}
