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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// This suite pins the write-order invariant itself with real fault injection against
// the envtest API server, not just an assertion on the final state - a regression
// that swapped ensureVectorAgentSecretAssets and ensureVectorAgentConfig back to
// config-before-assets would compile and pass every other test in this package,
// since with no injected failure both objects end up written either way. Only
// intercepting one specific write and observing what the OTHER object looks like on
// the server afterwards actually distinguishes the two orders.
var _ = Describe("VectorReconciler secret-assets write-order fault injection", func() {
	setupFixture := func(ns, vectorName string) (pl *v1alpha1.VectorPipeline, vector *v1alpha1.Vector) {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"cert": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		pl = &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.cert]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, pl)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		vector = &v1alpha1.Vector{
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
		return pl, vector
	}

	It("has already published the assets Secret when an injected config-write failure aborts the reconcile", func() {
		ns := "order-fail-config"
		vectorName := "order-fail-config-vector"
		_, vector := setupFixture(ns, vectorName)

		configSecretName := vectorName + "-agent"
		injected := errors.New("injected config write failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if s, ok := obj.(*corev1.Secret); ok && s.Name == configSecretName {
					return injected
				}
				return c.Create(ctx, obj, opts...)
			},
		})

		reconciler := &VectorReconciler{
			Client:             faultyClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}

		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected config-write failure must surface as a Reconcile error")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed(),
			"the assets Secret must already be on the API server - if a future regression swapped the write order back "+
				"(config before assets), the config write would have been attempted (and failed) before assets was ever written")
		Expect(assetsSecret.Data).To(HaveKey("order_fail_config_app_es_cert"))

		configSecret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: configSecretName}, configSecret)
		Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the config Secret must not exist - its create was the one that failed")

		gotVector := &v1alpha1.Vector{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
	})

	It("never attempts the config write at all when the assets write itself fails", func() {
		ns := "order-fail-assets"
		vectorName := "order-fail-assets-vector"
		setupFixture(ns, vectorName)

		assetsSecretName := vectorName + "-agent-secret-assets"
		injected := errors.New("injected assets write failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if s, ok := obj.(*corev1.Secret); ok && s.Name == assetsSecretName {
					return injected
				}
				return c.Create(ctx, obj, opts...)
			},
		})

		reconciler := &VectorReconciler{
			Client:             faultyClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}

		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred())

		configSecret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)
		Expect(api_errors.IsNotFound(err)).To(BeTrue(),
			"the config Secret must not exist - a config referencing a key the (failed-to-write) assets Secret does not "+
				"have would be exactly the crash-loop-on-restart bug this ordering exists to prevent")
	})
})
