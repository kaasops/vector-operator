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

	// The budget is opt-in, and only meaningful with more than one effective
	// replica. In every other case remove any budget we previously created.
	if !ctrl.Spec.PodDisruptionBudget.Enabled || maxReplicas <= 1 {
		log.Info("skip Reconcile Vector Aggregator PodDisruptionBudget, disabled or effective replicas <= 1")
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

	cfg := ctrl.Spec.PodDisruptionBudget

	spec := policyv1.PodDisruptionBudgetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: matchLabels,
		},
	}

	// A budget must set exactly one of MinAvailable or MaxUnavailable. Honor an
	// explicit choice, otherwise keep at most one pod unavailable by default.
	switch {
	case cfg.MinAvailable != nil:
		spec.MinAvailable = cfg.MinAvailable
	case cfg.MaxUnavailable != nil:
		spec.MaxUnavailable = cfg.MaxUnavailable
	default:
		maxUnavailable := intstr.FromInt32(1)
		spec.MaxUnavailable = &maxUnavailable
	}

	// Default to AlwaysAllow so not-Ready pods stay evictable regardless of the
	// budget and cannot block a node drain.
	if cfg.UnhealthyPodEvictionPolicy != nil {
		spec.UnhealthyPodEvictionPolicy = cfg.UnhealthyPodEvictionPolicy
	} else {
		policy := policyv1.AlwaysAllow
		spec.UnhealthyPodEvictionPolicy = &policy
	}

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec:       spec,
	}
}
