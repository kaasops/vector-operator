package controllers

import (
	corev1 "k8s.io/api/core/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *VectorReconciler) createVectorAgentService(v *vectorv1alpha1.Vector) *corev1.Service {
	labels := labelsForVectorAgent(v.Name)

	service := &corev1.Service{
		ObjectMeta: objectMetaVectorAgent(v, labels),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "prom-exporter",
					Protocol:   corev1.Protocol("TCP"),
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
				},
			},
			Selector: labels,
		},
	}
	return service
}
