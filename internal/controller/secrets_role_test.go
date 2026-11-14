/*
Copyright 2024.

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

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

func roleSources(sources string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(sources)}
}

func roleSinks() *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)}
}

// A validated pipeline whose spec was then edited: the status still carries the role of
// the validated spec.
func editedVP(statusRole v1alpha1.VectorPipelineRole, pin *v1alpha1.VectorPipelineRole, sources string) *v1alpha1.VectorPipeline {
	return &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "team"},
		Spec:       v1alpha1.VectorPipelineSpec{Role: pin, Sources: roleSources(sources), Sinks: roleSinks()},
		Status:     v1alpha1.VectorPipelineStatus{ConfigCheckResult: boolPtr(true), Role: rolePtr(statusRole)},
	}
}

func editedCVP(statusRole v1alpha1.VectorPipelineRole, pin *v1alpha1.VectorPipelineRole, sources string) *v1alpha1.ClusterVectorPipeline {
	return &v1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec:       v1alpha1.VectorPipelineSpec{Role: pin, Sources: roleSources(sources), Sinks: roleSinks()},
		Status:     v1alpha1.VectorPipelineStatus{ConfigCheckResult: boolPtr(true), Role: rolePtr(statusRole)},
	}
}

const (
	logsSource    = `{"in": {"type": "kubernetes_logs"}}`
	httpSource    = `{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`
	metricsSource = `{"in": {"type": "internal_metrics"}}` // runs in both roles, agent unless pinned
	fileSource    = `{"in": {"type": "file", "include": ["/var/log/*.log"]}}`
)

func TestResolveWorkloadPipelinesSkipsUnvalidatedRoleChange(t *testing.T) {
	agent, aggregator := v1alpha1.VectorPipelineRoleAgent, v1alpha1.VectorPipelineRoleAggregator
	cases := []struct {
		name       string
		statusRole v1alpha1.VectorPipelineRole
		pin        *v1alpha1.VectorPipelineRole
		sources    string
		// role of the workload that must still get the pipeline, "" for none
		keptBy v1alpha1.VectorPipelineRole
	}{
		{"agent to aggregator by source type", agent, nil, httpSource, ""},
		{"aggregator to agent by source type", aggregator, nil, logsSource, ""},
		{"pin added", agent, rolePtr(aggregator), metricsSource, ""},
		{"pin removed", aggregator, nil, metricsSource, ""},
		{"pin changed", aggregator, rolePtr(agent), metricsSource, ""},
		{"pin made explicit", aggregator, rolePtr(aggregator), metricsSource, aggregator},
		{"agent edit keeping the role", agent, nil, fileSource, agent},
		{"aggregator edit keeping the role", aggregator, nil, httpSource, aggregator},
		{"spec that yields no role", agent, nil, `{"in": {"type": "no_such_source"}}`, ""},
		{"spec that does not parse", agent, nil, `{"in": null}`, ""},
	}
	for _, tc := range cases {
		for _, kind := range []string{"VectorPipeline", "ClusterVectorPipeline"} {
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				var p client.Object = editedVP(tc.statusRole, tc.pin, tc.sources)
				if kind == "ClusterVectorPipeline" {
					p = editedCVP(tc.statusRole, tc.pin, tc.sources)
				}
				before := p.DeepCopyObject().(client.Object)
				c := newFakeClient(p)
				getter := staticGetter(nil)

				for _, role := range []v1alpha1.VectorPipelineRole{agent, aggregator} {
					filter := pipeline.FilterPipelines{Scope: pipeline.AllPipelines, Role: role}
					result, _, err := resolveWorkloadPipelines(context.Background(), c, getter, filter, "Vector", "default", "w", testAssetsPrototype())
					require.NoError(t, err)
					if role == tc.keptBy {
						require.Len(t, result, 1, "the %s workload must keep the pipeline", role)
					} else {
						assert.Empty(t, result, "the %s workload must not get the pipeline", role)
					}
				}

				got := p.DeepCopyObject().(client.Object)
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(p), got))
				assert.Equal(t, before.(pipeline.Pipeline).GetConfigCheckResult(), got.(pipeline.Pipeline).GetConfigCheckResult())
				assert.Equal(t, before.(pipeline.Pipeline).GetReason(), got.(pipeline.Pipeline).GetReason())
				assert.Equal(t, before.(pipeline.Pipeline).GetRole(), got.(pipeline.Pipeline).GetRole())
			})
		}
	}
}

// A skipped pipeline reads no Secret and takes no part in collision attribution: the
// younger neighbour it would collide with stays in the build. This holds for a pipeline
// held back by a secret attribution failure as well, which still enters the pool.
func TestResolveWorkloadPipelinesSkippedRoleChangeDoesNotAffectNeighbours(t *testing.T) {
	for name, reason := range map[string]*string{
		"valid":            nil,
		"collision":        ptr.To(secretCollisionReasonPrefix + "earlier"),
		"size":             ptr.To(secretSizeExclusionReasonPrefix + "earlier"),
		"object size":      ptr.To(secretObjectSizeExclusionReasonPrefix + "earlier"),
		"waiting for keys": ptr.To(secretAssetsWaitingReasonPrefix + "earlier"),
	} {
		t.Run(name, func(t *testing.T) {
			t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			older, younger := collidingVPs(t0)
			for _, vp := range []*v1alpha1.VectorPipeline{older, younger} {
				vp.Status.ConfigCheckResult = boolPtr(true)
				vp.Status.Role = rolePtr(v1alpha1.VectorPipelineRoleAgent)
			}
			if reason != nil {
				older.Status.ConfigCheckResult = boolPtr(false)
				older.Status.Reason = reason
			}
			// The older pipeline was edited into an aggregator one and not validated yet.
			older.Spec.Sources = roleSources(`{"logs": {"type": "http_server", "address": "0.0.0.0:8080"}}`)

			c := newFakeClient(older, younger)
			var reads []string
			getter := func(_ context.Context, namespace, name string) (*corev1.Secret, error) {
				reads = append(reads, namespace+"/"+name)
				return &corev1.Secret{Data: map[string][]byte{"username": []byte("u")}}, nil
			}

			result, reinstate, err := resolveWorkloadPipelines(context.Background(), c, getter, agentFilter(), "Vector", "default", "v", testAssetsPrototype())
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, "x", result[0].GetName())
			assert.Empty(t, reinstate)
			assert.NotContains(t, reads, "team/creds")

			gotYounger := &v1alpha1.VectorPipeline{}
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(younger), gotYounger))
			assert.True(t, gotYounger.IsValid())
			gotOlder := &v1alpha1.VectorPipeline{}
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(older), gotOlder))
			assert.Equal(t, older.Status.ConfigCheckResult, gotOlder.Status.ConfigCheckResult)
			assert.Equal(t, older.Status.Reason, gotOlder.Status.Reason)
		})
	}
}
