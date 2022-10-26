package vectorpipeline

import (
	"context"
	"encoding/json"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
	"github.com/mitchellh/mapstructure"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SelectSucceesed(ctx context.Context, rclient client.Client) ([]*vectorv1alpha1.VectorPipeline, error) {

	var vectorPipelinesCombined []*vectorv1alpha1.VectorPipeline

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
				vectorPipelinesCombined = append(vectorPipelinesCombined, item.DeepCopy())
			}
		}

	}
	return vectorPipelinesCombined, nil
}

func SetSucceesStatus(ctx context.Context, vp *vectorv1alpha1.VectorPipeline, c client.Client) error {
	var status = true
	vp.Status.ConfigCheckResult = &status
	vp.Status.Reason = nil

	return k8sutils.UpdateStatus(ctx, vp, c)
}

func SetFailedStatus(ctx context.Context, vp *vectorv1alpha1.VectorPipeline, c client.Client, err error) error {
	var status = false
	var reason = err.Error()
	vp.Status.ConfigCheckResult = &status
	vp.Status.Reason = &reason

	return k8sutils.UpdateStatus(ctx, vp, c)
}

func SetLastAppliedPipelineStatus(ctx context.Context, vp *vectorv1alpha1.VectorPipeline, c client.Client) error {
	hash, err := GetVpSpecHash(vp)
	if err != nil {
		return err
	}
	vp.Status.LastAppliedPipelineHash = hash
	if err := k8sutils.UpdateStatus(ctx, vp, c); err != nil {
		return err
	}
	return nil
}

func GetSources(vp *vectorv1alpha1.VectorPipeline, filter []string) ([]vector.Source, error) {
	var sources []vector.Source
	sourcesMap, err := decodeRaw(vp.Spec.Sources.Raw)
	if err != nil {
		return nil, err
	}
	for k, v := range sourcesMap {
		if len(filter) != 0 {
			if !contains(filter, k) {
				continue
			}
		}
		var source vector.Source
		if err := mapstructure.Decode(v, &source); err != nil {
			return nil, err
		}
		source.Name = addPrefix(vp, k)
		sources = append(sources, source)
	}
	return sources, nil
}

func GetTransforms(vp *vectorv1alpha1.VectorPipeline) ([]vector.Transform, error) {
	if vp.Spec.Transforms == nil {
		return nil, nil
	}
	transformsMap, err := decodeRaw(vp.Spec.Transforms.Raw)
	if err != nil {
		return nil, err
	}
	var transforms []vector.Transform
	if err := json.Unmarshal(vp.Spec.Transforms.Raw, &transformsMap); err != nil {
		return nil, err
	}
	for k, v := range transformsMap {
		var transform vector.Transform
		if err := mapstructure.Decode(v, &transform); err != nil {
			return nil, err
		}
		transform.Name = addPrefix(vp, k)
		for i, inputName := range transform.Inputs {
			transform.Inputs[i] = addPrefix(vp, inputName)
		}
		transforms = append(transforms, transform)
	}
	return transforms, nil
}

func GetSinks(vp *vectorv1alpha1.VectorPipeline) ([]vector.Sink, error) {
	sinksMap, err := decodeRaw(vp.Spec.Sinks.Raw)
	if err != nil {
		return nil, err
	}
	var sinks []vector.Sink
	for k, v := range sinksMap {
		var sink vector.Sink
		if err := mapstructure.Decode(v, &sink); err != nil {
			return nil, err
		}
		sink.Name = addPrefix(vp, k)
		for i, inputName := range sink.Inputs {
			sink.Inputs[i] = addPrefix(vp, inputName)
		}
		sinks = append(sinks, sink)
	}
	return sinks, nil
}

func decodeRaw(raw []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func addPrefix(vp *vectorv1alpha1.VectorPipeline, name string) string {
	return generateName(vp) + "-" + name
}

func generateName(vp *vectorv1alpha1.VectorPipeline) string {
	return vp.Namespace + "-" + vp.Name
}

func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}
