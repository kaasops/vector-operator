package vectorpipeline

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Select(ctx context.Context, rclient client.Client) (map[string]*vectorv1alpha1.VectorPipeline, error) {

	res := make(map[string]*vectorv1alpha1.VectorPipeline)

	var vectorPipelinesCombined []vectorv1alpha1.VectorPipeline

	objlist := vectorv1alpha1.VectorPipelineList{}
	err := rclient.List(ctx, &objlist)
	if err != nil {
		return nil, err
	}

	for _, item := range objlist.Items {
		if !item.DeletionTimestamp.IsZero() {
			continue
		}
		vectorPipelinesCombined = append(vectorPipelinesCombined, item)
	}

	for _, vectorPipeline := range vectorPipelinesCombined {
		m := vectorPipeline.DeepCopy()
		res[generateName(&vectorPipeline)] = m
	}
	return res, nil
}

func generateName(pipelineCR *vectorv1alpha1.VectorPipeline) string {
	return pipelineCR.Namespace + "-" + pipelineCR.Name
}
