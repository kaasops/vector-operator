package vectoragent

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

func (ctrl *Controller) createVectorAgentServiceMonitor() *monitorv1.ServiceMonitor {
	labels := ctrl.labelsForVectorAgent()

	servicemonitor := &monitorv1.ServiceMonitor{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels),
		Spec: monitorv1.ServiceMonitorSpec{
			Endpoints: []monitorv1.Endpoint{
				{
					Path: "/metrics",
					Port: "vectoragent-metrics",
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	return servicemonitor
}
