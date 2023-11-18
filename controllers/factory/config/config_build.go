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

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/utils/hash"
	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	"github.com/mitchellh/mapstructure"
)

const (
	KubernetesSourceType       = "kubernetes_logs"
	BlackholeSinkType          = "blackhole"
	InternalMetricsSourceType  = "internal_metrics"
	InternalMetricsSourceName  = "internalMetricsSource"
	InternalMetricsSinkType    = "prometheus_exporter"
	InternalMetricsSinkName    = "internalMetricsSink"
	MergedKubernetesSourceName = "mergedKubernetesSource"
	MergedSourceTransformName  = "merged"
	RouteTransformType         = "route"
	DefaultSourceName          = "defaultSource"
	PodSelectorType            = "pod_labels"
	NamespaceSelectorType      = "ns_labels"
	OptimizationConditionType  = "vrl"
)

var (
	sourceDefault = &Source{
		Name: "defaultSource",
		Type: KubernetesSourceType,
	}
	internalMetricSource = &Source{
		Name: InternalMetricsSourceName,
		Type: InternalMetricsSourceType,
	}
	sinkDefault = &Sink{
		Name:   "defaultSink",
		Type:   BlackholeSinkType,
		Inputs: []string{sourceDefault.Name},
		Options: map[string]interface{}{
			"rate":                100,
			"print_interval_secs": 60,
		},
	}
	internalMetricsExporter = &Sink{
		Name:   InternalMetricsSinkName,
		Type:   InternalMetricsSinkType,
		Inputs: []string{internalMetricSource.Name},
	}
)

var (
	PipelineTypeError  error = errors.New("type kubernetes_logs only allowed")
	PipelineScopeError error = errors.New("logs from external namespace not allowed")
)

type Builder struct {
	Name string
	// vaCtrl    *vectoragent.Controller
	Vector    *v1alpha1.Vector
	Pipelines []pipeline.Pipeline
}

func NewBuilder(vector *v1alpha1.Vector, pipelines ...pipeline.Pipeline) *Builder {
	return &Builder{
		// vaCtrl:    vaCtrl,
		Vector:    vector,
		Pipelines: pipelines,
	}
}

