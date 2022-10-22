package configcheck

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createVectorConfigCheckServiceAccount(ns string) *corev1.ServiceAccount {
	labels := labelsForVectorConfigCheck()

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vector-configcheck",
			Namespace: ns,
			Labels:    labels,
		},
	}

	return serviceAccount
}
