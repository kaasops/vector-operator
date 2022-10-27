/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pipeline

import (
	"context"
	"encoding/json"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline/clustervectorpipeline"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline/vectorpipeline"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
	"github.com/mitchellh/mapstructure"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Pipeline interface {
	Spec() vectorv1alpha1.VectorPipelineSpec
	Name() string
	Namespace() string
	Type() string
	SetConfigCheck(bool)
	SetReason(*string)
	GetLastAppliedPipeline() *uint32
	SetLastAppliedPipeline(*uint32)
	UpdateStatus(context.Context, client.Client) error
}

type Controller struct {
	Pipeline Pipeline
	Ctx      context.Context
	Client   client.Client
}

func NewController(ctx context.Context, c client.Client, p Pipeline) *Controller {
	return &Controller{
		Pipeline: p,
		Ctx:      ctx,
		Client:   c,
	}
}

func (ctrl *Controller) SelectSucceesed() ([]Controller, error) {
	var combined []Controller

	vpCombined, err := ctrl.SelectVectorPipelineSucceesed()
	if err != nil {
		return nil, err
	}

	cvpCombined, err := ctrl.SelectClusterVectorPipelineSucceesed()
	if err != nil {
		return nil, err
	}

	for _, vp := range vpCombined {
		combined = append(combined, Controller{
			Pipeline: vp,
		})
	}
	for _, cvp := range cvpCombined {
		combined = append(combined, Controller{
			Pipeline: cvp,
		})
	}

	return combined, nil

}

func (ctrl *Controller) SelectVectorPipelineSucceesed() ([]*vectorpipeline.Controller, error) {
	var vpCombined []*vectorpipeline.Controller

	vps := vectorv1alpha1.VectorPipelineList{}
	err := ctrl.Client.List(ctrl.Ctx, &vps)
	if err != nil {
		return nil, err
	}

	for _, vp := range vps.Items {
		if !vp.DeletionTimestamp.IsZero() {
			continue
		}
		if vp.Status.ConfigCheckResult != nil {
			if *vp.Status.ConfigCheckResult {
				vpCombined = append(vpCombined, &vectorpipeline.Controller{
					VectorPipeline: vp.DeepCopy(),
				})
			}
		}

	}
	return vpCombined, nil
}

func (ctrl *Controller) SelectClusterVectorPipelineSucceesed() ([]*clustervectorpipeline.Controller, error) {
	var cvpCombined []*clustervectorpipeline.Controller

	cvps := vectorv1alpha1.ClusterVectorPipelineList{}
	err := ctrl.Client.List(ctrl.Ctx, &cvps)
	if err != nil {
		return nil, err
	}

	for _, cvp := range cvps.Items {
		if !cvp.DeletionTimestamp.IsZero() {
			continue
		}
		if cvp.Status.ConfigCheckResult != nil {
			if *cvp.Status.ConfigCheckResult {
				cvpCombined = append(cvpCombined, &clustervectorpipeline.Controller{
					ClusterVectorPipeline: cvp.DeepCopy(),
				})
			}
		}

	}
	return cvpCombined, nil
}

func (ctrl *Controller) GetSources(filter []string) ([]vector.Source, error) {
	var sources []vector.Source
	sourcesMap, err := decodeRaw(ctrl.Pipeline.Spec().Sources.Raw)
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
		source.Name = addPrefix(ctrl.Pipeline.Namespace(), ctrl.Pipeline.Name(), k)
		sources = append(sources, source)
	}
	return sources, nil
}

func (ctrl *Controller) GetTransforms() ([]vector.Transform, error) {
	if ctrl.Pipeline.Spec().Transforms == nil {
		return nil, nil
	}
	transformsMap, err := decodeRaw(ctrl.Pipeline.Spec().Transforms.Raw)
	if err != nil {
		return nil, err
	}
	var transforms []vector.Transform
	if err := json.Unmarshal(ctrl.Pipeline.Spec().Transforms.Raw, &transformsMap); err != nil {
		return nil, err
	}
	for k, v := range transformsMap {
		var transform vector.Transform
		if err := mapstructure.Decode(v, &transform); err != nil {
			return nil, err
		}
		transform.Name = addPrefix(ctrl.Pipeline.Namespace(), ctrl.Pipeline.Name(), k)
		for i, inputName := range transform.Inputs {
			transform.Inputs[i] = addPrefix(ctrl.Pipeline.Namespace(), ctrl.Pipeline.Name(), inputName)
		}
		transforms = append(transforms, transform)
	}
	return transforms, nil
}

func (ctrl *Controller) GetSinks() ([]vector.Sink, error) {
	sinksMap, err := decodeRaw(ctrl.Pipeline.Spec().Sinks.Raw)
	if err != nil {
		return nil, err
	}
	var sinks []vector.Sink
	for k, v := range sinksMap {
		var sink vector.Sink
		if err := mapstructure.Decode(v, &sink); err != nil {
			return nil, err
		}
		sink.Name = addPrefix(ctrl.Pipeline.Namespace(), ctrl.Pipeline.Name(), k)
		for i, inputName := range sink.Inputs {
			sink.Inputs[i] = addPrefix(ctrl.Pipeline.Namespace(), ctrl.Pipeline.Name(), inputName)
		}
		sinks = append(sinks, sink)
	}
	return sinks, nil
}

func (ctrl *Controller) SetSucceesStatus() error {
	var status = true

	ctrl.Pipeline.SetConfigCheck(status)
	ctrl.Pipeline.SetReason(nil)

	return ctrl.Pipeline.UpdateStatus(ctrl.Ctx, ctrl.Client)
}

func (ctrl *Controller) SetFailedStatus(err error) error {
	var status = false
	var reason = err.Error()

	ctrl.Pipeline.SetConfigCheck(status)
	ctrl.Pipeline.SetReason(&reason)

	return ctrl.Pipeline.UpdateStatus(ctrl.Ctx, ctrl.Client)
}

func (ctrl *Controller) SetLastAppliedPipelineStatus() error {
	hash, err := ctrl.GetSpecHash()
	if err != nil {
		return err
	}
	ctrl.Pipeline.SetLastAppliedPipeline(hash)

	return ctrl.Pipeline.UpdateStatus(ctrl.Ctx, ctrl.Client)
}

func decodeRaw(raw []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

func addPrefix(Namespace, Name, componentName string) string {
	return generateName(Namespace, Name) + "-" + componentName
}

func generateName(Namespace, Name string) string {
	return Namespace + "-" + Name
}
