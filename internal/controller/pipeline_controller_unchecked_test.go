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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// A pipeline reconciled before any workload exists returns without a status. The first
// workload must bring it back, and only pipelines that were never checked.
var _ = Describe("Pipelines created before the first workload", func() {
	It("are reconciled again when a workload is created", func() {
		ns := "unchecked-pipelines"
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		reconciler := &PipelineReconciler{
			Client:                          k8sClient,
			Scheme:                          k8sClient.Scheme(),
			Clientset:                       clientset,
			ConfigCheckTimeout:              configCheckTimeout,
			VectorAgentEventCh:              make(chan event.GenericEvent, 10),
			VectorAggregatorsEventCh:        make(chan event.GenericEvent, 10),
			ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 10),
			APIReader:                       k8sClient,
			SecretIndex:                     pipeline.NewSecretIndex(),
		}
		newPipeline := func(name string) *v1alpha1.VectorPipeline {
			vp := &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: v1alpha1.VectorPipelineSpec{
					Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "kubernetes_logs"}}`)},
					Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["in"]}}`)},
				},
			}
			Expect(k8sClient.Create(ctx, vp)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })
			return vp
		}
		reconcileVP := func(vp *v1alpha1.VectorPipeline) *v1alpha1.VectorPipeline {
			_, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vp)})
			Expect(err).NotTo(HaveOccurred())
			got := &v1alpha1.VectorPipeline{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
			return got
		}

		// The shared envtest may hold workloads from other specs, so the early return itself is
		// not reproduced here: a pipeline that was never reconciled has the same empty status.
		early := newPipeline("early")

		agent := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
			Spec: v1alpha1.VectorSpec{Agent: &v1alpha1.VectorAgent{
				VectorCommon: v1alpha1.VectorCommon{ConfigCheck: v1alpha1.ConfigCheck{Disabled: true}},
			}},
		}
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent) })

		checked := newPipeline("checked")
		Expect(reconcileVP(checked).Status.ConfigCheckResult).To(HaveValue(BeTrue()))

		// A pipeline being deleted is left alone, as the workloads skip it too.
		deleting := newPipeline("deleting")
		deleting.Finalizers = []string{"test.kaasops.io/hold"}
		Expect(k8sClient.Update(ctx, deleting)).To(Succeed())
		Expect(k8sClient.Delete(ctx, deleting)).To(Succeed())
		DeferCleanup(func() {
			got := &v1alpha1.VectorPipeline{}
			if k8sClient.Get(ctx, client.ObjectKeyFromObject(deleting), got) == nil {
				got.Finalizers = nil
				_ = k8sClient.Update(ctx, got)
			}
		})

		Expect(reconciler.mapWorkloadToUncheckedPipelines(ctx, agent)).To(ConsistOf(
			reconcile.Request{NamespacedName: client.ObjectKeyFromObject(early)},
		), "only the pipeline that was never checked")

		Expect(reconcileVP(early).Status.ConfigCheckResult).To(HaveValue(BeTrue()))
	})
})
