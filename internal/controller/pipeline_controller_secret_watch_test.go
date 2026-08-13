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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/event"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// This suite proves the Secret watch end to end: it starts a real
// manager with PipelineReconciler.SetupWithManager wired up (unlike the rest of this
// package, which calls Reconcile directly) and observes that a Secret data change
// alone - no spec/annotation change on the pipeline itself - drives a reconcile. It
// also closes a gap: SecretIndex/RelatedSecretsHash were added elsewhere but had no
// Reconcile-level test proving a rotation is actually picked up.
var _ = Describe("PipelineReconciler Secret watch", func() {
	It("requeues a pipeline when its referenced secret changes, and ignores unrelated secrets", func() {
		ns := "default"

		By("creating the referenced secret and an unrelated secret")
		watched := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "watch-test-secret", Namespace: ns},
			Data:       map[string][]byte{"key": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, watched)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, watched) })

		unrelated := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "watch-test-unrelated-secret", Namespace: ns},
			Data:       map[string][]byte{"key": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, unrelated)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, unrelated) })

		By("creating a ClusterVectorAggregator so Reconcile does not early-exit before touching SecretIndex")
		cva := &v1alpha1.ClusterVectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "watch-test-cva"},
		}
		Expect(k8sClient.Create(ctx, cva)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cva) })

		By("creating a VectorPipeline whose sink actually references the watched secret")
		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "watch-test-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"s1": {Type: "kubernetes_secret", Name: watched.Name},
				},
				// A SECRET[] reference is required since resolution/watch scoping went
				// used-refs-only: a declared-but-unused backend is deliberately ignored.
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["src"], "auth": {"user": "SECRET[s1.key]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })

		By("starting a manager with the real PipelineReconciler wired up (SetupWithManager, not a direct Reconcile call)")
		// SkipNameValidation: this is not the only spec that stands up its own
		// manager and calls PipelineReconciler.SetupWithManager (see
		// pipeline_controller_secret_value_safety_test.go) - controller-runtime
		// tracks controller names (both default to "vectorpipeline") in a registry
		// shared process-wide, not per-manager, so without this a second spec's
		// controller registration fails outright depending on ginkgo's randomized
		// run order.
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:     k8sClient.Scheme(),
			Metrics:    metricsserver.Options{BindAddress: "0"},
			Controller: config.Controller{SkipNameValidation: boolPtr(true)},
		})
		Expect(err).NotTo(HaveOccurred())

		reconciler := &PipelineReconciler{
			Client:                          mgr.GetClient(),
			Scheme:                          mgr.GetScheme(),
			Clientset:                       clientset,
			ConfigCheckTimeout:              configCheckTimeout,
			VectorAgentEventCh:              make(chan event.GenericEvent, 10),
			VectorAggregatorsEventCh:        make(chan event.GenericEvent, 10),
			ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 10),
			APIReader:                       mgr.GetAPIReader(),
			SecretIndex:                     pipeline.NewSecretIndex(),
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed())

		mgrCtx, mgrCancel := context.WithCancel(ctx)
		DeferCleanup(mgrCancel)
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		Expect(mgr.GetCache().WaitForCacheSync(mgrCtx)).To(BeTrue())

		By("waiting for the initial reconcile to populate status.relatedSecretsHash")
		var initialHash *int64
		Eventually(func() *int64 {
			got := &v1alpha1.VectorPipeline{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got); err != nil {
				return nil
			}
			initialHash = got.Status.RelatedSecretsHash
			return initialHash
		}, 30*time.Second, 250*time.Millisecond).ShouldNot(BeNil())

		By("updating the watched secret's data")
		latest := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(watched), latest)).To(Succeed())
		preUpdateGeneration := latest.Generation
		latest.Data["key"] = []byte("v2")
		Expect(k8sClient.Update(ctx, latest)).To(Succeed())

		By("confirming empirically that the API server does not bump metadata.generation for this update " +
			"(why specAndAnnotationsPredicate, which gates on generation/annotations, cannot be reused for this watch)")
		Expect(latest.Generation).To(Equal(preUpdateGeneration),
			"Secret has no status subresource, so the API server must not bump generation on a data-only update")

		By("expecting status.relatedSecretsHash to change - proof the watch enqueued a reconcile")
		Eventually(func() *int64 {
			got := &v1alpha1.VectorPipeline{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
			return got.Status.RelatedSecretsHash
		}, 30*time.Second, 250*time.Millisecond).ShouldNot(Equal(initialHash))

		var hashAfterWatchedUpdate *int64
		got := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
		hashAfterWatchedUpdate = got.Status.RelatedSecretsHash

		By("updating the unrelated secret's data - the pipeline must not be requeued for it")
		latestUnrelated := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(unrelated), latestUnrelated)).To(Succeed())
		latestUnrelated.Data["key"] = []byte("v2")
		Expect(k8sClient.Update(ctx, latestUnrelated)).To(Succeed())

		Consistently(func() *int64 {
			got := &v1alpha1.VectorPipeline{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
			return got.Status.RelatedSecretsHash
		}, 5*time.Second, 500*time.Millisecond).Should(Equal(hashAfterWatchedUpdate))
	})
})
