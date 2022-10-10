package controllers

import (
	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *VectorReconciler) createVectorAggregatorService(v *vectorv1alpha1.Vector) *corev1.Service {
	labels := labelsForVectorAggregator(v.Name)

	service := &corev1.Service{
		ObjectMeta: objectMetaVectorAggregator(v, labels),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "vector",
					Protocol:   corev1.Protocol("TCP"),
					Port:       6000,
					TargetPort: intstr.FromInt(6000),
				},
			},
			Selector: labels,
		},
	}
	return service
}
