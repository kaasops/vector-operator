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
	"fmt"
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

// failNthGetReader wraps a client.Reader and fails the Nth Get call (1-indexed) for a
// specific object key, succeeding on every other call (including calls before and
// after it). It lets a test put a real transient API error precisely between two reads
// of the same object - here, resolveRelatedSecrets' successful read and Build*Config's
// later read of the same Secret.
type failNthGetReader struct {
	client.Reader
	key        client.ObjectKey
	failOnCall int
	calls      int
}

func (r *failNthGetReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key == r.key {
		r.calls++
		if r.calls == r.failOnCall {
			return fmt.Errorf("simulated transient API error")
		}
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

// This suite proves a fixed bug: a transient GET failure on the config-BUILD path (as
// opposed to the earlier, already-fixed resolveRelatedSecrets path - see
// pipeline_controller_secret_resolve_recovery_test.go for the analogous fix there)
// must not leave a stale, *successful* RelatedSecretsHash on the pipeline.
// resolveRelatedSecrets runs first each reconcile and, on success, already sets RelatedSecretsHash before the
// eg.Go/Build*Config phase even starts. If a getter error surfaces later, during
// Build*Config's own (separate, unmemoized-across-phases) read of the same Secret, the
// stored hash from the earlier successful resolve must be cleared - otherwise a later
// reconcile with the exact same (unchanged) secret data resolves to the same hash,
// matches the stale stored one, and the pipeline is skipped forever via "Pipeline has
// no changes" - stuck invalid with no external trigger able to wake it, since the
// secret's data never actually changed.
var _ = Describe("PipelineReconciler secret-resolve failure during config build", func() {
	It("does not stay stuck when a transient GET fails only on the build-path read of an already-resolved secret", func() {
		ns := "default"
		secretName := "build-resolve-secret"
		secretData := map[string][]byte{"username": []byte("u1")}

		By("creating a ClusterVectorAggregator so the pipeline reconciles into the aggregator config-build path")
		cva := &v1alpha1.ClusterVectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "build-resolve-cva"},
			Spec: v1alpha1.ClusterVectorAggregatorSpec{
				ResourceNamespace: ns,
			},
		}
		Expect(k8sClient.Create(ctx, cva)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cva) })

		By("creating the secret the pipeline will reference")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
			Data:       secretData,
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		By("creating a ClusterVectorPipeline (aggregator role, no kubernetes_logs source) referencing it")
		cvp := &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "build-resolve-cvp"},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: secretName, Namespace: ns},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.username]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, cvp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cvp) })

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: cvp.Name}}
		secretKey := client.ObjectKey{Namespace: ns, Name: secretName}

		By("first reconcile: resolveRelatedSecrets succeeds (Get #1), the build-path read of the same secret fails (Get #2)")
		reader := &failNthGetReader{Reader: k8sClient, key: secretKey, failOnCall: 2}
		reconciler := &PipelineReconciler{
			Client:                          k8sClient,
			Scheme:                          k8sClient.Scheme(),
			Clientset:                       clientset,
			ConfigCheckTimeout:              configCheckTimeout,
			VectorAgentEventCh:              make(chan event.GenericEvent, 10),
			VectorAggregatorsEventCh:        make(chan event.GenericEvent, 10),
			ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 10),
			APIReader:                       reader,
			SecretIndex:                     pipeline.NewSecretIndex(),
		}

		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader.calls).To(Equal(2), "the test setup itself requires exactly two reads of the same secret: resolve, then build")

		got := &v1alpha1.ClusterVectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeFalse()))
		Expect(got.Status.Reason).To(HaveValue(ContainSubstring("simulated transient API error")))
		Expect(got.Status.RelatedSecretsHash).To(BeNil(),
			"the fix: a build-path secret-resolve failure must clear RelatedSecretsHash too, "+
				"not just the resolveRelatedSecrets-path failure - otherwise the next reconcile with "+
				"identical secret data matches the stale successful hash and skips itself forever")
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"a transient build-path secret-resolve failure must be retried on a timer, symmetric with the resolve-path failure")

		By("second reconcile with a healthy reader: recovery must not be skipped as \"no changes\"")
		reconciler.APIReader = k8sClient
		// envtest has no kubelet, so a configcheck pod can never actually reach
		// Succeeded/Failed - shrink the timeout so this reconcile fails fast on
		// ErrConfigcheckTimeout instead of hanging for the suite's default 60s. That
		// failure is irrelevant to what this test proves: RelatedSecretsHash is
		// recomputed (and Status.Reason moves off the stale build-path message) before
		// configCheck.Run is ever reached, so it is unaffected by how that later,
		// unrelated step ends.
		reconciler.ConfigCheckTimeout = time.Second
		result, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.RelatedSecretsHash).NotTo(BeNil(),
			"stuck-bug symptom: on unfixed code this reconcile is skipped by the stale "+
				"\"Pipeline has no changes\" branch and RelatedSecretsHash never recovers from nil")
		Expect(got.Status.Reason).NotTo(HaveValue(ContainSubstring("simulated transient API error")),
			"proof the recovery reconcile actually re-ran config build instead of being skipped")
	})
})
