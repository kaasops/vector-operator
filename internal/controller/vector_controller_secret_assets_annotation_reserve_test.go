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
	"regexp"
	"strconv"
	"strings"
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
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// preFixObjectBudget is the value config.SecretAssetsObjectBudget had before this
// suite's fix folded the annotation-limit reserve in: etcdDefaultMaxRequestBytes minus
// TotalAnnotationSizeLimitB and nothing else - no headroom at all for what the model
// cannot see once the desired object leaves this package. Kept here, independent of the
// current constant, purely so the growth pipeline below can be sized against it: an
// object whose modelled total lands at or under this value is exactly the case the
// pre-fix code would have silently admitted.
const preFixObjectBudget = 1572864 - 262144 // 1 310 720

// objectBudgetExclusionNumbers pulls the byte counts secretSizeExclusionReason embeds
// in an object-budget attribution reason (see internal/controller/secrets.go) back out,
// so this test can check them against preFixObjectBudget without depending on internals
// only reachable by calling config.DetectSecretSizeOverflow directly - the whole point
// here is to go through the real reconciler and the real writer.
var objectBudgetExclusionPattern = regexp.MustCompile(
	`adding this pipeline's (\d+) bytes of secret entries \(key names and values\) to the (\d+) bytes already committed`)

func objectBudgetExclusionNumbers(reason string) (pipelineBytes, acceptedBytes int) {
	m := objectBudgetExclusionPattern.FindStringSubmatch(reason)
	Expect(m).To(HaveLen(3), "reason must be an object-budget exclusion: %s", reason)
	pipelineBytes, err := strconv.Atoi(m[1])
	Expect(err).NotTo(HaveOccurred())
	acceptedBytes, err = strconv.Atoi(m[2])
	Expect(err).NotTo(HaveOccurred())
	return pipelineBytes, acceptedBytes
}

