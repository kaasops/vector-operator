package vectoragent

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

func (ctrl *Controller) createVectorAgentPodMonitor() *monitorv1.PodMonitor {
	labels := ctrl.labelsForVectorAgent()
	matchLabels := ctrl.matchLabelsForVectorAgent()
	annotations := ctrl.annotationsForVectorAgent()

	endpoint := monitorv1.PodMetricsEndpoint{
		Path: "/metrics",
		Port: "prom-exporter",
	}

	if ctrl.Vector.Spec.Agent.ScrapeInterval != "" {
		endpoint.Interval = monitorv1.Duration(ctrl.Vector.Spec.Agent.ScrapeInterval)
	}
	if ctrl.Vector.Spec.Agent.ScrapeTimeout != "" {
		endpoint.ScrapeTimeout = monitorv1.Duration(ctrl.Vector.Spec.Agent.ScrapeTimeout)
	}

	podmonitor := &monitorv1.PodMonitor{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels, annotations, ctrl.Vector.Namespace),
		Spec: monitorv1.PodMonitorSpec{
			PodMetricsEndpoints: []monitorv1.PodMetricsEndpoint{endpoint},
			Selector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}

	return podmonitor
}
