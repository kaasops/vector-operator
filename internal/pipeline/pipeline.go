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
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Pipeline interface {
	client.Object
	GetSpec() v1alpha1.VectorPipelineSpec
	SetConfigCheck(bool)
	SetReason(*string)
	SkipPrefix() bool
	GetLastAppliedPipeline() *uint32
	SetLastAppliedPipeline(*uint32)
	GetConfigCheckResult() *bool
	IsValid() bool
	IsDeleted() bool
	UpdateStatus(context.Context, client.Client) error
	GetRole() v1alpha1.VectorPipelineRole
	SetRole(*v1alpha1.VectorPipelineRole)
	GetTypeMeta() v1.TypeMeta
}

type FilterPipelines struct {
	Scope     FilterScope
	Selector  *v1alpha1.VectorSelectorSpec
	Role      v1alpha1.VectorPipelineRole
	Namespace string
}

type FilterScope int

const (
	AllPipelines       FilterScope = iota
	NamespacedPipeline FilterScope = iota
	ClusterPipelines   FilterScope = iota
)

func GetValidPipelines(ctx context.Context, client client.Client, filter FilterPipelines) ([]Pipeline, error) {
	var validPipelines []Pipeline

	matchLabels := map[string]string{}
	if filter.Selector != nil && filter.Selector.MatchLabels != nil {
		matchLabels = filter.Selector.MatchLabels
	}

	if filter.Scope == AllPipelines || filter.Scope == NamespacedPipeline {

		if filter.Scope == NamespacedPipeline && filter.Namespace == "" {
			return nil, fmt.Errorf("namespace not specified")
		}

		vps, err := GetVectorPipelines(ctx, client)
		if err != nil {
			return nil, err
		}
		if len(vps) != 0 {
			for _, vp := range vps {
				if !vp.IsDeleted() &&
					vp.IsValid() &&
					(vp.GetRole() == filter.Role || vp.GetRole() == v1alpha1.VectorPipelineRoleMixed) &&
					(filter.Scope == AllPipelines || vp.Namespace == filter.Namespace) &&
					MatchLabels(matchLabels, vp.Labels) {
					validPipelines = append(validPipelines, vp.DeepCopy())
				}
			}
		}
	}

	if filter.Scope == AllPipelines || filter.Scope == ClusterPipelines {
		cvps, err := GetClusterVectorPipelines(ctx, client)
		if err != nil {
			return nil, err
		}
		if len(cvps) != 0 {
			for _, cvp := range cvps {
				if !cvp.IsDeleted() &&
					cvp.IsValid() &&
					(cvp.GetRole() == filter.Role || cvp.GetRole() == v1alpha1.VectorPipelineRoleMixed) &&
					MatchLabels(matchLabels, cvp.Labels) {
					validPipelines = append(validPipelines, cvp.DeepCopy())
				}
			}
		}
	}
	return validPipelines, nil
}

func SetSuccessStatus(ctx context.Context, client client.Client, p Pipeline) error {
	p.SetConfigCheck(true)
	p.SetReason(nil)
	hash, err := GetPipelineHash(p)
	if err != nil {
		return err
	}
	p.SetLastAppliedPipeline(hash)

	return p.UpdateStatus(ctx, client)
}

func SetFailedStatus(ctx context.Context, client client.Client, p Pipeline, reason string) error {

	p.SetConfigCheck(false)
	p.SetReason(&reason)
	hash, err := GetPipelineHash(p)
	if err != nil {
		return err
	}
	p.SetLastAppliedPipeline(hash)

	return p.UpdateStatus(ctx, client)
}

func GetVectorPipelines(ctx context.Context, client client.Client) ([]v1alpha1.VectorPipeline, error) {
	vps := v1alpha1.VectorPipelineList{}
	if err := client.List(ctx, &vps); err != nil {
		return nil, err
	}
	return vps.Items, nil
}

func GetClusterVectorPipelines(ctx context.Context, client client.Client) ([]v1alpha1.ClusterVectorPipeline, error) {
	cvps := v1alpha1.ClusterVectorPipelineList{}
	if err := client.List(ctx, &cvps); err != nil {
		return nil, err
	}
	return cvps.Items, nil
}

func MatchLabels(selector map[string]string, labels map[string]string) bool {
	if selector == nil {
		return true
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
