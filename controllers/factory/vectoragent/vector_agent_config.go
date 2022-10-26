package vectoragent

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/utils"
)

func createVectorAgentConfig(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client, cs *kubernetes.Clientset) (*corev1.Secret, error) {
	cfg, err := config.Get(ctx, v, c)
	if err != nil {
		return nil, err
	}

	cfgHash := utils.GetHash(cfg)

	if v.Status.LastAppliedConfigHash == nil || *v.Status.LastAppliedConfigHash != cfgHash {
		err = configcheck.Run(cfg, c, cs, v.Name, v.Namespace, v.Spec.Agent.Image)
		if _, ok := err.(*configcheck.ErrConfigCheck); ok {
			if err := setFailedStatus(ctx, v, c, err); err != nil {
				return nil, err
			}
			return nil, err
		}
		if err != nil {
			return nil, err
		}

		if err := SetLastAppliedPipelineStatus(ctx, v, c, &cfgHash); err != nil {
			return nil, err
		}

		if err := setSucceesStatus(ctx, v, c); err != nil {
			return nil, err
		}

	}

	labels := labelsForVectorAgent(v.Name)
	config := map[string][]byte{
		"agent.json": cfg,
	}

	secret := &corev1.Secret{
		ObjectMeta: objectMetaVectorAgent(v, labels),
		Data:       config,
	}

	return secret, nil
}
