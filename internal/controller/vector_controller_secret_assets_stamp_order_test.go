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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// This suite pins the status-write-ordering hazard: LastConfigPublishedAt used to
// be written ONLY by the final
// SetSuccessStatus, which runs after EnsureVectorAgent (DaemonSet, RBAC, Service,
// PodMonitor), reinstatePipelines, and the status update itself - so a round that
// successfully published a new config but then failed on any of those later steps
// left the mark exactly where a much earlier round left it. A retry afterward would
// see PublishedConfigMatches(active) flip true (the config really is already
// published) and, gated on that stale mark alone, could open the grace period's
// door immediately instead of the round the config was actually published in.
// EnsureVectorAgent's StampConfigPublishing closes this by writing the mark right
// after assets succeed and immediately before the first config write - see its own
// doc comment for the full rationale, including why it writes through a deep copy.
var _ = Describe("VectorReconciler config-publish stamp ordering", func() {
	setupFixture := func(ns, vectorName string) (pl *v1alpha1.VectorPipeline, vector *v1alpha1.Vector) {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"k1": []byte("v1"), "k2": []byte("v2")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		pl = &v1alpha1.VectorPipeline{
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

		vector = &v1alpha1.Vector{
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
		return pl, vector
	}

	newReconciler := func(c client.Client) *VectorReconciler {
		return &VectorReconciler{
			Client:             c,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
	}

	It("preserves the union and returns a non-zero grace remainder on retry when a write AFTER the config write fails", func() {
		ns := "stamp-order-post-fail"
		vectorName := "stamp-order-post-fail-vector"
		pl, _ := setupFixture(ns, vectorName)

		By("reconcile #1: establishes the baseline with k1")
		_, err := newReconciler(k8sClient).Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName},
		})
		Expect(err).NotTo(HaveOccurred())

		// Without this the spec proves nothing: reconcile #1's own SetSuccessStatus
		// seeds LastConfigPublishedAt, and an envtest spec runs its remaining rounds
		// within seconds - so even with StampConfigPublishing reverted, reconcile #3
		// would still find a mark well inside the 90-second grace period and requeue,
		// passing on the strength of the clock rather than of the fix. Backdating that
		// seeded mark past the grace period is what makes the two behaviours diverge:
		// WITH the fix, reconcile #2 stamps a fresh mark immediately before its config
		// write and #3 must still gate on it; WITHOUT it, #3 sees only this stale mark
		// and prunes on the spot. It cannot change what reconcile #2 itself does - that
		// round's config IS changing, so its prune decision is "not now" regardless of
		// any mark.
		By("backdating the seeded publish mark past the grace period")
		backdateVectorConfigPublishedAt(vectorName)

		By("editing the pipeline to reference k2 instead")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pl), pl)).To(Succeed())
		pl.Spec.Sinks = &runtime.RawExtension{Raw: []byte(
			`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k2]"}}}`,
		)}
		Expect(k8sClient.Update(ctx, pl)).To(Succeed())
		agentRole := v1alpha1.VectorPipelineRoleAgent
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		// The DaemonSet and the active config Secret happen to share the same name
		// (both "<vector>-agent") - two separate variables purely for readability
		// at each call site below, not because the values differ.
		daemonSetName := vectorName + "-agent"
		configSecretName := vectorName + "-agent"
		injected := errors.New("injected daemonset write failure, after config already landed")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if ds, ok := obj.(*appsv1.DaemonSet); ok && ds.Name == daemonSetName {
					return injected
				}
				return c.Update(ctx, obj, opts...)
			},
		})

		By("reconcile #2 (DaemonSet write fails, AFTER the config write already succeeded): the round errors out")
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}
		_, err = newReconciler(faultyClient).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected DaemonSet write failure must surface as a Reconcile error")

		configSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: configSecretName}, configSecret)).To(Succeed())
		Expect(string(configSecret.Data["agent.json"])).To(ContainSubstring("stamp_order_post_fail_app_es_k2"),
			"the config write itself succeeded before the DaemonSet write failed")

		By("reconcile #3 (no fault, immediate retry): the config now matches, but the mark StampConfigPublishing set in reconcile #2 is still fresh")
		result, err := newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"a stale pre-fix mark would have let this retry prune immediately (RequeueAfter == 0) - the fresh mark from reconcile #2's "+
				"StampConfigPublishing must still gate it behind the grace period")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey("stamp_order_post_fail_app_es_k1"),
			"the union (including the stale key) must still be staged - nothing may be pruned while grace is still pending")
		Expect(assetsSecret.Data).To(HaveKey("stamp_order_post_fail_app_es_k2"))
	})

	It("never attempts the config write at all when the pre-publish stamp write itself fails", func() {
		ns := "stamp-order-pre-fail"
		vectorName := "stamp-order-pre-fail-vector"
		setupFixture(ns, vectorName)

		configSecretName := vectorName + "-agent"
		injected := errors.New("injected status stamp failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				if subResourceName == "status" {
					if v, ok := obj.(*v1alpha1.Vector); ok && v.Name == vectorName {
						return injected
					}
				}
				return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		})

		By("reconcile #1 (status stamp write fails, on the workload's very first - purely additive - publish)")
		_, err = newReconciler(faultyClient).Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName},
		})
		Expect(err).To(HaveOccurred(), "the injected stamp-write failure must surface as a Reconcile error")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed(),
			"assets must already be published - the stamp write is the step immediately AFTER it, not before")

		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: configSecretName}, &corev1.Secret{})
		Expect(client.IgnoreNotFound(err)).To(Succeed())
		Expect(err).To(HaveOccurred(), "the config Secret must not exist - the stamp write is a PRECONDITION of publishing, "+
			"not a best-effort side attempt: its failure must block the config write outright")
	})
})
