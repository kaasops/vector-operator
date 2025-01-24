package aggregator

import (
	"context"
	"github.com/kaasops/vector-operator/internal/utils/compression"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type globalOptions struct {
	ExpireMetricsSecs *int `yaml:"expire_metrics_secs,omitempty"`
}

func (ctrl *Controller) ensureVectorAggregatorConfig(ctx context.Context) (bool, error) {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-secret", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator Secret")

	vectorAggregatorSecret, err := ctrl.createVectorAggregatorConfig(ctx)
	if err != nil {
		return false, err
	}

	globalOptionsChanged := false

	var prevSecret corev1.Secret
	key := types.NamespacedName{Namespace: vectorAggregatorSecret.Namespace, Name: vectorAggregatorSecret.Name}
	if err = ctrl.Get(ctx, key, &prevSecret); err == nil {
		// check that the global settings have not been changed
		var prevConfig globalOptions
		data := prevSecret.Data["config.json"]
		if ctrl.Spec.CompressConfigFile {
			data = compression.Decompress(data, log)
		}

		err = yaml.Unmarshal(data, &prevConfig)
		if err == nil {
			var actualConfig globalOptions
			err = yaml.Unmarshal(ctrl.ConfigBytes, &actualConfig)
			if err == nil {
				if actualConfig.ExpireMetricsSecs == nil && prevConfig.ExpireMetricsSecs != nil {
					globalOptionsChanged = true
				}
				if actualConfig.ExpireMetricsSecs != nil && prevConfig.ExpireMetricsSecs == nil {
					globalOptionsChanged = true
				}
				if actualConfig.ExpireMetricsSecs != nil &&
					prevConfig.ExpireMetricsSecs != nil &&
					*actualConfig.ExpireMetricsSecs != *prevConfig.ExpireMetricsSecs {
					globalOptionsChanged = true
				}
			}
		}
	}

	return globalOptionsChanged, k8s.CreateOrUpdateResource(ctx, vectorAggregatorSecret, ctrl.Client)
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
