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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// The poll interval must be spread across pipelines and stable for each one - see
// secretRotationPollInterval's own doc comment for why a fixed interval would turn an
// operator restart into a synchronized re-resolve of every pipeline in the cluster.
func TestSecretRotationPollInterval(t *testing.T) {
	g := NewWithT(t)

	key := types.NamespacedName{Namespace: "team-a", Name: "app"}
	g.Expect(secretRotationPollInterval(key)).To(Equal(secretRotationPollInterval(key)),
		"the interval must be a pure function of identity - a value that drifts between reconciles could be pushed out forever")

	distinct := map[time.Duration]struct{}{}
	lowest, highest := secretRotationPollBase+secretRotationPollSpread, secretRotationPollBase
	for i := 0; i < 200; i++ {
		d := secretRotationPollInterval(types.NamespacedName{Namespace: "team-a", Name: fmt.Sprintf("app-%d", i)})
		g.Expect(d).To(BeNumerically(">=", secretRotationPollBase))
		g.Expect(d).To(BeNumerically("<", secretRotationPollBase+secretRotationPollSpread))
		distinct[d] = struct{}{}
		if d < lowest {
			lowest = d
		}
		if d > highest {
			highest = d
		}
	}
	g.Expect(len(distinct)).To(BeNumerically(">", 50))

	// Counting distinct values is not enough, and this assertion is here because that
	// was learned the hard way: a reduction that degenerates (hashing modulo a spread
	// expressed in nanoseconds never wraps a uint32) still yields 200 distinct values
	// while confining every one of them to a ~4-second window - jitter that looks fine
	// in a distinctness check and leaves the herd essentially intact. What has to hold
	// is that the samples actually COVER the window.
	g.Expect(highest-lowest).To(BeNumerically(">", 30*time.Second),
		"the sampled intervals must span a real share of the spread, not merely differ from one another")

	// The namespace/name separator: without it these two collapse onto one phase.
	g.Expect(secretRotationPollInterval(types.NamespacedName{Namespace: "ab", Name: "c"})).
		NotTo(Equal(secretRotationPollInterval(types.NamespacedName{Namespace: "a", Name: "bc"})))
}

