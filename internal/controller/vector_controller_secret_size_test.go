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
	"bytes"
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

// This suite proves the size-attribution counterpart to
// vector_controller_secret_collision_test.go: two pipelines that individually fit
// under corev1.MaxSecretSize but together overflow the shared secret-assets Secret
// must only fail the YOUNGER pipeline (by CreationTimestamp), keep the workload
// running on the older pipeline's references, and never silently corrupt or freeze
// the workload - the live cross-tenant overflow repro, now attributed instead of
// freezing the whole workload with an unattributed "Too long: may not be more than
// 1048576 bytes" API error.
//
// It also proves the safe-publication contract at its sharpest edge: a pipeline's
// status must never claim it is valid before its values are actually part of the
// published config. Deleting the older pipeline (whose data was already staged in the
// assets Secret) does NOT let the younger pipeline reinstate on the very next
// reconcile. Recomputing the merged config for younger alone fits comfortably under
// the size limit on its own, but the assets Secret still holds the older pipeline's
// stale 600 KiB (never pruned in the very same round it stops being referenced - see
// ensureVectorAgentSecretAssets' deferred-prune doc comment: a pod could still be
// running the config that references it), and staging both together overflows.
//
// config.BridgeAssets/planSecretAssetsBridge solve this without ever writing
// something inconsistent: the round that discovers the overflow publishes a
// narrower "bridge" config (excluding whichever pipeline does not yet fit - here,
// younger) built only from what is already safely staged, and leaves the excluded
// pipeline with a "waiting for room" status - not failed, not valid, just delayed.
// Once that bridge round's config is confirmed unchanged on a LATER reconcile AND
// SecretAssetsPruneGracePeriod has elapsed since it was published (see
// secretAssetsPruneDecision), the deferred prune finally drops the stale key, and the
// round after that admits the waiting pipeline normally. No terminal failure, no
// manual intervention, no possibility of a config referencing a key the assets
// Secret does not have - resolved automatically over a few self-triggered and
// (once) explicitly requeued reconciles (each write is its own
// Secret change, and Owns(&corev1.Secret{}) turns that into a prompt re-reconcile).
var _ = Describe("VectorReconciler secret-assets size limit attribution", func() {
	It("fails only the younger pipeline, keeps the workload on the older one, and converges a swap that would itself overflow over a few reconciles", func() {
		olderNS := "size-team-a"
		youngerNS := "size-team-b"

		By("creating the two namespaces the fixture needs")
		for _, ns := range []string{olderNS, youngerNS} {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		}
		DeferCleanup(func() {
			for _, ns := range []string{olderNS, youngerNS} {
				_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
			}
		})

		By("creating two Secrets whose values individually fit under the 1 MiB limit but together overflow it")
		bigValue := bytes.Repeat([]byte("a"), 600000)
		olderSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: olderNS},
			Data:       map[string][]byte{"cert": bigValue},
		}
		youngerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: youngerNS},
			Data:       map[string][]byte{"cert": bigValue},
		}
		Expect(k8sClient.Create(ctx, olderSecret)).To(Succeed())
		Expect(k8sClient.Create(ctx, youngerSecret)).To(Succeed())

		newPipeline := func(ns, name, sourceKey string) *v1alpha1.VectorPipeline {
			return &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: v1alpha1.VectorPipelineSpec{
					Secret: map[string]v1alpha1.PipelineSecretBackend{
						"es": {Type: "kubernetes_secret", Name: "creds"},
					},
					Sources: &runtime.RawExtension{Raw: []byte(`{"` + sourceKey + `": {"type": "kubernetes_logs"}}`)},
					Sinks: &runtime.RawExtension{Raw: []byte(
						`{"out": {"type": "elasticsearch", "inputs": ["` + sourceKey + `"], "auth": {"user": "SECRET[es.cert]"}}}`,
					)},
				},
			}
		}

		By("creating the older pipeline and seeding it individually valid (real per-pipeline configcheck cannot " +
			"run in envtest - same rationale as the collision suite)")
		older := newPipeline(olderNS, "app", "srcOld")
		Expect(k8sClient.Create(ctx, older)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, older) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		older.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, older, pipelineStatusBase(older))).To(Succeed())

		// CreationTimestamp is second-precision in the Kubernetes API, so the two
		// pipelines need a real gap for attribution ("oldest survives") to be
		// unambiguous.
		time.Sleep(1100 * time.Millisecond)

		By("creating the younger pipeline the same way")
		younger := newPipeline(youngerNS, "app", "srcNew")
		Expect(k8sClient.Create(ctx, younger)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, younger) })
		younger.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, younger, pipelineStatusBase(younger))).To(Succeed())

		By("creating the Vector agent workload with configcheck disabled (no kubelet in envtest)")
		vectorName := "size-vector"
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

		By("first reconcile: the size overflow must fail only the younger pipeline")
		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		gotOlder := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(older), gotOlder)).To(Succeed())
		Expect(gotOlder.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "the older pipeline must stay valid")
		Expect(gotOlder.Status.Reason).To(BeNil())

		gotYounger := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeFalse()), "the younger pipeline must be excluded and failed")
		Expect(gotYounger.Status.Reason).To(HaveValue(ContainSubstring(secretSizeExclusionReasonPrefix)))
		Expect(gotYounger.Status.Reason).To(HaveValue(ContainSubstring("1048576")),
			"the reason must name the Kubernetes Secret limit")
		Expect(gotYounger.Status.Reason).To(HaveValue(ContainSubstring("Vector")),
			"the reason must name the workload whose merged build discovered the overflow")
		Expect(gotYounger.Status.RelatedSecretsHash).To(BeNil(),
			"clearing it is what lets the pipeline recover once reconsidered - same idiom as the collision path")

		configSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		builtConfig := string(configSecret.Data["agent.json"])
		Expect(builtConfig).To(ContainSubstring(`"size-team-a-app-srcOld"`), "the workload must keep the older pipeline's source")
		Expect(builtConfig).NotTo(ContainSubstring(`"size-team-b-app-srcNew"`), "the younger pipeline's source must be excluded")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveLen(1), "only the older pipeline's value must be in the aggregated assets Secret")

		By("deleting the older pipeline - its 600 KiB stays staged in the assets Secret, unpruned, since a pod could still be reading the config that references it")
		Expect(k8sClient.Delete(ctx, older)).To(Succeed())

		By("second reconcile: younger cannot be staged alongside the still-present stale key, so it waits - the workload itself still succeeds, publishing a narrower bridge config")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		gotVector := &v1alpha1.Vector{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		Expect(gotVector.Status.ConfigCheckResult).To(HaveValue(BeTrue()),
			"the workload reconcile itself must succeed - only younger's own admission is delayed, not the whole build")

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeFalse()),
			"must NOT be reinstated: resolveWorkloadPipelines' own attribution would readmit it, but its values are not actually part of the "+
				"config published this round, so marking it valid here would be exactly the lying green status this whole feature exists to prevent")
		Expect(gotYounger.Status.Reason).To(HaveValue(ContainSubstring(secretAssetsWaitingReasonPrefix)),
			"the reason must reflect the CURRENT situation (waiting for room), not the stale size-exclusion reason from the first reconcile")

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		builtConfig = string(configSecret.Data["agent.json"])
		Expect(builtConfig).NotTo(ContainSubstring(`"size-team-a-app-srcOld"`), "the now-deleted older pipeline's source must be gone from the published config")
		Expect(builtConfig).NotTo(ContainSubstring(`"size-team-b-app-srcNew"`), "younger is not part of this round's bridge")

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveLen(1), "the stale key must still be present - the config that stopped referencing it was only just published this round")

		By("third reconcile: the bridge config is confirmed unchanged, but SecretAssetsPruneGracePeriod has not elapsed since publish yet - the deferred prune must still wait")
		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"nothing else will trigger a fresh reconcile while only the grace period is pending, so the operator must ask to be woken explicitly")

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveLen(1), "the stale key must still be present - the grace period since publish has not elapsed yet")

		By("backdating the publish mark past the grace period, standing in for real time actually passing on a live cluster")
		backdateVectorConfigPublishedAt(vectorName)

		By("fourth reconcile: the grace period has now elapsed too, so the deferred prune finally drops the stale key")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)
		Expect(client.IgnoreNotFound(err)).To(Succeed())
		if err == nil {
			Expect(assetsSecret.Data).To(BeEmpty(), "the stale key must be pruned now that the config which dropped it is both confirmed live and past the grace period")
		}

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeFalse()), "still waiting - room has only just freed up this round")

		By("fifth reconcile: with the stale data actually gone, younger's swap now fits and completes automatically")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeTrue()))
		Expect(gotYounger.Status.Reason).To(BeNil())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		Expect(gotVector.Status.ConfigCheckResult).To(HaveValue(BeTrue()))

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		builtConfig = string(configSecret.Data["agent.json"])
		Expect(builtConfig).To(ContainSubstring(`"size-team-b-app-srcNew"`), "the recovered pipeline's source must be back in the build")

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveLen(1), "only younger's value now - no manual intervention was needed anywhere in this recovery")
	})
})
