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
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// A victim of the object budget must be told what actually happened. Reusing the
// values-limit wording would send someone whose values total a few hundred kilobytes
// looking for a 1 MiB overflow that does not exist - the mass is in the key names.
func TestSecretSizeExclusionReasonNamesTheObjectBudget(t *testing.T) {
	victim := &v1alpha1.VectorPipeline{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "team-a"}}

	objectReason := secretSizeExclusionReason(config.SecretSizeExclusion{
		Victim:              victim,
		AcceptedTotal:       1000,
		PipelineBytes:       2000,
		ObjectBudget:        true,
		AcceptedObjectBytes: 1200000,
		PipelineObjectBytes: 300000,
	}, "Vector", "observability", "agent")

	assert.True(t, strings.HasPrefix(objectReason, secretObjectSizeExclusionReasonPrefix),
		"the object-budget case needs its own frozen prefix, not the values one")
	assert.NotContains(t, objectReason, fmt.Sprint(corev1.MaxSecretSize),
		"it must not quote the values limit - that limit was not the one exceeded")
	assert.Contains(t, objectReason, fmt.Sprint(config.SecretAssetsObjectBudget))
	assert.Contains(t, objectReason, "300000", "the pipeline's own modelled contribution belongs in the reason")
	assert.Contains(t, objectReason, "1200000", "as does what older pipelines already committed")
	assert.Contains(t, objectReason, "Vector observability/agent")
	assert.True(t, isSecretAttributionReason(objectReason),
		"an object-budget victim is reconsidered on later rounds exactly like the other attribution classes")

	valuesReason := secretSizeExclusionReason(config.SecretSizeExclusion{
		Victim:        victim,
		AcceptedTotal: 900000,
		PipelineBytes: 200000,
	}, "Vector", "observability", "agent")
	assert.True(t, strings.HasPrefix(valuesReason, secretSizeExclusionReasonPrefix))
	assert.Contains(t, valuesReason, fmt.Sprint(corev1.MaxSecretSize),
		"the values case keeps quoting the limit the API server itself names")
}

// The end-to-end half of the same guarantee: a workload whose pipelines fit by values
// but not as an object must be caught by the pre-pass, with the victim carrying the
// object-budget reason - and the oversized Secret must never be attempted, so the
// operator never has to explain a raw `etcdserver: request is too large` after the
// fact.
var _ = Describe("VectorReconciler secret-assets object budget", func() {
	It("excludes the younger pipeline before the write instead of failing the API call", func() {
		// Long namespace and pipeline names on purpose: the flat key is
		// namespace+pipeline+alias+key, so realistic per-pipeline naming is what puts
		// the mass into key names rather than values.
		ns := "object-budget-" + strings.Repeat("n", 26)
		vectorName := "object-budget-vector"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		// Many long keys, tiny values: the shape measured on kind against a real etcd.
		data := map[string][]byte{}
		for i := 0; i < 5000; i++ {
			data[fmt.Sprintf("key-name-long-enough-to-matter-%d", i)] = []byte("240000000000000000000000")
		}
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns}, Data: data}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		sink := func() *runtime.RawExtension {
			var b strings.Builder
			b.WriteString(`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {`)
			for i := 0; i < 5000; i++ {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, `"opt%d": "SECRET[es.key-name-long-enough-to-matter-%d]"`, i, i)
			}
			b.WriteString(`}}}`)
			return &runtime.RawExtension{Raw: []byte(b.String())}
		}

		mk := func(name string) *v1alpha1.VectorPipeline {
			vp := &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: v1alpha1.VectorPipelineSpec{
					Secret: map[string]v1alpha1.PipelineSecretBackend{
						"es": {Type: "kubernetes_secret", Name: secret.Name},
					},
					Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
					Sinks:   sink(),
				},
			}
			Expect(k8sClient.Create(ctx, vp)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })
			agentRole := v1alpha1.VectorPipelineRoleAgent
			vp.SetRole(&agentRole)
			Expect(pipeline.SetSuccessStatus(ctx, k8sClient, vp, pipelineStatusBase(vp))).To(Succeed())
			return vp
		}

		older := mk("aaa-" + strings.Repeat("o", 26))
		younger := mk("zzz-" + strings.Repeat("y", 26))

		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: vectorName, Namespace: ns},
			Spec: v1alpha1.VectorSpec{
				Agent: &v1alpha1.VectorAgent{
					VectorCommon: v1alpha1.VectorCommon{ConfigCheck: v1alpha1.ConfigCheck{Disabled: true}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, vector)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vector) })

		_, err := (&VectorReconciler{
			Client:             k8sClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 10),
			APIReader:          k8sClient,
		}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: vectorName}})
		Expect(err).NotTo(HaveOccurred(),
			"the guard must resolve this before any write - an API rejection would surface here as a reconcile error")

		gotYounger := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeFalse()))
		Expect(gotYounger.Status.Reason).NotTo(BeNil())
		Expect(*gotYounger.Status.Reason).To(HavePrefix(secretObjectSizeExclusionReasonPrefix),
			"the victim must be told its keys did not fit the object, not that its values broke a 1 MiB limit")

		gotOlder := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(older), gotOlder)).To(Succeed())
		Expect(gotOlder.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "the older pipeline keeps the workload, as with every other budget")

		assets := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: vectorName + "-agent-secret-assets"}, assets)
		Expect(client.IgnoreNotFound(err)).To(Succeed())
		if err == nil {
			Expect(config.SecretAssetsObjectBudget).To(BeNumerically(">=", (&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: assets.Namespace, Name: assets.Name},
				Data:       assets.Data,
			}).Size()), "whatever was actually written must sit inside the budget")
		}
		Expect(api_errors.IsRequestEntityTooLargeError(err)).To(BeFalse())
	})
})

// testAssetsPrototype is the stand-in the unit specs pass where a workload controller
// would hand over its builder's own assets Secret.
func testAssetsPrototype() *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "v-agent-secret-assets"}}
}
