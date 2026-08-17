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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// This suite proves a fixed bug: a secret backend's namespace shape rule (namespace
// forbidden on VectorPipeline, required on ClusterVectorPipeline) is checked before
// resolveRelatedSecrets ever calls Get. Before the fix, a ClusterVectorPipeline backend
// missing its namespace fell straight into a Get-by-name call with an empty namespace,
// which client-go rejects with its own confusing error text instead of the designed
// "namespace is required" message - and the reconcile requeued that permanent spec
// error on the 10s resolve-retry timer forever, instead of leaving it to the pipeline's
// own generation-change watch (which already retriggers on a spec edit).
var _ = Describe("PipelineReconciler secret backend namespace shape validation", func() {
	It("fails a ClusterVectorPipeline backend without namespace with the designed message and no RequeueAfter", func() {
		secretName := "shape-test-cvp-secret"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
			Data:       map[string][]byte{"k": []byte("v")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		By("creating a ClusterVectorAggregator so Reconcile does not early-exit before touching secret resolution")
		cva := &v1alpha1.ClusterVectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "shape-test-cvp-cva"}}
		Expect(k8sClient.Create(ctx, cva)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cva) })

		cvp := &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "shape-test-cvp"},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					// Namespace intentionally left empty - forbidden shape for a CVP backend.
					"es": {Type: "kubernetes_secret", Name: secretName},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cvp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cvp) })

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
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: cvp.Name}}

		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(), "a permanent spec error must not be retried on a timer")

		got := &v1alpha1.ClusterVectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.Reason).To(HaveValue(ContainSubstring("namespace is required")))
		Expect(got.Status.Reason).NotTo(HaveValue(ContainSubstring("empty namespace may not be set")),
			"must surface the designed message, not the client-go error text")
	})

	It("fails a VectorPipeline backend with namespace set with the designed message and no RequeueAfter", func() {
		ns := "default"
		secretName := "shape-test-vp-secret"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data:       map[string][]byte{"k": []byte("v")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		By("creating a ClusterVectorAggregator so Reconcile does not early-exit before touching secret resolution")
		cva := &v1alpha1.ClusterVectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "shape-test-vp-cva"}}
		Expect(k8sClient.Create(ctx, cva)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cva) })

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "shape-test-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					// Namespace intentionally set - forbidden shape for a VP backend.
					"es": {Type: "kubernetes_secret", Name: secretName, Namespace: ns},
				},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })

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
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vp.Name}}

		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(), "a permanent spec error must not be retried on a timer")

		got := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
		Expect(got.Status.Reason).To(HaveValue(ContainSubstring("namespace is not allowed in VectorPipeline")))
	})
})
