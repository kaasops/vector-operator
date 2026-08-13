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

// See vectoraggregator_secret_assets_order_test.go's identical suite for the full
// rationale - this copies the same two fault-injection tests onto
// ClusterVectorAggregator, the second aggregator path that had no fault-injection
// coverage of its own.
var _ = Describe("ClusterVectorAggregatorReconciler secret-assets write-order fault injection", func() {
	setupFixture := func(ns, aggName string) (cvp *v1alpha1.ClusterVectorPipeline, agg *v1alpha1.ClusterVectorAggregator) {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"cert": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		cvp = &v1alpha1.ClusterVectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "cva-order-app"},
			Spec: v1alpha1.VectorPipelineSpec{
				// ClusterVectorPipeline backends must set namespace explicitly -
				// there is no "own namespace" to default to.
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds", Namespace: ns},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "vector", "address": "0.0.0.0:6000"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["in"], "auth": {"user": "SECRET[es.cert]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, cvp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cvp) })
		aggregatorRole := v1alpha1.VectorPipelineRoleAggregator
		cvp.SetRole(&aggregatorRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, cvp, pipelineStatusBase(cvp))).To(Succeed())

		agg = &v1alpha1.ClusterVectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: aggName},
			Spec: v1alpha1.ClusterVectorAggregatorSpec{
				ResourceNamespace: ns,
				VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{
					VectorCommon: v1alpha1.VectorCommon{
						ConfigCheck: v1alpha1.ConfigCheck{Disabled: true},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agg)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agg) })
		return cvp, agg
	}

	newReconciler := func(c client.Client) *ClusterVectorAggregatorReconciler {
		return &ClusterVectorAggregatorReconciler{
			Client:             c,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
	}

	It("has already published assets when an injected config-write failure aborts the reconcile", func() {
		ns := "cva-order-fail-config"
		aggName := "cva-order-fail-config-agg"
		_, agg := setupFixture(ns, aggName)

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

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: aggName}}
		_, err = newReconciler(faultyClient).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected config-write failure must surface as a Reconcile error")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator-secret-assets"}, assetsSecret)).To(Succeed(),
			"the assets Secret must already be on the API server - assets comes before config in EnsureVectorAggregator")
		Expect(assetsSecret.Data).To(HaveKey("cva_order_app_es_cert"))

		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: configSecretName}, &corev1.Secret{})
		Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the config Secret must not exist - its create was the one that failed")

		gotAgg := &v1alpha1.ClusterVectorAggregator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agg), gotAgg)).To(Succeed())
	})

	It("never attempts the config write at all when the workload (Deployment) write itself fails", func() {
		ns := "cva-order-fail-workload"
		aggName := "cva-order-fail-workload-agg"
		setupFixture(ns, aggName)

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

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: aggName}}
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

	// The publish-mark stamp had fault-injection coverage on the agent path only, so
	// nothing held either aggregator path to the same contract: the stamp is a
	// PRECONDITION of publishing, not a best-effort side write, and its failure must
	// abort the round before the config Secret is touched. Without this, a regression
	// that logged-and-continued past a failed stamp would pass every other spec.
	It("never attempts the config write at all when the pre-publish stamp write itself fails", func() {
		ns := "cva-order-fail-stamp"
		aggName := "cva-order-fail-stamp-agg"
		setupFixture(ns, aggName)

		injected := errors.New("injected status stamp failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				if subResourceName == "status" {
					if agg, ok := obj.(*v1alpha1.ClusterVectorAggregator); ok && agg.Name == aggName {
						return injected
					}
				}
				return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		})

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: aggName}}
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
