package vectorpipeline

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SelectSucceesed(ctx context.Context, rclient client.Client) (map[string]*vectorv1alpha1.VectorPipeline, error) {

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
		if item.Status.ConfigCheckResult != nil {
			if *item.Status.ConfigCheckResult {
				vectorPipelinesCombined = append(vectorPipelinesCombined, item)
			}
		}

	}

	for _, vectorPipeline := range vectorPipelinesCombined {
		m := vectorPipeline.DeepCopy()
		res[generateName(&vectorPipeline)] = m
	}
	return res, nil
}

func generateName(vp *vectorv1alpha1.VectorPipeline) string {
	return vp.Namespace + "-" + vp.Name
}

func SetSucceesStatus(ctx context.Context, vp *vectorv1alpha1.VectorPipeline, c client.Client) {
	var status = true
	vp.Status.ConfigCheckResult = &status
	k8sutils.UpdateStatus(ctx, vp, c)
}

func SetFailedStatus(ctx context.Context, vp *vectorv1alpha1.VectorPipeline, c client.Client) {
	var status = false
	vp.Status.ConfigCheckResult = &status
	k8sutils.UpdateStatus(ctx, vp, c)
}
