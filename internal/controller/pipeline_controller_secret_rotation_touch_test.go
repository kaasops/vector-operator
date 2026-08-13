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
	"testing"

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

// These three specs back the manual-rotation recipe documented for scoped mode
// (--watch-namespace / --watch-name, see docs/secrets.md), one link of the chain each:
// an annotation-only edit reaches the pipeline reconcile at all; that reconcile does
// not dismiss a silently rotated Secret as "no changes"; and the workload reconcile it
// then triggers re-reads the values rather than serving whatever the cache holds.
// A documented recipe with no test is a claim, not a feature.

// Link 1: the pipeline's own generation does not move for a metadata-only edit, so
// without the annotations arm of the predicate the recipe would never wake anything.
func TestSpecAndAnnotationsPredicate_AnnotationOnlyEditPasses(t *testing.T) {
	g := NewWithT(t)

	old := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "vp", Namespace: "default", Generation: 7},
	}
	updated := &v1alpha1.VectorPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vp", Namespace: "default", Generation: 7,
			Annotations: map[string]string{"example.com/rotate": "1"},
		},
	}

	g.Expect(specAndAnnotationsPredicate.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: updated})).To(BeTrue(),
		"an annotation-only edit must pass the predicate - the documented rotation recipe has no other way in")

	unchanged := updated.DeepCopy()
	g.Expect(specAndAnnotationsPredicate.Update(event.UpdateEvent{ObjectOld: updated, ObjectNew: unchanged})).To(BeFalse(),
		"and an edit that changes neither generation nor annotations must still be filtered out")
}

// Link 2: the reconcile that edit triggers must notice the rotation. What carries it
// is not the pipeline hash - a neutral annotation is deliberately not part of it
// (GetPipelineHash folds in only serviceName, config-optimization and
// force-configcheck) - but RelatedSecretsHash: resolveRelatedSecrets re-reads every
// USED backend through the UNCACHED reader and tokenizes their identities including
// resourceVersion, which the API server bumps on any data change. The "Pipeline has no
// changes" early return needs BOTH signals to match, so it opens for a rotated Secret
// and stays shut when nothing moved.
var _ = Describe("PipelineReconciler manual rotation pickup", func() {
	It("does not dismiss a silently rotated secret as unchanged, and stays quiet when nothing rotated", func() {
		ns := "default"

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "rotation-touch-secret", Namespace: ns},
			Data:       map[string][]byte{"key": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "rotation-touch-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: secret.Name},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.key]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })

		// The Vector's selector deliberately does NOT match this pipeline. The pipeline
		// path validates every SELECTED pipeline through a configcheck pod, and envtest
		// has no kubelet to ever run one - a matching selector would just park this spec
		// on ConfigCheckTimeout. Nothing under test here depends on the match: the early
		// return being decided happens well before the selector is consulted, and the
		// workload notification that follows it is issued to every agent unconditionally.
		// Link 3 below covers the selected-pipeline half on the workload reconciler.
		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: "rotation-touch-vector", Namespace: ns},
			Spec: v1alpha1.VectorSpec{
				Selector: &v1alpha1.VectorSelectorSpec{MatchLabels: map[string]string{"selects": "nothing-here"}},
				Agent: &v1alpha1.VectorAgent{
					VectorCommon: v1alpha1.VectorCommon{
						ConfigCheck: v1alpha1.ConfigCheck{Disabled: true},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, vector)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vector) })

		agentEvents := make(chan event.GenericEvent, 10)
		reconciler := &PipelineReconciler{
			Client:                          k8sClient,
			Scheme:                          k8sClient.Scheme(),
			Clientset:                       clientset,
			ConfigCheckTimeout:              configCheckTimeout,
			VectorAgentEventCh:              agentEvents,
			VectorAggregatorsEventCh:        make(chan event.GenericEvent, 10),
			ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 10),
			APIReader:                       k8sClient,
			SecretIndex:                     pipeline.NewSecretIndex(),
		}
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vp)}
		drain := func() {
			for {
				select {
				case <-agentEvents:
				default:
					return
				}
			}
		}
		relatedSecretsHash := func() *int64 {
			got := &v1alpha1.VectorPipeline{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
			return got.Status.RelatedSecretsHash
		}

		By("baseline reconcile: the pipeline records the token of the secret it resolved")
		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		baselineToken := relatedSecretsHash()
		Expect(baselineToken).NotTo(BeNil())

		By("reconciling again with nothing changed: the early return fires, nobody is rebuilt")
		drain()
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(agentEvents).To(BeEmpty(),
			"neither the pipeline hash nor the related-secrets token moved, so this round must take the 'no changes' early return")

		By("rotating the Secret behind the operator's back - no watch event, exactly as scoped mode leaves it")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
		secret.Data["key"] = []byte("v2")
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())

		By("touching a neutral annotation - the documented recipe - and reconciling as that edit would")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), vp)).To(Succeed())
		vp.Annotations = map[string]string{"example.com/rotate": "1"}
		Expect(k8sClient.Update(ctx, vp)).To(Succeed())

		drain()
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(relatedSecretsHash()).NotTo(Equal(baselineToken),
			"the rotated Secret's new resourceVersion must change the recorded token - that is what keeps the early return from "+
				"swallowing a rotation the watch never reported")
		Expect(agentEvents).To(HaveLen(1),
			"and the round must run to completion and tell the agents to rebuild")
	})
})

// Link 3: the workload reconcile the notification triggers must resolve values afresh.
// Nothing here is cached-reader-dependent by luck: pipelineSecretGetter is built on the
// reconciler's uncached APIReader precisely so a rotation is picked up whatever the
// manager's cache is scoped to - which is what makes the recipe work in scoped mode at
// all, where the cache may never see this Secret.
var _ = Describe("VectorReconciler secret value freshness", func() {
	It("republishes assets from the rotated secret on the very next reconcile", func() {
		ns := "default"
		vectorName := "rotation-fresh-vector"
		flatKey := "default_rotation_fresh_vp_es_key"

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "rotation-fresh-secret", Namespace: ns},
			Data:       map[string][]byte{"key": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "rotation-fresh-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: secret.Name},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.key]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		vp.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, vp, pipelineStatusBase(vp))).To(Succeed())

		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: vectorName, Namespace: ns},
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
			EventChan:          make(chan event.GenericEvent, 10),
			APIReader:          k8sClient,
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vectorName}}
		assetsKey := types.NamespacedName{Namespace: ns, Name: vectorName + "-agent-secret-assets"}

		By("baseline: the agent's assets Secret carries the current value")
		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		assets := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, assetsKey, assets)).To(Succeed())
		Expect(assets.Data).To(HaveKeyWithValue(flatKey, []byte("v1")))

		By("rotating the Secret and reconciling the workload again, with nothing else changed")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
		secret.Data["key"] = []byte("v2")
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())

		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, assetsKey, assets)).To(Succeed())
		Expect(assets.Data).To(HaveKeyWithValue(flatKey, []byte("v2")),
			"the value the agent mounts must be the rotated one - the reconcile resolves it fresh, it does not reuse what it "+
				"published last time")
	})
})
