package vectoragent

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
)

func createVectorAgentConfig(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client, cs *kubernetes.Clientset) (*corev1.Secret, error) {
	cfg, err := config.Get(ctx, v, c)
	if err != nil {
		return nil, err
	}

	err = configcheck.Run(cfg, c, cs, v.Name, v.Namespace, v.Spec.Agent.Image)
	if _, ok := err.(*configcheck.ErrConfigCheck); ok {
		setFailedStatus(ctx, v, c, err)
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	setSucceesStatus(ctx, v, c)

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
