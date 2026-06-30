package aggregator

import (
	"context"

	policyv1 "k8s.io/api/policy/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

func (ctrl *Controller) ensureVectorAggregatorPodDisruptionBudget(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-pdb", ctrl.Name)

	// A PodDisruptionBudget only makes sense with more than one replica.
	// When autoscaling is enabled the replica count is driven by the HPA, so use
	// the configured maximum; otherwise fall back to the static replica count,
	// treating an unset value as a single replica.
	maxReplicas := int32(1)
	if ctrl.Spec.Autoscaling.Enabled {
		maxReplicas = ctrl.Spec.Autoscaling.MaxReplicas
	} else if ctrl.Spec.Replicas != nil {
		maxReplicas = *ctrl.Spec.Replicas
	}

	// When scaled to one or zero replicas, remove any PDB we previously created.
	if maxReplicas <= 1 {
		log.Info("skip Reconcile Vector Aggregator PodDisruptionBudget, effective replicas <= 1")
		pdb := ctrl.createVectorAggregatorPodDisruptionBudget()
		if err := ctrl.Client.Delete(ctx, pdb); err != nil && !api_errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	log.Info("start Reconcile Vector Aggregator PodDisruptionBudget")
	pdb := ctrl.createVectorAggregatorPodDisruptionBudget()
	return k8s.CreateOrUpdateResource(ctx, pdb, ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorPodDisruptionBudget() *policyv1.PodDisruptionBudget {
	labels := ctrl.labelsForVectorAggregator()
	matchLabels := ctrl.matchLabelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	maxUnavailable := intstr.FromInt32(1)

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
}