// This suite is the regression test for the object budget's annotation-limit reserve
// (see config.SecretAssetsObjectBudget's doc comment): a candidate that only just
// fits the model can still be dangerous to write, because CreateOrUpdateResource's
// update path (internal/utils/k8s/k8s.go) MERGES the desired object's annotations into
// whatever the EXISTING assets Secret already carries - so an annotation stamped on the
// live object by anything other than this builder survives every reconcile, unseen by
// the model, which only ever charges the freshly-built prototype (secretObjectBaseSize)
// and never re-reads the object it is about to update.
//
// Before the fix, the budget's headroom below etcd's request ceiling was exactly the
// Kubernetes annotation limit and nothing more (preFixObjectBudget above) - so a
// pipeline pool that fit under it could still combine with a real, legal foreign
// annotation already on the object into a write far closer to etcd's actual ceiling
// than the model ever priced in. The fix reserves that same annotation ceiling
// explicitly, plus wrapperReserve on top, so nothing sized to fit the new budget can
// repeat this regardless of what shows up on the live object in the meantime.
var _ = Describe("VectorReconciler secret-assets object budget annotation reserve", func() {
	It("excludes a candidate the pre-fix budget would have admitted, once a foreign annotation already sits on the live assets Secret", func() {
		ns := "annot-reserve-" + strings.Repeat("n", 26)
		vectorName := "annot-reserve-vector"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		By("creating a small pipeline and reconciling so the assets Secret is created the normal, built-in way")
		smallSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "small-creds", Namespace: ns},
			Data:       map[string][]byte{"cert": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, smallSecret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, smallSecret) })

		small := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "small", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "small-creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logsSmall": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logsSmall"], "auth": {"user": "SECRET[es.cert]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, small)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, small) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		small.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, small, pipelineStatusBase(small))).To(Succeed())

		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: vectorName, Namespace: "default"},
			Spec: v1alpha1.VectorSpec{
				Agent: &v1alpha1.VectorAgent{
					VectorCommon: v1alpha1.VectorCommon{ConfigCheck: v1alpha1.ConfigCheck{Disabled: true}},
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

		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		assetsName := vectorName + "-agent-secret-assets"
		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: assetsName}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveLen(1), "only the small pipeline's value should be staged yet")

		By("stamping a foreign annotation directly onto the live assets Secret - standing in for anything other than this builder touching it out of band")
		// Legal under Kubernetes' own 256 KiB total-annotation ceiling
		// (apivalidation.TotalAnnotationSizeLimitB) and comfortably above wrapperReserve
		// (64 KiB): large enough that the reserve this fix folds in actually matters.
		foreignAnnotationSize := 220 * 1024
		assetsSecret.Annotations = map[string]string{
			"team.example.com/foreign-note": strings.Repeat("x", foreignAnnotationSize),
		}
		Expect(k8sClient.Update(ctx, assetsSecret)).To(Succeed())

		By("creating a second, younger pipeline whose entries are sized to land strictly between the pre-fix and the current object budget")
		// Many long flat keys with tiny values - the shape measured on kind against
		// a real etcd - so the values sum stays far under corev1.MaxSecretSize and
		// it is the OBJECT budget, not the values one, that this pipeline trips.
		//
		// entryCount is calibrated (see the sibling calibration in
		// internal/config/secrets_object_size_test.go's many-long-keys test for the
		// same shape) so the resulting object total sits comfortably inside the
		// window between config.SecretAssetsObjectBudget (excludes) and
		// preFixObjectBudget (would not have excluded) - roughly 65 KiB wide, and the
		// margin here is tens of KiB on either side, well clear of exact calibration.
		const entryCount = 9060
		growthNS := strings.Repeat("n", 40)
		growthSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "growth-creds", Namespace: growthNS}}
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: growthNS}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: growthNS}}) })

		data := make(map[string][]byte, entryCount)
		var sink strings.Builder
		sink.WriteString(`{"out": {"type": "elasticsearch", "inputs": ["logsGrowth"], "auth": {`)
		for i := 0; i < entryCount; i++ {
			key := fmt.Sprintf("key-name-long-enough-to-matter-%d", i)
			data[key] = []byte("240000000000000000000000") // 24 bytes, tiny on purpose
			if i > 0 {
				sink.WriteString(", ")
			}
			fmt.Fprintf(&sink, `"opt%d": "SECRET[es.%s]"`, i, key)
		}
		sink.WriteString(`}}}`)
		growthSecret.Data = data
		Expect(k8sClient.Create(ctx, growthSecret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, growthSecret) })

		// CreationTimestamp is second-precision, so the older/younger split above
		// needs a real gap to be unambiguous.
		time.Sleep(1100 * time.Millisecond)

		growth := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("p", 30), Namespace: growthNS},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "growth-creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logsGrowth": {"type": "kubernetes_logs"}}`)},
				Sinks:   &runtime.RawExtension{Raw: []byte(sink.String())},
			},
		}
		Expect(k8sClient.Create(ctx, growth)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, growth) })
		growth.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, growth, pipelineStatusBase(growth))).To(Succeed())

		By("reconciling again: the guard must reject growth before any write is attempted, never reaching the API with an oversized object")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred(), "the guard must resolve this itself - an API rejection would surface here as a reconcile error")

		gotSmall := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(small), gotSmall)).To(Succeed())
		Expect(gotSmall.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "the small pipeline must be unaffected")

		gotGrowth := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(growth), gotGrowth)).To(Succeed())
		Expect(gotGrowth.Status.ConfigCheckResult).To(HaveValue(BeFalse()), "growth must be excluded rather than staged into an object that was never modelled safely")
		Expect(gotGrowth.Status.Reason).NotTo(BeNil())
		Expect(*gotGrowth.Status.Reason).To(HavePrefix(secretObjectSizeExclusionReasonPrefix),
			"the exclusion must be attributed to the OBJECT budget, not a flat-key collision or the values limit")

		pipelineBytes, acceptedBytes := objectBudgetExclusionNumbers(*gotGrowth.Status.Reason)
		total := pipelineBytes + acceptedBytes
		Expect(total).To(BeNumerically(">", config.SecretAssetsObjectBudget),
			"sanity: this is exactly what made the current guard exclude it")
		Expect(total).To(BeNumerically("<=", preFixObjectBudget),
			"the mutation check: this same candidate must fit under the pre-fix budget - if it did not, this test would not "+
				"actually be proving the fix closed anything, since the pre-fix code would have excluded it too. Reverting "+
				"config.SecretAssetsObjectBudget to preFixObjectBudget and rerunning this test must turn this Reconcile into "+
				"one that admits growth instead - the whole point of this suite")

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: assetsName}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveLen(1), "growth's entries must never have been staged - the small pipeline's key is still the only one")
		Expect(assetsSecret.Annotations).To(HaveKeyWithValue("team.example.com/foreign-note", strings.Repeat("x", foreignAnnotationSize)),
			"the foreign annotation must have survived this round's write untouched - createOrUpdateSecret merges rather than replaces annotations "+
				"(internal/utils/k8s/k8s.go), which is exactly the behavior the object budget's reserve exists to make safe")
	})
})
