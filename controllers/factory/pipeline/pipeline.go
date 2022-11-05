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

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Pipeline interface {
	GetSpec() vectorv1alpha1.VectorPipelineSpec
	GetName() string
	GetNamespace() string
	Type() string
	SetConfigCheck(bool)
	SetReason(*string)
	GetLastAppliedPipeline() *uint32
	SetLastAppliedPipeline(*uint32)
	IsValid() bool
	IsDeleted() bool
	UpdateStatus(context.Context, client.Client) error
}

func GetValidPipelines(ctx context.Context, client client.Client) ([]Pipeline, error) {
	var validPipelines []Pipeline
	vps, err := GetVectorPipelines(ctx, client)
	if err != nil {
		return nil, err
	}
	cvps, err := GetClusterVectorPipelines(ctx, client)
	if err != nil {
		return nil, err
	}
	if len(vps) != 0 {
		for _, vp := range vps {
			if !vp.IsDeleted() && vp.IsValid() {
				validPipelines = append(validPipelines, vp.DeepCopy())
			}
		}
	}
	if len(cvps) != 0 {
		for _, cvp := range cvps {
			if !cvp.IsDeleted() && cvp.IsValid() {
				validPipelines = append(validPipelines, cvp.DeepCopy())
			}
		}
	}
	return validPipelines, nil
}

func SetSuccessStatus(ctx context.Context, client client.Client, p Pipeline) error {
	var status = true

	p.SetConfigCheck(status)
	p.SetReason(nil)

	return p.UpdateStatus(ctx, client)
}

func SetFailedStatus(ctx context.Context, client client.Client, p Pipeline, err error) error {
	var status = false
	var reason = err.Error()

	p.SetConfigCheck(status)
	p.SetReason(&reason)

	return p.UpdateStatus(ctx, client)
}

func SetLastAppliedPipelineStatus(ctx context.Context, client client.Client, p Pipeline) error {
	hash, err := GetSpecHash(p)
	if err != nil {
		return err
	}
	p.SetLastAppliedPipeline(hash)

	return p.UpdateStatus(ctx, client)
}

func GetVectorPipelines(ctx context.Context, client client.Client) ([]vectorv1alpha1.VectorPipeline, error) {
	vps := vectorv1alpha1.VectorPipelineList{}
	if err := client.List(ctx, &vps); err != nil {
		return nil, err
	}
	return vps.Items, nil
}

func GetClusterVectorPipelines(ctx context.Context, client client.Client) ([]vectorv1alpha1.ClusterVectorPipeline, error) {
	cvps := vectorv1alpha1.ClusterVectorPipelineList{}
	if err := client.List(ctx, &cvps); err != nil {
		return nil, err
	}
	return cvps.Items, nil
}
