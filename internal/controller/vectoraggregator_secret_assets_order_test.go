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

	appsv1 "k8s.io/api/apps/v1"
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

// This suite copies vector_controller_secret_assets_write_order_test.go and
// vector_controller_secret_assets_template_order_test.go onto VectorAggregator:
// the agent's write ordering (assets -> workload/template -> config, per
// EnsureVectorAggregator's own doc
// comment) had real fault-injection pinning it, but the aggregator's identical
// ordering only held by code symmetry - a regression that swapped the calls back
// would compile and pass every other test in this package, since with no injected
// failure the order never becomes observable.
var _ = Describe("VectorAggregatorReconciler secret-assets write-order fault injection", func() {
	setupFixture := func(ns, aggName string) (vp *v1alpha1.VectorPipeline, agg *v1alpha1.VectorAggregator) {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"cert": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		vp = &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.cert]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, vp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vp) })
		aggregatorRole := v1alpha1.VectorPipelineRoleAggregator
		vp.SetRole(&aggregatorRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, vp, pipelineStatusBase(vp))).To(Succeed())

		agg = &v1alpha1.VectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: aggName, Namespace: ns},
			Spec: v1alpha1.VectorAggregatorSpec{
				VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{
					VectorCommon: v1alpha1.VectorCommon{
						ConfigCheck: v1alpha1.ConfigCheck{Disabled: true},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agg)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agg) })
		return vp, agg
	}

	newReconciler := func(c client.Client) *VectorAggregatorReconciler {
		return &VectorAggregatorReconciler{
			Client:             c,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
	}

	It("has already published assets when an injected config-write failure aborts the reconcile", func() {
		ns := "va-order-fail-config"
		aggName := "va-order-fail-config-agg"
		_, agg := setupFixture(ns, aggName)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: aggName}}
		// The very first reconcile of a fresh VectorAggregator only adds the
		// finalizer and returns - see Reconcile's !HasFinalizer branch - so it has
		// to run once, unfaulted, before createOrUpdateVectorAggregator (where the
		// write order actually happens) is ever reached.
		_, err := newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		configSecretName := aggName + "-aggregator"
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

		_, err = newReconciler(faultyClient).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected config-write failure must surface as a Reconcile error")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator-secret-assets"}, assetsSecret)).To(Succeed(),
			"the assets Secret must already be on the API server - assets comes before config in EnsureVectorAggregator")
		Expect(assetsSecret.Data).To(HaveKey("va_order_fail_config_app_es_cert"))

		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: configSecretName}, &corev1.Secret{})
		Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the config Secret must not exist - its create was the one that failed")

		gotAgg := &v1alpha1.VectorAggregator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agg), gotAgg)).To(Succeed())
	})

	It("never attempts the config write at all when the workload (Deployment) write itself fails", func() {
		ns := "va-order-fail-workload"
		aggName := "va-order-fail-workload-agg"
		setupFixture(ns, aggName)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: aggName}}
		_, err := newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		deploymentName := aggName + "-aggregator"
		injected := errors.New("injected deployment write failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if d, ok := obj.(*appsv1.Deployment); ok && d.Name == deploymentName {
					return injected
				}
				return c.Create(ctx, obj, opts...)
			},
		})

		_, err = newReconciler(faultyClient).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred())

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator-secret-assets"}, assetsSecret)).To(Succeed(),
			"assets must already be published - it comes before the workload in the mount-gaining order")

		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: deploymentName}, &appsv1.Deployment{})
		Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the Deployment must not exist - its create was the one that failed")

		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator"}, &corev1.Secret{})
		Expect(api_errors.IsNotFound(err)).To(BeTrue(),
			"the config Secret must not exist - a config referencing a key a workload with no mount cannot resolve "+
				"would be exactly the crash-loop-on-restart bug this ordering exists to prevent")
	})

	// See the identical spec on the ClusterVectorAggregator path: the publish-mark
	// stamp is a precondition of publishing, and only the agent path had that pinned
	// with an injected failure.
	It("never attempts the config write at all when the pre-publish stamp write itself fails", func() {
		ns := "va-order-fail-stamp"
		aggName := "va-order-fail-stamp-agg"
		setupFixture(ns, aggName)

		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: aggName}}
		_, err := newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		injected := errors.New("injected status stamp failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				if subResourceName == "status" {
					if agg, ok := obj.(*v1alpha1.VectorAggregator); ok && agg.Name == aggName {
						return injected
					}
				}
				return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		})

		_, err = newReconciler(faultyClient).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected stamp-write failure must surface as a Reconcile error")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator-secret-assets"}, assetsSecret)).To(Succeed(),
			"assets must already be published - the stamp write is the step immediately AFTER it, not before")

		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator"}, &corev1.Secret{})
		Expect(api_errors.IsNotFound(err)).To(BeTrue(),
			"the config Secret must not exist - a failed stamp must block the config write outright")
	})
})
