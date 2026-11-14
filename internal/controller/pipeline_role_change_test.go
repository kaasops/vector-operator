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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// A workload reconcile can run between a role-changing edit and the pipeline's own
// validation of it, for example when a neighbour pipeline changes. Until then neither
// workload may build the edited spec. The pipelines are seeded valid directly, since the
// pipeline's own config check needs a kubelet.
var _ = Describe("Workloads during an unvalidated role change", func() {
	const (
		ns          = "role-change"
		logsSource  = `{"srcMoved": {"type": "kubernetes_logs"}}`
		httpSource  = `{"srcMoved": {"type": "http_server", "address": "0.0.0.0:8080"}}`
		neighbourVP = "neighbour"
	)
	selector := map[string]string{"role-change-test": "true"}
	disabled := v1alpha1.VectorCommon{ConfigCheck: v1alpha1.ConfigCheck{Disabled: true}}

	spec := func(sources, input string) v1alpha1.VectorPipelineSpec {
		return v1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(sources)},
			Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["` + input + `"]}}`)},
		}
	}
	validate := func(p pipeline.Pipeline, role v1alpha1.VectorPipelineRole) {
		p.SetRole(&role)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, p, pipelineStatusBase(p))).To(Succeed())
	}
	edit := func(p client.Object, sources string) {
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(p), p)).To(Succeed())
		switch o := p.(type) {
		case *v1alpha1.VectorPipeline:
			o.Spec.Sources = &runtime.RawExtension{Raw: []byte(sources)}
		case *v1alpha1.ClusterVectorPipeline:
			o.Spec.Sources = &runtime.RawExtension{Raw: []byte(sources)}
		}
		Expect(k8sClient.Update(ctx, p)).To(Succeed())
	}
	builtConfig := func(namespace, name, key string) string {
		s := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s)).To(Succeed())
		return string(s.Data[key])
	}

	var reconcileAll func()
	var agentConfig, vaConfig, cvaConfig func() string

	BeforeEach(func() {
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}))).To(Succeed())

		neighbour := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: neighbourVP, Namespace: ns, Labels: selector},
			Spec:       spec(`{"srcStays": {"type": "kubernetes_logs"}}`, "srcStays"),
		}
		Expect(k8sClient.Create(ctx, neighbour)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, neighbour) })
		validate(neighbour, v1alpha1.VectorPipelineRoleAgent)

		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: "role-change", Namespace: ns},
			Spec: v1alpha1.VectorSpec{
				Selector: &v1alpha1.VectorSelectorSpec{MatchLabels: selector},
				Agent:    &v1alpha1.VectorAgent{VectorCommon: disabled},
			},
		}
		va := &v1alpha1.VectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "role-change", Namespace: ns},
			Spec: v1alpha1.VectorAggregatorSpec{VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{
				Selector: &v1alpha1.VectorSelectorSpec{MatchLabels: selector}, VectorCommon: disabled,
			}},
		}
		cva := &v1alpha1.ClusterVectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "role-change-cluster"},
			Spec: v1alpha1.ClusterVectorAggregatorSpec{ResourceNamespace: ns, VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{
				Selector: &v1alpha1.VectorSelectorSpec{MatchLabels: selector}, VectorCommon: disabled,
			}},
		}
		for _, o := range []client.Object{vector, va, cva} {
			Expect(k8sClient.Create(ctx, o)).To(Succeed())
		}

		vr := &VectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Clientset: clientset, ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient: clientset.DiscoveryClient, EventChan: make(chan event.GenericEvent, 1), APIReader: k8sClient}
		var_ := &VectorAggregatorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Clientset: clientset, ConfigCheckTimeout: configCheckTimeout,
			EventChan: make(chan event.GenericEvent, 1), APIReader: k8sClient}
		cvar := &ClusterVectorAggregatorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Clientset: clientset, ConfigCheckTimeout: configCheckTimeout,
			EventChan: make(chan event.GenericEvent, 1), APIReader: k8sClient}
		reconcileAll = func() {
			for _, r := range []struct {
				r reconcile.Reconciler
				o client.Object
			}{{vr, vector}, {var_, va}, {cvar, cva}} {
				_, err := r.r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(r.o)})
				Expect(err).NotTo(HaveOccurred())
			}
		}
		DeferCleanup(func() {
			for _, o := range []client.Object{vector, va, cva} {
				_ = k8sClient.Delete(ctx, o)
			}
			// envtest runs no controller, so the finalizers are removed by reconciling the deleted objects.
			_, _ = var_.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(va)})
			_, _ = cvar.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cva)})
		})

		agentConfig = func() string { return builtConfig(ns, "role-change-agent", "agent.json") }
		vaConfig = func() string { return builtConfig(ns, "role-change-aggregator", "config.json") }
		cvaConfig = func() string { return builtConfig(ns, "role-change-cluster-aggregator", "config.json") }
	})

	It("moves a VectorPipeline from the agent to the aggregator only after validation", func() {
		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "moved", Namespace: ns, Labels: selector},
			Spec:       spec(logsSource, "srcMoved"),
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })
		validate(vp, v1alpha1.VectorPipelineRoleAgent)

		reconcileAll()
		reconcileAll() // the first aggregator reconcile only adds the finalizer
		Expect(agentConfig()).To(ContainSubstring(`"role-change-moved-srcMoved"`))
		Expect(vaConfig()).NotTo(ContainSubstring("srcMoved"))

		By("editing the spec into an aggregator pipeline, before its own reconcile")
		edit(vp, httpSource)
		reconcileAll()
		Expect(agentConfig()).NotTo(ContainSubstring("srcMoved"), "the agent must not build the edited spec")
		Expect(agentConfig()).To(ContainSubstring(`"role-change-neighbour-srcStays"`), "the neighbour stays")
		Expect(vaConfig()).NotTo(ContainSubstring("srcMoved"), "the aggregator must wait for validation")

		got := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
		Expect(got.Status.Role).To(HaveValue(Equal(v1alpha1.VectorPipelineRoleAgent)), "the status is not rewritten")
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeTrue()))

		By("validating the edit")
		validate(got, v1alpha1.VectorPipelineRoleAggregator)
		reconcileAll()
		Expect(vaConfig()).To(ContainSubstring(`"role-change-moved-srcMoved"`))
		Expect(agentConfig()).NotTo(ContainSubstring("srcMoved"))
	})

	It("moves a ClusterVectorPipeline from the aggregator to the agent only after validation", func() {
		cvp := &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "role-change-moved", Labels: selector},
			Spec:       spec(httpSource, "srcMoved"),
		}
		Expect(k8sClient.Create(ctx, cvp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cvp) })
		validate(cvp, v1alpha1.VectorPipelineRoleAggregator)

		reconcileAll()
		reconcileAll()
		Expect(cvaConfig()).To(ContainSubstring("srcMoved"))
		Expect(agentConfig()).NotTo(ContainSubstring("srcMoved"))

		By("editing the spec into an agent pipeline, before its own reconcile")
		edit(cvp, logsSource)
		reconcileAll()
		Expect(cvaConfig()).NotTo(ContainSubstring("srcMoved"), "the aggregator must not build the edited spec")
		Expect(agentConfig()).NotTo(ContainSubstring("srcMoved"), "the agent must wait for validation")
		Expect(agentConfig()).To(ContainSubstring(`"role-change-neighbour-srcStays"`), "the neighbour stays")

		By("validating the edit")
		got := &v1alpha1.ClusterVectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		validate(got, v1alpha1.VectorPipelineRoleAgent)
		reconcileAll()
		Expect(agentConfig()).To(ContainSubstring("srcMoved"))
		Expect(cvaConfig()).NotTo(ContainSubstring("srcMoved"))
	})
})
