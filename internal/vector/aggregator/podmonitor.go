package aggregator

import (
	"context"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

func (ctrl *Controller) ensureVectorAggregatorPodMonitor(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-podmonitor", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator PodMonitor")
	vectorAggregatorPodMonitor := ctrl.createVectorAggregatorPodMonitor()
	return k8s.CreateOrUpdateResource(ctx, vectorAggregatorPodMonitor, ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorPodMonitor() *monitorv1.PodMonitor {
	labels := ctrl.labelsForVectorAggregator()
	matchLabels := ctrl.matchLabelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	endpoint := monitorv1.PodMetricsEndpoint{
		Path: "/metrics",
		Port: "prom-exporter",
	}

	if ctrl.Spec.ScrapeInterval != "" {
		endpoint.Interval = monitorv1.Duration(ctrl.Spec.ScrapeInterval)
	}
	if ctrl.Spec.ScrapeTimeout != "" {
		endpoint.ScrapeTimeout = monitorv1.Duration(ctrl.Spec.ScrapeTimeout)
	}

	podmonitor := &monitorv1.PodMonitor{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: monitorv1.PodMonitorSpec{
			PodMetricsEndpoints: []monitorv1.PodMetricsEndpoint{endpoint},
			Selector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}

	return podmonitor
}
