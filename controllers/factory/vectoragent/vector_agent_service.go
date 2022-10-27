package vectoragent

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
)

func (vr *VectorAgentReconciler) createVectorAgentService() *corev1.Service {
	labels := vr.labelsForVectorAgent()

	service := &corev1.Service{
		ObjectMeta: vr.objectMetaVectorAgent(labels),
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
