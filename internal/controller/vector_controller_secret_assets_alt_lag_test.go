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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

// This suite pins the checkpoint-migration hazard: under checkpoint migration,
// PublishedConfigMatches on the ACTIVE config Secret alone is not a safe enough
// signal to gate pruning, because the two variants are independent objects that can
// fail to write independently of each other. A round where the alt (standby) write
// keeps failing while the active write succeeds must never let the active variant's
// own "unchanged" status open the door to pruning a key the still-stale alt config
// Secret continues to reference - allConfigsUnchanged (vector_controller.go) exists
// exactly to require BOTH variants confirmed matching before that door opens.
//
// Real fault injection (not just an assertion on final state) is used for the same
// reason vector_controller_secret_assets_write_order_test.go uses it: a regression
// that went back to gating on the active variant alone would compile and pass every
// other test in this package, since with no injected failure both variants converge
// together anyway.
var _ = Describe("VectorReconciler secret-assets pruning under a lagging alt variant", func() {
	It("keeps the stale key staged in both variants while the alt config Secret repeatedly fails to catch up, and only prunes once both are confirmed AND the grace period has elapsed", func() {
		ns := "alt-lag-team"

		By("creating the namespace and the Secret with both keys the pipeline will reference in turn")
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
		vectorName := "alt-lag-vector"
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

		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}
		activeConfigName := vectorName + "-agent"
		altConfigName := vectorName + "-agent-opt"
		activeAssetsName := types.NamespacedName{Namespace: "default", Name: activeConfigName + "-secret-assets"}
		altAssetsName := types.NamespacedName{Namespace: "default", Name: altConfigName + "-secret-assets"}
		flatK1 := "alt_lag_team_app_es_k1"
		flatK2 := "alt_lag_team_app_es_k2"

		newReconciler := func(c client.Client) *VectorReconciler {
			return &VectorReconciler{
				Client:                    c,
				Scheme:                    k8sClient.Scheme(),
				Clientset:                 clientset,
				ConfigCheckTimeout:        configCheckTimeout,
				DiscoveryClient:           clientset.DiscoveryClient,
				EventChan:                 make(chan event.GenericEvent, 1),
				APIReader:                 k8sClient,
				EnableCheckpointMigration: true,
				CheckpointMergerImage:     "example.com/checkpoint-merger:test",
			}
		}

		// faultyOnAlt fails every Update to the alt (standby) config Secret, and
		// passes everything else through to a fresh watch client - simulating a
		// standby write that keeps failing (a write conflict, a transient API
		// error, a quota rejection specific to that object) while the active one
		// keeps succeeding.
		faultyOnAlt := func() client.Client {
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			return interceptor.NewClient(watchClient, interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if s, ok := obj.(*corev1.Secret); ok && s.Name == altConfigName {
						return errors.New("injected alt config write failure")
					}
					return c.Update(ctx, obj, opts...)
				},
			})
		}

		By("reconcile #1 (no fault): establishes the baseline, both variants reference k1")
		_, err := newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		activeAssets := &corev1.Secret{}
		altAssets := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, activeAssetsName, activeAssets)).To(Succeed())
		Expect(k8sClient.Get(ctx, altAssetsName, altAssets)).To(Succeed())
		Expect(activeAssets.Data).To(HaveKey(flatK1))
		Expect(altAssets.Data).To(HaveKey(flatK1))

		gotVector := &v1alpha1.Vector{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		Expect(gotVector.Status.LastConfigPublishedAt).NotTo(BeNil(), "reconcile #1 is the first-ever publish, so the mark is seeded here")
		markAfterRound1 := *gotVector.Status.LastConfigPublishedAt

		By("editing the pipeline to reference SECRET[es.k2] instead")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pl), pl)).To(Succeed())
		pl.Spec.Sinks = &runtime.RawExtension{Raw: []byte(
			`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k2]"}}}`,
		)}
		Expect(k8sClient.Update(ctx, pl)).To(Succeed())
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		// LastConfigPublishedAt is a Kubernetes API timestamp (RFC3339, second
		// precision once round-tripped through the server) - the sleeps below give
		// the strict "advanced" assertions a real second to land in, the same
		// pattern already used for CreationTimestamp ordering elsewhere in this
		// package (e.g. vector_controller_secret_size_test.go).
		time.Sleep(1100 * time.Millisecond)

		By("reconcile #2 (alt write fails): active catches up to k2, alt stays on k1, assets stage the union")
		_, err = newReconciler(faultyOnAlt()).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected alt config write failure must surface as a Reconcile error")

		activeConfig := &corev1.Secret{}
		altConfig := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: activeConfigName}, activeConfig)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: altConfigName}, altConfig)).To(Succeed())
		Expect(string(activeConfig.Data["agent.json"])).To(ContainSubstring(flatK2), "the active config must already reference the new key")
		Expect(string(altConfig.Data["agent.json"])).To(ContainSubstring(flatK1), "the alt config write failed - it must still be the old one, referencing k1")
		Expect(string(altConfig.Data["agent.json"])).NotTo(ContainSubstring(flatK2))

		// StampConfigPublishing (the status-write-ordering fix) fires here
		// even though the round as a whole errors out: allConfigsUnchanged is false
		// this round (alt has not caught up), so EnsureVectorAgent stamps the mark
		// right after assets succeed and before the active config write - which is
		// BEFORE the alt write that then fails. The mark advancing on a round that
		// is still trying to publish is correct, not a regression: it is exactly
		// what keeps a LATER retry (once active alone looks "unchanged") from
		// gating on a stale mark left over from long before this attempt started -
		// see the checkpoint-migration hazard described at the top of this suite,
		// the bug this suite pins.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		Expect(gotVector.Status.LastConfigPublishedAt).NotTo(BeNil())
		Expect(gotVector.Status.LastConfigPublishedAt.Time.After(markAfterRound1.Time)).To(BeTrue(),
			"the mark must advance on reconcile #2 - it is still trying to publish (alt has not caught up), just failing partway through")
		markAfterRound2 := *gotVector.Status.LastConfigPublishedAt

		time.Sleep(1100 * time.Millisecond)

		By("reconcile #3 (alt write fails AGAIN): the active retry alone must not be mistaken for both variants being caught up")
		_, err = newReconciler(faultyOnAlt()).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred())

		Expect(k8sClient.Get(ctx, activeAssetsName, activeAssets)).To(Succeed())
		Expect(k8sClient.Get(ctx, altAssetsName, altAssets)).To(Succeed())
		Expect(activeAssets.Data).To(HaveKey(flatK1),
			"pruning must not fire just because the ACTIVE config matched on retry - the alt config Secret is still stale and still needs k1")
		Expect(altAssets.Data).To(HaveKey(flatK1))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		Expect(gotVector.Status.LastConfigPublishedAt).NotTo(BeNil())
		Expect(gotVector.Status.LastConfigPublishedAt.Time.After(markAfterRound2.Time)).To(BeTrue(),
			"reconcile #3 is still trying to publish too (alt is STILL stale), so the mark advances again - the point is that pruning stays "+
				"gated on allConfigsUnchanged regardless of how many times this happens, not that the mark stands still")

		By("reconcile #4 (no fault): the alt write finally succeeds and catches up to k2")
		_, err = newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: altConfigName}, altConfig)).To(Succeed())
		Expect(string(altConfig.Data["agent.json"])).To(ContainSubstring(flatK2), "the alt config must now be caught up")
		Expect(string(altConfig.Data["agent.json"])).NotTo(ContainSubstring(flatK1))

		Expect(k8sClient.Get(ctx, activeAssetsName, activeAssets)).To(Succeed())
		Expect(activeAssets.Data).To(HaveKey(flatK1), "still not pruned - both variants only just became consistent THIS round, the grace period has not started yet")

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		Expect(gotVector.Status.LastConfigPublishedAt).NotTo(BeNil(), "this round is the first to actually publish something (the alt catch-up), so the mark is seeded here")

		By("reconcile #5 (no fault, immediate): both variants are now confirmed unchanged, but the grace period has not elapsed - still deferred")
		result, err := newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		Expect(k8sClient.Get(ctx, activeAssetsName, activeAssets)).To(Succeed())
		Expect(k8sClient.Get(ctx, altAssetsName, altAssets)).To(Succeed())
		Expect(activeAssets.Data).To(HaveKey(flatK1))
		Expect(altAssets.Data).To(HaveKey(flatK1))

		By("backdating the publish mark past the grace period, standing in for real time actually passing")
		backdateVectorConfigPublishedAt(vectorName)

		By("reconcile #6: both variants confirmed unchanged AND the grace period has elapsed - the stale key is finally pruned from both")
		result, err = newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		Expect(k8sClient.Get(ctx, activeAssetsName, activeAssets)).To(Succeed())
		Expect(k8sClient.Get(ctx, altAssetsName, altAssets)).To(Succeed())
		Expect(activeAssets.Data).NotTo(HaveKey(flatK1))
		Expect(altAssets.Data).NotTo(HaveKey(flatK1))
		Expect(activeAssets.Data).To(HaveKey(flatK2))
		Expect(altAssets.Data).To(HaveKey(flatK2))
	})
})
