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

// This suite is the end-to-end counterpart to internal/config's
// TestBuildAgentConfigRejectsUnsafeSecretValue and friends: those pin the guard
// itself (config.secretValueSafeForJSONText, checked on every path that reads a
// secret value), this suite pins what the OPERATOR does with the failure once it
// reaches a real pipeline's status, through a real manager with the Secret watch
// wired up (SetupWithManager, not a direct Reconcile call - the same harness
// pipeline_controller_secret_watch_test.go uses).
//
// Two things this suite is the only place that proves: (1) the failure reason never
// repeats any part of the unsafe value - .status.reason is readable by anyone with
// RBAC on the pipeline, a far wider audience than whoever can read the Secret itself,
// so it must never become a side channel for the secret's content; (2) rotating the
// Secret to a safe value recovers the pipeline all the way to ConfigCheckResult=true
// WITHOUT any manual trigger - proving config.SecretValueUnsafeError is classified as
// a permanent-until-the-secret-actually-changes failure (like a missing key), not a
// transient one like config.SecretResolveError, which would instead retry blindly on
// a short timer.
//
// The aggregator's ResourceNamespace is deliberately a namespace that is never
// created: PipelineReconciler's own per-pipeline validation (unlike the workload
// reconcilers') never honors ConfigCheck.Disabled - it always tries to create a real
// configcheck pod, and envtest has no kubelet to ever start one. Pointing configcheck
// at a namespace k8s.NamespaceIsTerminating reports as gone (NotFound counts, see its
// own doc comment) makes ConfigCheck.Run take its existing, legitimate
// ErrConfigcheckSkipped path instead - reaching a REAL, honest SetSuccessStatus
// (ConfigCheckResult=true, Reason=nil) without needing a kubelet, rather than relying
// on a stand-in signal for "would have reached configcheck".
//
// Uses a ClusterVectorAggregator + ClusterVectorPipeline, the same combination
// pipeline_controller_secret_build_resolve_test.go already proved reaches
// BuildAggregatorConfig's real secret-resolution path (and, via the CVP watch
// alongside the Secret watch, is reachable purely by a Secret rotation with no
// spec/annotation change on the pipeline itself).
var _ = Describe("PipelineReconciler secret value safety", func() {
	It("fails a pipeline on a JSON-unsafe secret value without leaking it, and recovers automatically to ConfigCheckResult=true once the secret rotates to a safe one", func() {
		ns := "default"
		secretName := "value-safety-secret"

		By("creating a ClusterVectorAggregator whose ResourceNamespace does not exist, so configcheck takes its ErrConfigcheckSkipped path instead of needing a kubelet")
		cva := &v1alpha1.ClusterVectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "value-safety-cva"},
			Spec: v1alpha1.ClusterVectorAggregatorSpec{
				ResourceNamespace: "value-safety-missing-ns",
			},
		}
		Expect(k8sClient.Create(ctx, cva)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cva) })

		By("creating the secret with an initially safe value")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data:       map[string][]byte{"cert": []byte("safe-value-1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		By("creating a ClusterVectorPipeline (aggregator role) referencing it")
		cvp := &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "value-safety-cvp"},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: secretName, Namespace: ns},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.cert]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, cvp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cvp) })

		By("starting a manager with the real PipelineReconciler wired up (SetupWithManager, not a direct Reconcile call)")
		// SkipNameValidation: this suite is not the only spec that stands up its own
		// manager and calls PipelineReconciler.SetupWithManager (see
		// pipeline_controller_secret_watch_test.go) - controller-runtime tracks
		// controller names (both default to "vectorpipeline") in a registry shared
		// process-wide, not per-manager, so without this a second spec's controller
		// registration fails outright depending on ginkgo's randomized run order.
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

		By("waiting for the pipeline to become genuinely valid with the initial safe value")
		Eventually(func() *bool {
			got := &v1alpha1.ClusterVectorPipeline{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
			return got.Status.ConfigCheckResult
		}, 30*time.Second, 250*time.Millisecond).Should(HaveValue(BeTrue()))
		got := &v1alpha1.ClusterVectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.Reason).To(BeNil())

		By("rotating the secret to a JSON-unsafe value (a double quote, plus a distinctive marker to check for leakage)")
		const secretMarker = "xk7Q-p0nyZ4rd"
		unsafeValue := secretMarker + `"` + secretMarker
		latest := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), latest)).To(Succeed())
		latest.Data["cert"] = []byte(unsafeValue)
		Expect(k8sClient.Update(ctx, latest)).To(Succeed())

		By("expecting the pipeline to fail attributed to the secret/key, without leaking the value")
		var gotReason *string
		Eventually(func() *bool {
			got := &v1alpha1.ClusterVectorPipeline{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
			gotReason = got.Status.Reason
			return got.Status.ConfigCheckResult
		}, 30*time.Second, 250*time.Millisecond).Should(HaveValue(BeFalse()))

		Expect(gotReason).NotTo(BeNil())
		Expect(*gotReason).To(ContainSubstring(secretName), "the reason must name the Secret")
		Expect(*gotReason).To(ContainSubstring("cert"), "the reason must name the key")
		Expect(*gotReason).NotTo(ContainSubstring(unsafeValue), "the reason must never repeat the unsafe value")
		Expect(*gotReason).NotTo(ContainSubstring(secretMarker), "the reason must not leak even a fragment of the value's content")

		By("rotating the secret to a new, safe value - the pipeline must recover automatically, all the way to ConfigCheckResult=true, with no manual trigger")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), latest)).To(Succeed())
		latest.Data["cert"] = []byte("safe-value-2")
		Expect(k8sClient.Update(ctx, latest)).To(Succeed())

		Eventually(func() *bool {
			got := &v1alpha1.ClusterVectorPipeline{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
			return got.Status.ConfigCheckResult
		}, 30*time.Second, 250*time.Millisecond).Should(HaveValue(BeTrue()))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.Reason).To(BeNil(), "recovery must clear the stale failure reason")
	})
})
