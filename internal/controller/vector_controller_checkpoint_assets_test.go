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
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// With checkpoint migration enabled the agent keeps BOTH config Secret variants up
// to date so pods not yet rolled after a mode switch stay functional (see
// createOrUpdateVector). The secret-assets Secret must mirror that lifecycle: while
// migration is on, both name variants exist and carry the same resolved data (the
// old-generation pods keep receiving rotations mid-rollout and a replacement pod on
// the old template can still mount its Secret); once migration is off, the standby
// variant is removed together with the standby config Secret. This suite drives the
// full VectorReconciler like the collision suite does (pipeline seeded as
// individually valid - envtest has no kubelet for a real configcheck pod).
var _ = Describe("VectorReconciler secret-assets lifecycle under checkpoint migration", func() {
	It("maintains both assets variants while migration is on and drops the standby once it is off", func() {
		nsName := "cmassets-team"

		By("creating the namespace, the referenced Secret, and a valid agent pipeline")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}})).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}})
		})

		creds := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: nsName},
			Data:       map[string][]byte{"username": []byte("u1")},
		}
		Expect(k8sClient.Create(ctx, creds)).To(Succeed())

		vp := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipe", Namespace: nsName},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"src": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["src"], "auth": {"user": "SECRET[es.username]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		vp.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, vp, pipelineStatusBase(vp))).To(Succeed())

		By("creating the Vector agent workload with configcheck disabled (no kubelet in envtest)")
		vectorName := "cmassets-vector"
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

		newReconciler := func(enableMigration bool) *VectorReconciler {
			return &VectorReconciler{
				Client:                    k8sClient,
				Scheme:                    k8sClient.Scheme(),
				Clientset:                 clientset,
				ConfigCheckTimeout:        configCheckTimeout,
				DiscoveryClient:           clientset.DiscoveryClient,
				EventChan:                 make(chan event.GenericEvent, 1),
				APIReader:                 k8sClient,
				EnableCheckpointMigration: enableMigration,
				CheckpointMergerImage:     "example.com/checkpoint-merger:test",
			}
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}
		activeAssets := types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}
		standbyAssets := types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-opt-secret-assets"}

		By("reconciling with checkpoint migration on: both assets variants must exist with identical data")
		_, err := newReconciler(true).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		active := &corev1.Secret{}
		standby := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, activeAssets, active)).To(Succeed())
		Expect(k8sClient.Get(ctx, standbyAssets, standby)).To(Succeed())
		Expect(active.Data).To(Equal(standby.Data), "both variants must carry the same resolved data")
		Expect(active.Data).To(HaveLen(1))
		for _, v := range active.Data {
			Expect(v).To(Equal([]byte("u1")))
		}

		By("rotating the referenced Secret and reconciling again: both variants must pick up the new value")
		creds.Data["username"] = []byte("u2")
		Expect(k8sClient.Update(ctx, creds)).To(Succeed())
		_, err = newReconciler(true).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, activeAssets, active)).To(Succeed())
		Expect(k8sClient.Get(ctx, standbyAssets, standby)).To(Succeed())
		for _, secret := range []*corev1.Secret{active, standby} {
			for _, v := range secret.Data {
				Expect(v).To(Equal([]byte("u2")), "no variant may be left stale after a rotation")
			}
		}

		By("reconciling with checkpoint migration off: the standby variant must be removed, the active one kept")
		_, err = newReconciler(false).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		Expect(api_errors.IsNotFound(k8sClient.Get(ctx, standbyAssets, &corev1.Secret{}))).To(BeTrue(),
			"the standby assets Secret must be deleted together with the standby config Secret")
		Expect(k8sClient.Get(ctx, activeAssets, active)).To(Succeed())
		for _, v := range active.Data {
			Expect(v).To(Equal([]byte("u2")))
		}
	})
})
