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

// This suite proves the deferred-prune compromise documented on
// vectoragent.Controller.ensureVectorAgentSecretAssets: the operator has no
// rollout-completion signal, so instead of dropping a stale secret-assets key in the
// very same reconcile that stops referencing it (zero-delay window for a restarting
// pod to hit a missing key), pruning waits for a LATER reconcile that observes the
// new config already published unchanged AND at least SecretAssetsPruneGracePeriod
// elapsed since that publish (see secretAssetsPruneDecision) - narrowing, not
// eliminating, that window.
var _ = Describe("VectorReconciler secret-assets deferred prune", func() {
	It("keeps the old flat key staged for one extra reconcile after the reference that used it changes, then prunes it", func() {
		ns := "defer-prune-team"

		By("creating the namespace and the Secret with both keys the pipeline will reference in turn")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"k1": []byte("v1"), "k2": []byte("v2")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("creating a pipeline referencing SECRET[es.k1]")
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
		vectorName := "defer-prune-vector"
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

		flatK1 := "defer_prune_team_app_es_k1"
		flatK2 := "defer_prune_team_app_es_k2"

		By("reconcile #1: establishes the baseline with k1's flat key")
		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey(flatK1))

		By("editing the pipeline to reference SECRET[es.k2] instead - k1's flat key drops, k2's appears")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pl), pl)).To(Succeed())
		pl.Spec.Sinks = &runtime.RawExtension{Raw: []byte(
			`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k2]"}}}`,
		)}
		Expect(k8sClient.Update(ctx, pl)).To(Succeed())
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		By("reconcile #2: the config now changes to reference k2 - the prune of k1 must be deferred")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey(flatK2), "the new key must already be staged")
		Expect(assetsSecret.Data).To(HaveKey(flatK1), "the stale key must NOT be pruned in the same round the config that stopped referencing it was published")

		configSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		Expect(string(configSecret.Data["agent.json"])).To(ContainSubstring(flatK2))
		Expect(string(configSecret.Data["agent.json"])).NotTo(ContainSubstring(flatK1), "the published config itself must already only reference the new key")

		By("reconcile #3: config confirmed unchanged, but SecretAssetsPruneGracePeriod has not elapsed since publish yet - the stale key must stay")
		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"nothing else (no Secret write, no pipeline status change) will trigger a fresh reconcile while only the grace period is pending, so the operator must ask to be woken explicitly")
		Expect(result.RequeueAfter).To(BeNumerically("<=", SecretAssetsPruneGracePeriod))

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey(flatK1), "the stale key must NOT be pruned before kubelet has had a chance to project the config that dropped it")

		By("backdating the publish mark past the grace period, standing in for real time actually passing on a live cluster")
		backdateVectorConfigPublishedAt(vectorName)

		By("reconcile #4: the grace period has now elapsed, so the deferred prune finally runs")
		result, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey(flatK2))
		Expect(assetsSecret.Data).NotTo(HaveKey(flatK1), "the stale key must be pruned once a later reconcile confirms both the config that dropped it is already live AND the grace period has elapsed")
	})
})

// backdateVectorConfigPublishedAt rewrites the Vector's status.lastConfigPublishedAt
// to look like it was published SecretAssetsPruneGracePeriod (plus a safety margin)
// in the past, standing in for real time actually passing so tests do not have to
// sleep for the real grace period.
func backdateVectorConfigPublishedAt(vectorName string) {
	v := &v1alpha1.Vector{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName}, v)).To(Succeed())
	past := metav1.NewTime(time.Now().Add(-SecretAssetsPruneGracePeriod - time.Second))
	v.Status.LastConfigPublishedAt = &past
	Expect(k8sClient.Status().Update(ctx, v)).To(Succeed())
}
