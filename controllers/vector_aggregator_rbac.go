package controllers

import (
	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func (r *VectorReconciler) createVectorAggregatorServiceAccount(v *vectorv1alpha1.Vector) *corev1.ServiceAccount {
	labels := labelsForVectorAggregator(v.Name)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: objectMetaVectorAggregator(v, labels),
	}

	return serviceAccount
}
