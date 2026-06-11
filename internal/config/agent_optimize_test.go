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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

func testPipeline(namespace, name string, sources, sinks string) pipeline.Pipeline {
	return &vectorv1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: vectorv1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(sources)},
			Sinks:   &runtime.RawExtension{Raw: []byte(sinks)},
		},
	}
}

func testLogPipeline(namespace string) pipeline.Pipeline {
	return testPipeline(namespace, "pipe",
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "blackhole", "inputs": ["logs"]}}`)
}

func TestOptimizeSourcesCollapsesIdenticalSources(t *testing.T) {
	cfg, _, err := BuildAgentConfig(VectorConfigParams{OptimizeSources: true, UseApiServerCache: true},
		testLogPipeline("ns-a"), testLogPipeline("ns-b"), testLogPipeline("ns-c"))
	require.NoError(t, err)

	require.Len(t, cfg.Sources, 1)
	var collapsed *Source
	for _, s := range cfg.Sources {
		collapsed = s
	}
	assert.Equal(t, KubernetesLogsType, collapsed.Type)
	assert.Equal(t, "kubernetes.io/metadata.name in (ns-a,ns-b,ns-c)", collapsed.ExtraNamespaceLabelSelector)
	assert.True(t, collapsed.UseApiServerCache)

	// flat routing: single route transform with a route per original source
	require.Len(t, cfg.Transforms, 1)
	var router *Transform
	for _, tr := range cfg.Transforms {
		router = tr
	}
	assert.Equal(t, RouteTransformType, router.Type)
	assert.Equal(t, []string{collapsed.Name}, router.Inputs)
	routes := router.Options["route"].(map[string]string)
	assert.Equal(t, `.kubernetes.pod_namespace == "ns-a"`, routes["ns-a"])
	assert.Len(t, routes, 3)

	// sink inputs are rewritten to the route outputs
	assert.Equal(t, []string{router.Name + ".ns-a"}, cfg.Sinks["ns-a-pipe-out"].Inputs)
	assert.Equal(t, []string{router.Name + ".ns-b"}, cfg.Sinks["ns-b-pipe-out"].Inputs)
}

func TestOptimizeSourcesDisabledKeepsSources(t *testing.T) {
	cfg, _, err := BuildAgentConfig(VectorConfigParams{},
		testLogPipeline("ns-a"), testLogPipeline("ns-b"))
	require.NoError(t, err)
	assert.Len(t, cfg.Sources, 2)
	assert.Len(t, cfg.Transforms, 0)
	assert.Equal(t, []string{"ns-a-pipe-logs"}, cfg.Sinks["ns-a-pipe-out"].Inputs)
}

func TestOptimizeSourcesGroupsBySignature(t *testing.T) {
	withSelector := testPipeline("ns-c", "pipe",
		`{"logs": {"type": "kubernetes_logs", "extra_label_selector": "app=foo"}}`,
		`{"out": {"type": "blackhole", "inputs": ["logs"]}}`)
	cfg, _, err := BuildAgentConfig(VectorConfigParams{OptimizeSources: true},
		testLogPipeline("ns-a"), testLogPipeline("ns-b"), withSelector)
	require.NoError(t, err)

	// ns-a + ns-b collapse; the source with a label selector forms a group of one and stays as is
	require.Len(t, cfg.Sources, 2)
	standalone := cfg.Sources["ns-c-pipe-logs"]
	require.NotNil(t, standalone)
	assert.Equal(t, "app=foo", standalone.ExtraLabelSelector)
	assert.Equal(t, "kubernetes.io/metadata.name=ns-c", standalone.ExtraNamespaceLabelSelector)
	assert.Equal(t, []string{"ns-c-pipe-logs"}, cfg.Sinks["ns-c-pipe-out"].Inputs)
}

func TestOptimizeSourcesHierarchicalRouting(t *testing.T) {
	pipelines := make([]pipeline.Pipeline, 0, 40)
	for i := 0; i < 40; i++ {
		pipelines = append(pipelines, testLogPipeline(fmt.Sprintf("ns-%02d", i)))
	}
	cfg, _, err := BuildAgentConfig(VectorConfigParams{OptimizeSources: true}, pipelines...)
	require.NoError(t, err)

	require.Len(t, cfg.Sources, 1)
	var bucketer *Transform
	routers := 0
	for _, tr := range cfg.Transforms {
		switch tr.Type {
		case RemapTransformType:
			bucketer = tr
		case RouteTransformType:
			routers++
		}
	}
	// bucketCount(40) = 8: remap + l1 + at most 8 l2 routers
	require.NotNil(t, bucketer)
	assert.Contains(t, bucketer.Options["source"], "% 8")
	assert.GreaterOrEqual(t, routers, 2)

	// every sink input points to an existing transform output
	outputs := make(map[string]bool)
	for name, tr := range cfg.Transforms {
		if tr.Type != RouteTransformType {
			continue
		}
		for key := range tr.Options["route"].(map[string]string) {
			outputs[name+"."+key] = true
		}
	}
	for name, sink := range cfg.Sinks {
		require.Len(t, sink.Inputs, 1, name)
		assert.True(t, outputs[sink.Inputs[0]], "sink %s input %s has no producer", name, sink.Inputs[0])
	}
}

func TestOptimizeSourcesDeterministicConfig(t *testing.T) {
	build := func() string {
		_, b, err := BuildAgentConfig(VectorConfigParams{OptimizeSources: true},
			testLogPipeline("ns-a"), testLogPipeline("ns-b"), testLogPipeline("ns-c"))
		require.NoError(t, err)
		return string(b)
	}
	first := build()
	for i := 0; i < 10; i++ {
		assert.Equal(t, first, build())
	}
}

func TestOptimizeSourcesSharedNamespaceRoute(t *testing.T) {
	second := testPipeline("ns-a", "second",
		`{"logs": {"type": "kubernetes_logs"}}`,
		`{"out": {"type": "blackhole", "inputs": ["logs"]}}`)
	cfg, _, err := BuildAgentConfig(VectorConfigParams{OptimizeSources: true},
		testLogPipeline("ns-a"), second, testLogPipeline("ns-b"))
	require.NoError(t, err)

	// two pipelines of ns-a share one route: conditions per namespace, not per source
	var router *Transform
	for _, tr := range cfg.Transforms {
		router = tr
	}
	require.NotNil(t, router)
	assert.Len(t, router.Options["route"].(map[string]string), 2)
	assert.Equal(t, []string{router.Name + ".ns-a"}, cfg.Sinks["ns-a-pipe-out"].Inputs)
	assert.Equal(t, []string{router.Name + ".ns-a"}, cfg.Sinks["ns-a-second-out"].Inputs)
	collapsed, groups := cfg.OptimizationSummary()
	assert.Equal(t, 3, collapsed)
	assert.Equal(t, 1, groups)
}

func TestOptimizeSourcesChunksLargeGroups(t *testing.T) {
	cfg := newVectorConfig(VectorConfigParams{})
	for i := 0; i < 2200; i++ {
		name := fmt.Sprintf("ns-%04d-pipe-logs", i)
		cfg.Sources[name] = &Source{
			Name:                        name,
			Type:                        KubernetesLogsType,
			ExtraNamespaceLabelSelector: fmt.Sprintf("kubernetes.io/metadata.name=ns-%04d", i),
		}
	}
	optimizeAgentSources(cfg)

	// 2200 namespaces are split into 3 sources of at most maxNamespacesPerSource
	require.Len(t, cfg.Sources, 3)
	total := 0
	for _, src := range cfg.Sources {
		assert.Contains(t, src.ExtraNamespaceLabelSelector, "kubernetes.io/metadata.name in (")
		total += strings.Count(src.ExtraNamespaceLabelSelector, "ns-")
	}
	assert.Equal(t, 2200, total)
	collapsed, groups := cfg.OptimizationSummary()
	assert.Equal(t, 2200, collapsed)
	assert.Equal(t, 3, groups)
}

func TestBucketCount(t *testing.T) {
	assert.Equal(t, 1, bucketCount(1))
	assert.Equal(t, 4, bucketCount(17))
	assert.Equal(t, 8, bucketCount(40))
	assert.Equal(t, 32, bucketCount(1000))
	assert.Equal(t, 256, bucketCount(1000000))
}
