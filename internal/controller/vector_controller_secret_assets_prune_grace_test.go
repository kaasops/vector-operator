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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// This suite proves the specific failure mode secretAssetsPruneDecision's anchoring
// is meant to rule out: a workload that reconciles far more often than
// SecretAssetsPruneGracePeriod (a debounce train, an unrelated watch firing, ...)
// must still converge to actually pruning once the real grace period elapses, not
// have its wait pushed out indefinitely because writing the publish mark itself
// looks like "the config changed again" to the next reconcile. See
// vectoragent.Controller.SetSuccessStatus / aggregator.Controller.SetSuccessStatus,
// which only (re)stamp LastConfigPublishedAt when the config actually changed this
// round or no mark exists yet - never on every call.
//
// The pipeline switches from SECRET[es.k1] to SECRET[es.k2] partway through: a
// round that would not actually drop anything (the common no-secrets-used, or
// purely-additive, case) never enters the grace/requeue path at all any more (see
// assetsWouldDropAKey) - it is not the case this suite is about, and asserting a
// requeue there would just be re-asserting a bug already fixed elsewhere: a round
// that drops nothing used to still be forced through the grace/requeue path. This
// suite needs an ACTUAL stale key so the anchoring logic is the thing under test,
// not the fast path that skips it.
var _ = Describe("VectorReconciler secret-assets prune grace anchoring", func() {
	It("keeps the publish mark fixed and the requeue deadline non-increasing across repeated no-op reconciles", func() {
		ns := "prune-grace-team"

		By("creating the namespace and a Secret with both keys the pipeline will reference in turn")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"k1": []byte("v1"), "k2": []byte("v2")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		pl := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k1]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, pl)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		By("creating the Vector agent workload with configcheck disabled (no kubelet in envtest)")
		vectorName := "prune-grace-vector"
		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: vectorName, Namespace: "default"},
			Spec: v1alpha1.VectorSpec{
				Agent: &v1alpha1.VectorAgent{
					VectorCommon: v1alpha1.VectorCommon{
						ConfigCheck: v1alpha1.ConfigCheck{Disabled: true},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, vector)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vector) })

		reconciler := &VectorReconciler{
			Client:             k8sClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}

		By("reconcile #1: the config is published for the first time - purely additive, so no grace wait yet")
		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(), "nothing existing is being dropped by this round's publish")

		By("editing the pipeline to reference SECRET[es.k2] instead - k1's flat key now becomes a stale, droppable key")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pl), pl)).To(Succeed())
		pl.Spec.Sinks = &runtime.RawExtension{Raw: []byte(
			`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k2]"}}}`,
		)}
		Expect(k8sClient.Update(ctx, pl)).To(Succeed())
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		By("reconcile #2: the config changes to reference k2 - the publish mark resets here")
		result, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(), "the config itself is changing this round, not yet the 'unchanged, waiting on grace' case")

		gotVector := &v1alpha1.Vector{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName}, gotVector)).To(Succeed())
		Expect(gotVector.Status.LastConfigPublishedAt).NotTo(BeNil())
		publishedAt := *gotVector.Status.LastConfigPublishedAt

		By("reconcile #3 and #4, immediately after: the config is now unchanged but k1 is still a stale, undropped key - each is the 'waiting on grace' case")
		var previousRequeueAfter *time.Duration
		for i := 0; i < 2; i++ {
			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"the grace period has not elapsed yet, and nothing else will trigger a fresh reconcile on its own")

			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName}, gotVector)).To(Succeed())
			Expect(gotVector.Status.LastConfigPublishedAt).NotTo(BeNil())
			Expect(gotVector.Status.LastConfigPublishedAt.Time).To(BeTemporally("==", publishedAt.Time),
				"repeated no-op reconciles must NOT push the publish mark forward - doing so would defer pruning indefinitely on a workload reconciled more often than the grace period")

			if previousRequeueAfter != nil {
				Expect(result.RequeueAfter).To(BeNumerically("<=", *previousRequeueAfter),
					"the remaining wait must shrink (real time passing against the same fixed anchor), never grow, across repeated calls")
			}
			previousRequeueAfter = &result.RequeueAfter
		}
	})

	// The other half of the same binding, and the one no other spec pinned: a
	// workload whose assets have CONVERGED (it uses spec.secret, and this round's
	// exact target drops nothing) must not enter the grace/requeue path at all.
	// assetsWouldDropAKey is the guard that keeps it out, and it is only wired in at
	// one call site in createOrUpdateVector - unwiring it there compiles, keeps every
	// other spec in this package green, and quietly buys every steady-state reconcile
	// of every secret-using workload a scheduled wakeup 90 seconds later, forever.
	It("does not schedule a grace requeue for a converged workload whose repeat reconcile drops nothing", func() {
		ns := "prune-grace-converged"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"k1": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		pl := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k1]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, pl)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		vectorName := "prune-grace-converged-vector"
		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: vectorName, Namespace: "default"},
			Spec: v1alpha1.VectorSpec{
				Agent: &v1alpha1.VectorAgent{
					VectorCommon: v1alpha1.VectorCommon{
						ConfigCheck: v1alpha1.ConfigCheck{Disabled: true},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, vector)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vector) })

		reconciler := &VectorReconciler{
			Client:             k8sClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}

		By("reconcile #1: the first publish - additive, so no grace wait")
		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		By("reconcile #2: nothing changed, and the exact target is what is already staged")
		result, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(),
			"the config is unchanged and the publish mark is minutes short of the grace period, so the grace path WOULD requeue here - "+
				"it must never be entered at all when publishing the exact target drops no key")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey("prune_grace_converged_app_es_k1"),
			"and the workload really is a secret user - otherwise this would be asserting the empty-map case instead")
	})
})
