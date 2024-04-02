package vectoragent

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

func (ctrl *Controller) createVectorAgentPodMonitor() *monitorv1.PodMonitor {
	labels := ctrl.labelsForVectorAgent()
	annotations := ctrl.annotationsForVectorAgent()

	podmonitor := &monitorv1.PodMonitor{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels, annotations, ctrl.Vector.Namespace),
		Spec: monitorv1.PodMonitorSpec{
			PodMetricsEndpoints: []monitorv1.PodMetricsEndpoint{
				{
					Path: "/metrics",
					Port: "prom-exporter",
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	return podmonitor
}
