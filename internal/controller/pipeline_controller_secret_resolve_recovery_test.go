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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// This suite proves a fixed bug: a pipeline whose declared secret briefly fails to
// resolve (e.g. the Secret was deleted) must not get stuck reporting that stale
// failure forever once the Secret comes back with the SAME data. Before the fix,
// SetFailedStatus on the resolve-error path wrote LastAppliedPipelineHash for the
// (unchanged) spec, so the recovery reconcile saw notChanged==true AND the old
// RelatedSecretsHash still equal to the freshly resolved one, and hit the "Pipeline
// has no changes" skip branch in Reconcile - leaving status.Reason pinned to the
// long-gone "failed to get secret" error even though the secret resolves fine again.
var _ = Describe("PipelineReconciler secret-resolve failure recovery", func() {
	It("does not stay stuck on a stale secret-resolve failure once the secret is recreated with identical data", func() {
		ns := "default"
		secretName := "recovery-secret"
		secretData := map[string][]byte{"key": []byte("same-value")}

		By("creating a ClusterVectorAggregator so Reconcile does not early-exit before touching secret resolution")
		cva := &v1alpha1.ClusterVectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "recovery-test-cva"}}
		Expect(k8sClient.Create(ctx, cva)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cva) })

		By("creating the secret the pipeline will reference")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data:       secretData,
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("creating a VectorPipeline whose sink references it - no sources on purpose, so the pipeline's own " +
			"role check ('sources list is empty') is the stand-in for 'the reconcile got past secret " +
			"resolution and did real work', distinguishable from a secret-resolve failure reason")
		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "recovery-test-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"s1": {Type: "kubernetes_secret", Name: secretName},
				},
				// A SECRET[] reference is required since resolution went used-refs-only:
				// a declared-but-unused backend is deliberately never resolved.
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["src"], "auth": {"user": "SECRET[s1.key]"}}}`,
				)},
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

		By("initial reconcile: secret resolves, RelatedSecretsHash is populated")
		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		got := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
		Expect(got.Status.RelatedSecretsHash).NotTo(BeNil())
		Expect(got.Status.Reason).To(HaveValue(ContainSubstring("sources list is empty")),
			"with no secret-resolve error, the reconcile must have proceeded to its own role check")
		initialHash := *got.Status.RelatedSecretsHash

		By("deleting the secret and reconciling: the pipeline must fail on secret resolution")
		Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(relatedSecretsResolveRetryDelay),
			"a transient resolve failure (as opposed to a permanent shape violation) must be retried on the fixed delay")

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
		Expect(got.Status.Reason).To(HaveValue(ContainSubstring("failed to get secret")))
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeFalse()))
		Expect(got.Status.RelatedSecretsHash).To(BeNil(),
			"the fix: the resolve-error path must clear RelatedSecretsHash so a later "+
				"recovery reconcile with identical secret data cannot match it and skip itself")

		By("recreating the secret with IDENTICAL data and reconciling again: the recovery reconcile must not be skipped as \"no changes\"")
		recreated := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data:       secretData,
		}
		Expect(k8sClient.Create(ctx, recreated)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, recreated) })

		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), got)).To(Succeed())
		Expect(got.Status.Reason).NotTo(HaveValue(ContainSubstring("failed to get secret")),
			"stuck-bug symptom: on unfixed code this reconcile is skipped by the stale "+
				"'Pipeline has no changes' branch and the reason never moves past the long-gone resolve failure")
		Expect(got.Status.Reason).To(HaveValue(ContainSubstring("sources list is empty")),
			"proof the recovery reconcile actually proceeded past secret resolution and re-ran the role check")
		Expect(got.Status.RelatedSecretsHash).NotTo(BeNil())
		// The hash is an identity token (UID/resourceVersion), not a data hash: a
		// delete-then-recreate is a new object and MUST read as a change even with
		// byte-identical data - that property is itself part of the protection this
		// suite pins, so equality with the pre-delete hash would now be the bug.
		Expect(*got.Status.RelatedSecretsHash).NotTo(Equal(initialHash),
			"recreate must produce a different identity token (new UID) even with identical data")
	})
})
