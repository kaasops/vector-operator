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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// envtest has no kubelet, so a config check pod never finishes: a pipeline reconcile that
// starts one parks until the timeout. With configCheck.disabled on the selecting workload
// the pipeline must not start one, as the workload itself does not.
var _ = Describe("Pipeline config check with configCheck.disabled", func() {
	newReconciler := func() *PipelineReconciler {
		return &PipelineReconciler{
			Client:                          k8sClient,
			Scheme:                          k8sClient.Scheme(),
			Clientset:                       clientset,
			ConfigCheckTimeout:              5 * time.Second,
			VectorAgentEventCh:              make(chan event.GenericEvent, 10),
			VectorAggregatorsEventCh:        make(chan event.GenericEvent, 10),
			ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 10),
			APIReader:                       k8sClient,
			SecretIndex:                     pipeline.NewSecretIndex(),
		}
	}

	reconcilePipeline := func(vp *v1alpha1.VectorPipeline) (*v1alpha1.VectorPipeline, error) {
		_, err := newReconciler().Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vp)})
		got := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
		return got, err
	}

	createNamespace := func(ns string) {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })
	}

	create := func(obj client.Object) {
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })
	}

	disabled := v1alpha1.VectorCommon{ConfigCheck: v1alpha1.ConfigCheck{Disabled: true}}

	// The other side: with the check enabled it still runs and, with no kubelet here, times out.
	DescribeTable("still runs the config check when it is enabled",
		func(ns string, workload client.Object, sources string) {
			createNamespace(ns)
			create(workload)
			vp := &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: ns},
				Spec: v1alpha1.VectorPipelineSpec{
					Sources: &runtime.RawExtension{Raw: []byte(sources)},
					Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)},
				},
			}
			create(vp)

			got, err := reconcilePipeline(vp)
			Expect(err != nil || got.Status.ConfigCheckResult == nil || !*got.Status.ConfigCheckResult).To(BeTrue(),
				"the pipeline must not be valid without a finished check")
		},
		Entry("agent", "cc-enabled-agent",
			&v1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "cc-enabled-agent"}, Spec: v1alpha1.VectorSpec{Agent: &v1alpha1.VectorAgent{}}},
			`{"in": {"type": "kubernetes_logs"}}`),
		Entry("aggregator", "cc-enabled-aggregator",
			&v1alpha1.VectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "agg", Namespace: "cc-enabled-aggregator"}},
			`{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`),
	)

	It("does not run a config check for an agent pipeline", func() {
		ns := "cc-disabled-agent"
		createNamespace(ns)
		create(&v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
			Spec:       v1alpha1.VectorSpec{Agent: &v1alpha1.VectorAgent{VectorCommon: disabled}},
		})
		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "logs", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "kubernetes_logs"}}`)},
				Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)},
			},
		}
		create(vp)

		got, err := reconcilePipeline(vp)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "reason: %v", got.Status.Reason)
	})

	It("does not run a config check for an aggregator pipeline", func() {
		ns := "cc-disabled-aggregator"
		createNamespace(ns)
		create(&v1alpha1.VectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "agg", Namespace: ns},
			Spec:       v1alpha1.VectorAggregatorSpec{VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{VectorCommon: disabled}},
		})
		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`)},
				Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)},
			},
		}
		create(vp)

		got, err := reconcilePipeline(vp)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "reason: %v", got.Status.Reason)
	})

	It("does not run a config check for a cluster aggregator pipeline", func() {
		ns := "cc-disabled-cluster"
		createNamespace(ns)
		create(&v1alpha1.ClusterVectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "cc-disabled-cagg"},
			Spec: v1alpha1.ClusterVectorAggregatorSpec{
				ResourceNamespace:      ns,
				VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{VectorCommon: disabled},
			},
		})
		cvp := &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "cc-disabled-cweb"},
			Spec: v1alpha1.VectorPipelineSpec{
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`)},
				Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)},
			},
		}
		create(cvp)

		reconciler := newReconciler()
		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cvp)})
		Expect(err).NotTo(HaveOccurred())
		got := &v1alpha1.ClusterVectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "reason: %v", got.Status.Reason)
	})
})