// This suite covers the scoped-mode rotation poll: when the Secret watch cannot be
// trusted to report a rotation (--watch-namespace / --watch-name narrow the Secret
// informer), pipelines that actually reference Secrets re-check themselves on a timer.
// The specs pin who gets a poll, who does not, and that the poll survives the paths
// where it matters.
var _ = Describe("PipelineReconciler scoped-mode rotation poll", func() {
	makeSecret := func(name, ns string) *corev1.Secret {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string][]byte{"key": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, s)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, s) })
		return s
	}

	// No Vector/aggregator exists in these specs on purpose: the reconcile then returns
	// at its "Vectors not found" exit, which is one of the non-terminal outcomes the
	// poll has to survive, and it keeps the specs off the configcheck path that envtest
	// (no kubelet) cannot complete.
	newReconciler := func(poll bool) *PipelineReconciler {
		return &PipelineReconciler{
			Client:                          k8sClient,
			Scheme:                          k8sClient.Scheme(),
			Clientset:                       clientset,
			ConfigCheckTimeout:              configCheckTimeout,
			VectorAgentEventCh:              make(chan event.GenericEvent, 10),
			VectorAggregatorsEventCh:        make(chan event.GenericEvent, 10),
			ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 10),
			APIReader:                       k8sClient,
			SecretIndex:                     pipeline.NewSecretIndex(),
			PollSecretRotation:              poll,
		}
	}

	It("arms a poll for a VectorPipeline that references a secret", func() {
		ns := "default"
		secret := makeSecret("poll-vp-secret", ns)

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-vp", Namespace: ns},
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

		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vp)}
		result, err := newReconciler(true).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(secretRotationPollInterval(req.NamespacedName)))

		By("and none at all when the operator is not running scoped")
		result, err = newReconciler(false).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(),
			"in default mode the Secret watch reports rotations on its own, so a timer would be API load for nothing")
	})

	// Which way the composition goes is the whole point of it, and nothing pinned the
	// direction: a body that set its own short retry (here the 10-second re-check for a
	// Secret that does not exist yet) must keep it, because the poll is a floor under
	// such retries, not a replacement that would stretch a 10-second recovery into
	// minutes. Swapping the comparison survives every other spec in this file.
	It("keeps the body's own shorter retry instead of stretching it to the poll interval", func() {
		ns := "default"

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-missing-secret-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "poll-missing-secret-does-not-exist"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.key]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })

		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vp)}
		result, err := newReconciler(true).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(result.RequeueAfter).To(Equal(relatedSecretsResolveRetryDelay),
			"the unresolvable secret's own retry is seconds away and the poll is minutes away - the sooner wakeup has to win")
		Expect(result.RequeueAfter).To(BeNumerically("<", secretRotationPollInterval(req.NamespacedName)))
	})

	It("arms a poll for a ClusterVectorPipeline too", func() {
		ns := "default"
		secret := makeSecret("poll-cvp-secret", ns)

		cvp := &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-cvp"},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: secret.Name, Namespace: ns},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.key]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, cvp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cvp) })

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: cvp.Name}}
		result, err := newReconciler(true).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(secretRotationPollInterval(req.NamespacedName)))
	})

	It("leaves a pipeline with no used secret backend alone", func() {
		ns := "default"
		secret := makeSecret("poll-unused-secret", ns)

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-unused-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				// Declared but never referenced by a SECRET[] placeholder - the same
				// used-refs-only rule resolveRelatedSecrets applies: nothing is read for
				// it, so there is nothing to poll for either.
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: secret.Name},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"]}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })

		result, err := newReconciler(true).Reconcile(context.Background(),
			reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vp)})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		By("and so is a pipeline with no spec.secret at all")
		plain := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-plain-vp", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "blackhole", "inputs": ["logs"]}}`)},
			},
		}
		Expect(k8sClient.Create(ctx, plain)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, plain) })

		result, err = newReconciler(true).Reconcile(context.Background(),
			reconcile.Request{NamespacedName: client.ObjectKeyFromObject(plain)})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
	})

	It("keeps the poll armed on a converged round without doing any work", func() {
		ns := "poll-noop"
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := makeSecret("poll-noop-secret", ns)

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-noop-vp", Namespace: ns},
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

		// A Vector that DOES select this pipeline, so that a round which did not take
		// the early return would spawn a configcheck pod - that is what makes the
		// "no configcheck, no writes" assertion below mean something.
		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-noop-vector", Namespace: ns},
			Spec:       v1alpha1.VectorSpec{Agent: &v1alpha1.VectorAgent{}},
		}
		Expect(k8sClient.Create(ctx, vector)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vector) })

		By("priming the pipeline into the converged state the operator would have left it in")
		used, err := config.UsedSecretBackends(vp)
		Expect(err).NotTo(HaveOccurred())
		_, token, err := resolveRelatedSecrets(ctx, k8sClient, vp, used)
		Expect(err).NotTo(HaveOccurred())
		agentRole := v1alpha1.VectorPipelineRoleAgent
		vp.SetRole(&agentRole)
		vp.SetRelatedSecretsHash(token)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, vp, pipelineStatusBase(vp))).To(Succeed())

		reconciler := newReconciler(true)
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vp)}
		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(result.RequeueAfter).To(Equal(secretRotationPollInterval(req.NamespacedName)),
			"a converged pipeline is exactly the one nothing else will ever wake, so it is the one that most needs the next poll")
		Expect(reconciler.VectorAgentEventCh).To(BeEmpty(), "nothing changed, so no workload may be asked to rebuild")

		secrets := &corev1.SecretList{}
		Expect(k8sClient.List(ctx, secrets, client.InNamespace(ns))).To(Succeed())
		for _, s := range secrets.Items {
			Expect(strings.HasPrefix(s.Name, "configcheck")).To(BeFalse(),
				"the converged round must take the early return, well before any config is built or validated")
		}
	})
})