func (b *Builder) GetByteConfig(pipelines ...pipeline.Pipeline) ([]byte, error) {
	config, err := b.generateVectorConfig(pipelines)
	if err != nil {
		return nil, err
	}
	data, err := vectorConfigToByte(config)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (b *Builder) GetByteConfigs() ([][]byte, error) {
	data := [][]byte{}

	var vectorCount = 1
	if b.Vector.Spec.Agent.InstanceCount != nil {
		vectorCount = *b.Vector.Spec.Agent.InstanceCount
	}

	slicePipelines := b.slicePipelines(vectorCount)

	for _, sp := range slicePipelines {
		byteConfig, err := b.GetByteConfig(sp...)
		if err != nil {
			return nil, err
		}

		data = append(data, byteConfig)
	}

	return data, nil
}

func (b *Builder) generateVectorConfig(pipelines []pipeline.Pipeline) (*VectorConfig, error) {
	vectorConfig := New(b.Vector)

	sources, transforms, sinks, err := b.getComponents(pipelines)
	if err != nil {
		return nil, err
	}

	if b.Vector.Spec.Agent.InternalMetrics && !isExporterSinkExists(sinks) {
		sources = append(sources, internalMetricSource)
		sinks = append(sinks, internalMetricsExporter)
	}

	if len(sources) == 0 {
		sources = []*Source{sourceDefault}
	}
	if len(sinks) == 0 {
		sinks = []*Sink{sinkDefault}
	}

	vectorConfig.Sinks = sinks
	vectorConfig.Sources = sources
	vectorConfig.Transforms = transforms

	if b.Vector.Spec.MergeKubernetesSources {
		if err := b.mergeKubernetesSources(vectorConfig); err != nil {
			return nil, err
		}
	}

	if b.Vector.Spec.MergeSinks {
		if err := b.mergeSyncs(vectorConfig); err != nil {
			return nil, err
		}
	}

	return vectorConfig, nil
}

func (b *Builder) getComponents(pipelines []pipeline.Pipeline) (sources []*Source, transforms []*Transform, sinks []*Sink, err error) {
	for _, pipeline := range pipelines {
		pipelineSources, err := getSources(pipeline, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, source := range pipelineSources {
			if err != nil {
				return nil, nil, nil, err
			}
			if source.Type == KubernetesSourceType {
				if pipeline.Type() != vectorv1alpha1.ClusterPipelineKind && source.ExtraNamespaceLabelSelector == "" {
					source.ExtraNamespaceLabelSelector = k8s.NamespaceNameToLabel(pipeline.GetNamespace())
				}
			}
			if pipeline.Type() != vectorv1alpha1.ClusterPipelineKind {
				if source.Type != KubernetesSourceType {
					return nil, nil, nil, PipelineTypeError
				}
				if source.Type == InternalMetricsSourceType {
					return nil, nil, nil, PipelineTypeError
				}
				if source.ExtraNamespaceLabelSelector != "" {
					if source.ExtraNamespaceLabelSelector != k8s.NamespaceNameToLabel(pipeline.GetNamespace()) {
						return nil, nil, nil, PipelineScopeError
					}
				}
			}
			sources = append(sources, source)
		}
		pipelineTransforms, err := getTransforms(pipeline)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, transform := range pipelineTransforms {
			if err != nil {
				return nil, nil, nil, err
			}
			transforms = append(transforms, transform)
		}
		pipelineSinks, err := getSinks(pipeline)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, sink := range pipelineSinks {
			if err != nil {
				return nil, nil, nil, err
			}
			sinks = append(sinks, sink)
		}
	}
	return sources, transforms, sinks, nil
}

func vectorConfigToByte(config *VectorConfig) ([]byte, error) {
	cfgMap, err := cfgToMap(config)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(cfgMap)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func getSources(pipeline pipeline.Pipeline, filter []string) ([]*Source, error) {
	if pipeline.GetSpec().Sources == nil {
		return nil, nil
	}
	var sources []*Source
	sourcesMap, err := decodeRaw(pipeline.GetSpec().Sources.Raw)
	if err != nil {
		return nil, err
	}
	for k, v := range sourcesMap {
		if len(filter) != 0 {
			if !contains(filter, k) {
				continue
			}
		}
		var source *Source
		if err := mapstructure.Decode(v, &source); err != nil {
			return nil, err
		}
		source.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
		sources = append(sources, source)
	}
	return sources, nil
}

func getTransforms(pipeline pipeline.Pipeline) ([]*Transform, error) {
	if pipeline.GetSpec().Transforms == nil {
		return nil, nil
	}
	transformsMap, err := decodeRaw(pipeline.GetSpec().Transforms.Raw)
	if err != nil {
		return nil, err
	}
	var transforms []*Transform
	if err := json.Unmarshal(pipeline.GetSpec().Transforms.Raw, &transformsMap); err != nil {
		return nil, err
	}
	for k, v := range transformsMap {
		var transform *Transform
		if err := mapstructure.Decode(v, &transform); err != nil {
			return nil, err
		}
		transform.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
		for i, inputName := range transform.Inputs {
			transform.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
		}
		transforms = append(transforms, transform)
	}
	return transforms, nil
}

func getSinks(pipeline pipeline.Pipeline) ([]*Sink, error) {
	if pipeline.GetSpec().Sinks == nil {
		return nil, nil
	}
	sinksMap, err := decodeRaw(pipeline.GetSpec().Sinks.Raw)
	if err != nil {
		return nil, err
	}
	var sinks []*Sink
	for k, v := range sinksMap {
		var sink *Sink
		if err := mapstructure.Decode(v, &sink); err != nil {
			return nil, err
		}
		sink.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k)
		for i, inputName := range sink.Inputs {
			sink.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName)
		}
		optbyte, err := json.Marshal(sink.Options)
		if err != nil {
			return nil, err
		}
		sink.OptionsHash = fmt.Sprint(hash.Get(optbyte))
		sinks = append(sinks, sink)
	}
	return sinks, nil
}

func cfgToMap(config *VectorConfig) (cfgMap map[string]interface{}, err error) {
	sources := make(map[string]interface{})
	transforms := make(map[string]interface{})
	sinks := make(map[string]interface{})
	for _, source := range config.Sources {
		spec, err := Mapper(source)
		if err != nil {
			return nil, err
		}
		sources[source.Name] = spec
	}
	for _, transform := range config.Transforms {
		spec, err := Mapper(transform)
		if err != nil {
			return nil, err
		}
		transforms[transform.Name] = spec
	}
	for _, sink := range config.Sinks {
		spec, err := Mapper(sink)
		if err != nil {
			return nil, err
		}
		sinks[sink.Name] = spec
	}

	err = mapstructure.Decode(config, &cfgMap)
	if err != nil {
		return nil, err
	}
	// TODO: remove hardcoded map keys
	cfgMap["sources"] = sources
	cfgMap["transforms"] = transforms
	cfgMap["sinks"] = sinks

	return cfgMap, nil
}

func isExporterSinkExists(sinks []*Sink) bool {
	for _, sink := range sinks {
		if sink.Type == InternalMetricsSinkType {
			return true
		}
	}
	return false
}

// Merges a large number of kubernetes sources to reduce k8s api pressure during vector start.
func (b *Builder) mergeKubernetesSources(config *VectorConfig) error {
	var mergedSource []*Source
	routes := make(map[string]string)
	for _, source := range config.Sources {
		if source.Type == KubernetesSourceType {
			if source.ExtraFieldSelector == "" && source.ExtraNamespaceLabelSelector != "" && source.ExtraLabelSelector != "" && source.Options == nil {
				routes[source.Name] = generateVrlFilter(source.ExtraLabelSelector, PodSelectorType) + "&&" + generateVrlFilter(source.ExtraNamespaceLabelSelector, NamespaceSelectorType)
				continue
			}
		}
		mergedSource = append(mergedSource, source)
	}

	if len(routes) > 0 {
		mergedSource = append(mergedSource, &Source{
			Name: MergedKubernetesSourceName,
			Type: KubernetesSourceType,
		})
		transform := &Transform{
			Name:   MergedSourceTransformName,
			Type:   RouteTransformType,
			Inputs: []string{MergedKubernetesSourceName},
			Route:  routes,
		}

		for _, t := range config.Transforms {
			for n, i := range t.Inputs {
				_, ok := routes[i]
				if ok {
					t.Inputs[n] = MergedSourceTransformName + "." + i

				}
			}
		}

		for _, t := range config.Sinks {
			for n, i := range t.Inputs {
				_, ok := routes[i]
				if ok {
					t.Inputs[n] = MergedSourceTransformName + "." + i

				}
			}
		}
		config.Sources = mergedSource
		config.Transforms = append(config.Transforms, transform)
	}

	return nil
}

func (b *Builder) mergeSyncs(config *VectorConfig) error {
	uniqOpts := make(map[string]*Sink)
	var mergedSinks []*Sink

	for _, sink := range config.Sinks {
		v, ok := uniqOpts[sink.OptionsHash]
		if ok {
			if sink.Type == v.Type {
				// If sink spec already exists rename and merge inputs
				v.Name = v.OptionsHash
				v.Inputs = append(v.Inputs, sink.Inputs...)
				sort.Strings(v.Inputs)
				continue
			}
		}
		uniqOpts[sink.OptionsHash] = sink
		mergedSinks = append(mergedSinks, sink)
	}

	if len(mergedSinks) > 0 {
		config.Sinks = mergedSinks
	}
	return nil
}

func (b *Builder) slicePipelines(count int) [][]pipeline.Pipeline {
	slicePipelines := [][]pipeline.Pipeline{}

	if count == 1 {
		slicePipelines = append(slicePipelines, b.Pipelines)
		return slicePipelines
	}

	for i := 1; i <= count; i++ {
		if i == 1 {
			lastElem := len(b.Pipelines) / count
			slicePipelines = append(slicePipelines, b.Pipelines[0:lastElem])
			continue
		}

		if i == count {
			firstElem := (len(b.Pipelines) / count) * (i - 1)
			slicePipelines = append(slicePipelines, b.Pipelines[firstElem:])
			continue
		}

		firstElem := (len(b.Pipelines) / count) * (i - 1)
		lastElem := (len(b.Pipelines) / count) * (i)
		slicePipelines = append(slicePipelines, b.Pipelines[firstElem:lastElem])
	}

	return slicePipelines
}

func generateVrlFilter(selector string, selectorType string) string {
	buffer := new(bytes.Buffer)

	labels := strings.Split(selector, ",")

	for _, label := range labels {
		label = formatVrlSelector(label)
		switch selectorType {
		case PodSelectorType:
			fmt.Fprintf(buffer, ".kubernetes.pod_labels.%s&&", label)
		case NamespaceSelectorType:
			fmt.Fprintf(buffer, ".kubernetes.namespace_labels.%s&&", label)
		}
	}

	vrlstring := buffer.String()
	return strings.TrimSuffix(vrlstring, "&&")
}

func formatVrlSelector(label string) string {
	l := strings.Split(label, "!=")
	if len(l) == 2 {
		return fmt.Sprintf("\"%s\" != \"%s\"", l[0], l[1])
	}

	l = strings.Split(label, "=")
	if len(l) != 2 {
		return label
	}
	return fmt.Sprintf("\"%s\" == \"%s\"", l[0], l[1])
}
