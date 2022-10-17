package vectoragent

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
	"github.com/kaasops/vector-operator/controllers/factory/vectorpipeline"
)

func createVectorAgentConfig(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client) (*corev1.Secret, error) {
	cfg, err := getConfig(ctx, v, c)
	if err != nil {
		return nil, err
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

func getConfig(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client) ([]byte, error) {
	vps, err := vectorpipeline.Select(ctx, c)
	if err != nil {
		return nil, err
	}
	cfg, err := vector.GenerateConfig(v, vps)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