// The scoped cache is the whole reason this feature exists, so one spec runs against a
// real one: a manager whose Secret informer is confined to a single namespace, with the
// pipeline's Secret living somewhere else entirely. Values must still resolve, and a
// rotation must still register, because resolution goes through the uncached APIReader
// rather than that cache.
var _ = Describe("PipelineReconciler with a namespace-scoped cache", func() {
	It("resolves and re-resolves a Secret the scoped cache cannot even see", func() {
		scoped := "poll-scoped-watched"
		outside := "poll-scoped-outside"
		for _, ns := range []string{scoped, outside} {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: outside},
			Data:       map[string][]byte{"key": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, secret) })

		// ClusterVectorPipeline, because only it can reference a Secret in another
		// namespace - which is precisely the case --watch-namespace leaves uncovered.
		cvp := &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-scoped-cvp"},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: secret.Name, Namespace: outside},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.key]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, cvp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cvp) })

		// A workload has to exist for the reconcile to get past its "Vectors not found"
		// exit and actually record what it resolved. An AGENT is used deliberately: this
		// pipeline is aggregator-role (its source is `vector`), so the agent branch -
		// the one that would spawn a configcheck pod envtest cannot run - is never
		// entered, while the resolve/record path under test runs in full.
		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: "poll-scoped-vector", Namespace: scoped},
			Spec:       v1alpha1.VectorSpec{Agent: &v1alpha1.VectorAgent{}},
		}
		Expect(k8sClient.Create(ctx, vector)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vector) })

		By("starting a manager whose Secret cache is confined to one namespace, as --watch-namespace does")
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  k8sClient.Scheme(),
			Metrics: metricsserver.Options{BindAddress: "0"},
			Cache: cache.Options{
				ByObject: map[client.Object]cache.ByObject{
					&corev1.Secret{}: {Namespaces: map[string]cache.Config{scoped: {}}},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		mgrCtx, mgrCancel := context.WithCancel(ctx)
		DeferCleanup(mgrCancel)
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		Expect(mgr.GetCache().WaitForCacheSync(mgrCtx)).To(BeTrue())

		By("confirming the scoping is real: the cached client cannot read that Secret at all")
		err = mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(secret), &corev1.Secret{})
		Expect(err).To(HaveOccurred(),
			"if the cached client could read it, this spec would prove nothing about where resolution actually reads from")

		reconciler := &PipelineReconciler{
			Client:                          mgr.GetClient(),
			Scheme:                          mgr.GetScheme(),
			Clientset:                       clientset,
			ConfigCheckTimeout:              configCheckTimeout,
			VectorAgentEventCh:              make(chan event.GenericEvent, 10),
			VectorAggregatorsEventCh:        make(chan event.GenericEvent, 10),
			ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 10),
			APIReader:                       mgr.GetAPIReader(),
			SecretIndex:                     pipeline.NewSecretIndex(),
			PollSecretRotation:              true,
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: cvp.Name}}

		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(secretRotationPollInterval(req.NamespacedName)))

		got := &v1alpha1.ClusterVectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.RelatedSecretsHash).NotTo(BeNil(),
			"the value resolved despite the cache never having seen that namespace - reads go through the APIReader")
		firstToken := *got.Status.RelatedSecretsHash

		By("rotating the out-of-scope Secret: no watch can report this, only the next poll's read")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
		secret.Data["key"] = []byte("v2")
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())

		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cvp), got)).To(Succeed())
		Expect(got.Status.RelatedSecretsHash).NotTo(BeNil())
		Expect(*got.Status.RelatedSecretsHash).NotTo(Equal(firstToken),
			"the poll round must notice the rotation the scoped watch structurally cannot")
	})
})
