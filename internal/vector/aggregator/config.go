package aggregator

import (
	"context"
	"github.com/kaasops/vector-operator/internal/utils/compression"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type globalOptions struct {
	ExpireMetricsSecs *int `yaml:"expire_metrics_secs,omitempty"`
}

func (ctrl *Controller) ensureVectorAggregatorConfig(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-secret", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator Secret")

	vectorAggregatorSecret, err := ctrl.createVectorAggregatorConfig(ctx)
	if err != nil {
		return err
	}

	return k8s.CreateOrUpdateResource(ctx, vectorAggregatorSecret, ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorConfig(ctx context.Context) (*corev1.Secret, error) {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-config", ctrl.Name)
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()
	data := ctrl.ConfigBytes

	if ctrl.Spec.CompressConfigFile {
		data = compression.Compress(ctrl.ConfigBytes, log)
	}
	config := map[string][]byte{
		"config.json": data,
	}
	secret := &corev1.Secret{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Data:       config,
	}
	return secret, nil
}
