package vectoragent

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/utils"
)

func (vr *VectorAgentReconciler) createVectorAgentConfig(ctx context.Context) (*corev1.Secret, error) {
	cfg, err := config.Get(ctx, vr.Vector, vr.Client)
	if err != nil {
		return nil, err
	}

	cfgHash := utils.GetHash(cfg)

	if vr.Vector.Status.LastAppliedConfigHash == nil || *vr.Vector.Status.LastAppliedConfigHash != cfgHash {
		err = configcheck.Run(cfg, vr.Client, vr.Clientset, vr.Vector.Name, vr.Vector.Namespace, vr.Vector.Spec.Agent.Image)
		if _, ok := err.(*configcheck.ErrConfigCheck); ok {
			if err := setFailedStatus(ctx, vr.Vector, vr.Client, err); err != nil {
				return nil, err
			}
			return nil, err
		}
		if err != nil {
			return nil, err
		}

		if err := SetLastAppliedPipelineStatus(ctx, vr.Vector, vr.Client, &cfgHash); err != nil {
			return nil, err
		}

		if err := setSucceesStatus(ctx, vr.Vector, vr.Client); err != nil {
			return nil, err
		}

	}

	labels := vr.labelsForVectorAgent()
	config := map[string][]byte{
		"agent.json": cfg,
	}

	secret := &corev1.Secret{
		ObjectMeta: vr.objectMetaVectorAgent(labels),
		Data:       config,
	}

	return secret, nil
}
