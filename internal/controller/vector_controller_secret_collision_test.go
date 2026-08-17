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

// This suite proves the collision-attribution behavior: a
// secret flat-key collision between two pipelines on the same workload must fail
// only the YOUNGER pipeline (by CreationTimestamp) and let the workload keep running
// on the older pipeline's references, instead of freezing the whole workload the way
// a hard Build*Config error does (see TestSecretFlatKeyCollisionAcrossPipelines in
// internal/config, which pins that older, still-correct behavior for any direct
// Build*Config caller that does not pre-filter with config.DetectSecretCollisions -
// resolveWorkloadPipelines, exercised here, is that pre-filter for the workload
// reconcilers). It also proves recovery: once the collision is gone (the older
// pipeline deleted), the younger pipeline must come back to a valid status and its
// references must reappear in the built config on the very next reconcile - the trap
// this class of bug repeatedly falls into (see pipeline_controller_secret_resolve_recovery_test.go
// and pipeline_controller_secret_build_resolve_test.go for two other instances) is a
// pipeline that goes invalid and then has nothing left to ever reconcile it again.
//
// Individual per-pipeline validation (PipelineReconciler's own configcheck) needs a
// real pod to reach Succeeded/Failed, which envtest's apiserver-without-kubelet can
// never provide (see pipeline_controller_secret_build_resolve_test.go's own comment
// on the same limitation) - so this suite
// seeds both pipelines directly as already individually valid (pipeline.SetRole +
// pipeline.SetSuccessStatus) and drives only VectorReconciler, which is where all of
// resolveWorkloadPipelines' new logic lives; PipelineReconciler is not involved.
var _ = Describe("VectorReconciler secret flat-key collision attribution", func() {
	It("fails only the younger pipeline, keeps the workload on the older one, and recovers once the collision is gone", func() {
		olderNS := "collision-team"
		youngerNS := "collision-team-a"

		By("creating the two namespaces the collision fixture needs")
		for _, ns := range []string{olderNS, youngerNS} {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		}
		DeferCleanup(func() {
			for _, ns := range []string{olderNS, youngerNS} {
				_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
			}
		})

		By("creating the secrets each pipeline references")
		olderSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: olderNS},
			Data:       map[string][]byte{"username": []byte("u-older")},
		}
		youngerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: youngerNS},
			Data:       map[string][]byte{"username": []byte("u-younger")},
		}
		Expect(k8sClient.Create(ctx, olderSecret)).To(Succeed())
		Expect(k8sClient.Create(ctx, youngerSecret)).To(Succeed())

		// namespace "collision-team" + pipeline "a-x" and namespace "collision-team-a"
		// + pipeline "x" both sanitize to the same flat key: generateName joins
		// namespace and name with "-", and flatKey folds every "-" to "_" - see
		// TestSecretFlatKeyCollisionAcrossPipelines in internal/config for the same
		// fixture used directly against Build*Config.
		newPipeline := func(ns, name, sourceKey string) *v1alpha1.VectorPipeline {
			return &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: v1alpha1.VectorPipelineSpec{
					Secret: map[string]v1alpha1.PipelineSecretBackend{
						"es": {Type: "kubernetes_secret", Name: "creds"},
					},
					Sources: &runtime.RawExtension{Raw: []byte(`{"` + sourceKey + `": {"type": "kubernetes_logs"}}`)},
					Sinks: &runtime.RawExtension{Raw: []byte(
						`{"out": {"type": "elasticsearch", "inputs": ["` + sourceKey + `"], "auth": {"user": "SECRET[es.username]"}}}`,
					)},
				},
			}
		}

		By("creating the older pipeline and seeding it individually valid (agent role) - real per-pipeline " +
			"configcheck cannot run in envtest, see the suite doc comment above")
		older := newPipeline(olderNS, "a-x", "srcOld")
		Expect(k8sClient.Create(ctx, older)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, older) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		older.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, older, pipelineStatusBase(older))).To(Succeed())

		// CreationTimestamp is second-precision in the Kubernetes API, so the two
		// pipelines need a real gap between them for attribution ("oldest survives")
		// to be unambiguous.
		time.Sleep(1100 * time.Millisecond)

		By("creating the younger pipeline the same way")
		younger := newPipeline(youngerNS, "x", "srcNew")
		Expect(k8sClient.Create(ctx, younger)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, younger) })
		younger.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, younger, pipelineStatusBase(younger))).To(Succeed())

		By("creating the Vector agent workload with configcheck disabled (no kubelet in envtest)")
		vectorName := "collision-vector"
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

		By("first reconcile: the collision must fail only the younger pipeline")
		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		gotOlder := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(older), gotOlder)).To(Succeed())
		Expect(gotOlder.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "the older pipeline must stay valid")
		Expect(gotOlder.Status.Reason).To(BeNil())

		gotYounger := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeFalse()), "the younger pipeline must be excluded and failed")
		Expect(gotYounger.Status.Reason).To(HaveValue(ContainSubstring("collision_team_a_x_es_username")),
			"the reason must name the flat key")
		Expect(gotYounger.Status.Reason).To(HaveValue(ContainSubstring("collision-team/a-x")),
			"the reason must name the surviving pipeline it collided with")
		Expect(gotYounger.Status.Reason).To(HaveValue(ContainSubstring("Vector")),
			"the reason must name the workload whose merged build discovered the collision")
		Expect(gotYounger.Status.RelatedSecretsHash).To(BeNil(),
			"clearing it is what lets the pipeline recover once reconsidered - same idiom as the resolve-recovery/build-resolve fixes elsewhere")

		configSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		builtConfig := string(configSecret.Data["agent.json"])
		Expect(builtConfig).To(ContainSubstring(`"collision-team-a-x-srcOld"`), "the workload must keep the older pipeline's source")
		Expect(builtConfig).NotTo(ContainSubstring(`"collision-team-a-x-srcNew"`), "the younger pipeline's source must be excluded")

		By("resolving the collision by deleting the older pipeline")
		Expect(k8sClient.Delete(ctx, older)).To(Succeed())

		By("second reconcile: the younger pipeline must recover and its source must reappear in the built config")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeTrue()),
			"stuck-bug symptom: on unfixed code nothing ever reconsiders a collision-failed pipeline once its own spec/annotations never change")
		Expect(gotYounger.Status.Reason).To(BeNil())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		builtConfig = string(configSecret.Data["agent.json"])
		Expect(builtConfig).To(ContainSubstring(`"collision-team-a-x-srcNew"`), "the recovered pipeline's source must be back in the build")
	})

	// DetectSecretCollisions never reads a Secret's actual Data,
	// only its identity - so a retry candidate can clear collision detection while
	// still being individually broken (its declared Secret key vanished in the
	// meantime). Reinstating it (SetSuccessStatus) before Build*Config has actually
	// confirmed the merged config works would write a green status that lies: the
	// workload build fails right after, and the pipeline is left falsely valid,
	// possibly stuck that way until its own reconcile runs again.
	It("does not reinstate a retry candidate whose build still fails, and reinstates it once the build succeeds", func() {
		olderNS := "collision-team-bf"
		youngerNS := "collision-team-bf-a"

		By("creating the two namespaces the collision fixture needs")
		for _, ns := range []string{olderNS, youngerNS} {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		}
		DeferCleanup(func() {
			for _, ns := range []string{olderNS, youngerNS} {
				_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
			}
		})

		By("creating the secrets each pipeline references")
		olderSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: olderNS},
			Data:       map[string][]byte{"username": []byte("u-older")},
		}
		youngerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: youngerNS},
			Data:       map[string][]byte{"username": []byte("u-younger")},
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
						`{"out": {"type": "elasticsearch", "inputs": ["` + sourceKey + `"], "auth": {"user": "SECRET[es.username]"}}}`,
					)},
				},
			}
		}

		older := newPipeline(olderNS, "a-x", "srcOldBF")
		Expect(k8sClient.Create(ctx, older)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, older) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		older.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, older, pipelineStatusBase(older))).To(Succeed())

		time.Sleep(1100 * time.Millisecond)

		younger := newPipeline(youngerNS, "x", "srcNewBF")
		Expect(k8sClient.Create(ctx, younger)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, younger) })
		younger.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, younger, pipelineStatusBase(younger))).To(Succeed())

		vectorName := "collision-vector-bf"
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

		By("first reconcile: the collision fails only the younger pipeline")
		_, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		gotYounger := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeFalse()))

		By("resolving the collision (delete older) but ALSO breaking the younger pipeline's own build " +
			"(delete the Secret key it references) - DetectSecretCollisions cannot see this, only Build*Config can")
		Expect(k8sClient.Delete(ctx, older)).To(Succeed())
		youngerSecret.Data = map[string][]byte{} // "username" key gone
		Expect(k8sClient.Update(ctx, youngerSecret)).To(Succeed())

		By("second reconcile: must NOT reinstate the younger pipeline - its build still fails")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeFalse()),
			"must stay failed - SetSuccessStatus must not run before Build*Config confirms the config actually works")

		gotVector := &v1alpha1.Vector{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		Expect(gotVector.Status.ConfigCheckResult).To(HaveValue(BeFalse()), "the workload build must report the failure")

		By("restoring the Secret key")
		youngerSecret.Data = map[string][]byte{"username": []byte("u-younger-2")}
		Expect(k8sClient.Update(ctx, youngerSecret)).To(Succeed())

		By("third reconcile: the younger pipeline must now reinstate and its source must reappear")
		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(younger), gotYounger)).To(Succeed())
		Expect(gotYounger.Status.ConfigCheckResult).To(HaveValue(BeTrue()))
		Expect(gotYounger.Status.Reason).To(BeNil())

		configSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		builtConfig := string(configSecret.Data["agent.json"])
		Expect(builtConfig).To(ContainSubstring(`"collision-team-bf-a-x-srcNewBF"`), "the recovered pipeline's source must be back in the build")
	})
})
