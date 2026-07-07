package aggregator

import (
	"context"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

func (ctrl *Controller) ensureVectorAggregatorHPA(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-hpa", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator HPA")
	if !ctrl.Spec.Autoscaling.Enabled {
		return ctrl.cleanupHPA(ctx)
	}
	hpa := ctrl.createVectorAggregatorHPA()

	return k8s.CreateOrUpdateResource(ctx, hpa, ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorHPA() *autoscalingv2.HorizontalPodAutoscaler {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	// The aggregator is a StatefulSet in persistent mode and a Deployment otherwise.
	// Both support the scale subresource, so the HPA just needs the right kind.
	targetKind := "Deployment"
	if ctrl.persistenceEnabled() {
		targetKind = "StatefulSet"
	}

	HPA := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: ctrl.Spec.Autoscaling.MinReplicas,
			MaxReplicas: ctrl.Spec.Autoscaling.MaxReplicas,
			Behavior:    ctrl.Spec.Autoscaling.Behavior,
			Metrics:     ctrl.Spec.Autoscaling.Metrics,
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       targetKind,
				Name:       ctrl.getNameVectorAggregator(),
				APIVersion: "apps/v1",
			},
		},
	}

	return HPA
}

func (ctrl *Controller) cleanupHPA(ctx context.Context) error {
	if err := ctrl.Delete(ctx, ctrl.createVectorAggregatorHPA()); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	return nil
}
