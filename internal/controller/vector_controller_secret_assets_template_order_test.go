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

// This suite pins a related write-order hazard: the write-order fault injection in
// vector_controller_secret_assets_write_order_test.go only ever pinned
// the assets/config boundary. It never proved anything about where the DaemonSet's
// own pod template write falls - and a new pod (created by ANY trigger: an eviction,
// a node drain, a new node) always gets whatever template happens to be persisted at
// that instant, so a template lacking the secret-assets mount is exactly as fatal as
// a config referencing a key the assets Secret does not have, if the config on the
// cluster at that same instant already needs it. See EnsureVectorAgent's own doc
// comment for the full ordering rationale this suite is pinning.
var _ = Describe("VectorReconciler secret-assets template-boundary fault injection", func() {
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

	daemonSetHasMount := func(ds *appsv1.DaemonSet) bool {
		for _, v := range ds.Spec.Template.Spec.Volumes {
			if v.Name == "secret-assets" {
				return true
			}
		}
		return false
	}

	Context("gaining the mount for the first time", func() {
		It("has already published assets AND the DaemonSet mount when an injected config-write failure aborts the reconcile", func() {
			ns := "tmpl-order-fail-config"
			vectorName := "tmpl-order-fail-config-vector"
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
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
			Expect(assetsSecret.Data).To(HaveKey("tmpl_order_fail_config_app_es_cert"))

			daemonSet := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, daemonSet)).To(Succeed(),
				"the DaemonSet must already exist - if a future regression put config back before the template write, this GET would race an object that may not exist yet")
			Expect(daemonSetHasMount(daemonSet)).To(BeTrue(),
				"the template must already carry the mount before config is ever allowed to reference it")

			configSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: configSecretName}, configSecret)
			Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the config Secret must not exist - its create was the one that failed")

			gotVector := &v1alpha1.Vector{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vector), gotVector)).To(Succeed())
		})

		It("never attempts the config write at all when the DaemonSet template write itself fails", func() {
			ns := "tmpl-order-fail-ds"
			vectorName := "tmpl-order-fail-ds-vector"
			setupFixture(ns, vectorName)

			daemonSetName := vectorName + "-agent"
			injected := errors.New("injected daemonset write failure")
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if ds, ok := obj.(*appsv1.DaemonSet); ok && ds.Name == daemonSetName {
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

			assetsSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed(),
				"assets must already be published - it comes before the template in the mount-gaining order")

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: daemonSetName}, &appsv1.DaemonSet{})
			Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the DaemonSet must not exist - its create was the one that failed")

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, &corev1.Secret{})
			Expect(api_errors.IsNotFound(err)).To(BeTrue(),
				"the config Secret must not exist - a config referencing a key a template with no mount cannot resolve "+
					"would be exactly the crash-loop-on-restart bug this ordering exists to prevent")
		})
	})

	Context("losing the mount entirely", func() {
		It("never deletes the assets Secret before the DaemonSet template's mount is actually removed", func() {
			ns := "tmpl-order-fail-remove"
			vectorName := "tmpl-order-fail-remove-vector"
			pl, _ := setupFixture(ns, vectorName)

			plainReconciler := &VectorReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				Clientset:          clientset,
				ConfigCheckTimeout: configCheckTimeout,
				DiscoveryClient:    clientset.DiscoveryClient,
				EventChan:          make(chan event.GenericEvent, 1),
				APIReader:          k8sClient,
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}

			By("reconcile #1: establishes the mount")
			_, err := plainReconciler.Reconcile(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			daemonSet := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, daemonSet)).To(Succeed())
			Expect(daemonSetHasMount(daemonSet)).To(BeTrue())

			By("dropping the pipeline's only secret reference")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pl), pl)).To(Succeed())
			pl.Spec.Sinks = &runtime.RawExtension{Raw: []byte(`{"out": {"type": "elasticsearch", "inputs": ["logs"], "endpoints": ["http://elasticsearch.example.com:9200"], "healthcheck": {"enabled": false}}}`)}
			Expect(k8sClient.Update(ctx, pl)).To(Succeed())
			agentRole := v1alpha1.VectorPipelineRoleAgent
			pl.SetRole(&agentRole)
			Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

			By("reconcile #2: the config drops the reference, but the stale key stays staged (deferred prune) and the mount stays put")
			_, err = plainReconciler.Reconcile(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			assetsSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed())
			Expect(assetsSecret.Data).NotTo(BeEmpty(), "still staged - not yet confirmed unchanged, let alone past the grace period")

			By("backdating the publish mark past the grace period, standing in for real time actually passing")
			backdateVectorConfigPublishedAt(vectorName)

			By("reconcile #3 (DaemonSet write fails): the prune round now wants to drop the mount, but the template write itself is injected to fail")
			daemonSetName := vectorName + "-agent"
			injected := errors.New("injected daemonset update failure")
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if ds, ok := obj.(*appsv1.DaemonSet); ok && ds.Name == daemonSetName {
						return injected
					}
					return c.Update(ctx, obj, opts...)
				},
			})
			faultyReconciler := &VectorReconciler{
				Client:             faultyClient,
				Scheme:             k8sClient.Scheme(),
				Clientset:          clientset,
				ConfigCheckTimeout: configCheckTimeout,
				DiscoveryClient:    clientset.DiscoveryClient,
				EventChan:          make(chan event.GenericEvent, 1),
				APIReader:          k8sClient,
			}
			_, err = faultyReconciler.Reconcile(context.Background(), req)
			Expect(err).To(HaveOccurred(), "the injected DaemonSet write failure must surface as a Reconcile error")

			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed(),
				"the assets Secret must NOT have been deleted - the template's mount removal (which failed) must land first")
			Expect(assetsSecret.Data).NotTo(BeEmpty())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: daemonSetName}, daemonSet)).To(Succeed())
			Expect(daemonSetHasMount(daemonSet)).To(BeTrue(), "the template update failed, so the mount must still be exactly as it was")
		})
	})
})

// This suite pins a related hazard: hasSecretAssetsMount used to
// match the DaemonSet's pod template by volume NAME alone ("secret-assets"), not by
// checking it is actually the operator's own Secret-backed volume. Before a
// workload's first-ever secret reference, ctrl.SecretAssets is empty and the
// operator never touches the volume list at all (SetAuthoritativeVolume only runs
// once SecretAssets is non-empty - see its own doc comment and
// TestDaemonSetSecretAssetsVolumeIsAuthoritative), so a user's own, unrelated
// volume that merely happens to be named "secret-assets" (an EmptyDir, here) passes
// straight through untouched. A name-only check would read that as "the mount
// already exists", skip the gaining-the-mount write order (assets -> DaemonSet ->
// config) on the exact round that order exists for, and publish a config
// referencing the new key before the DaemonSet template gets the OPERATOR's real
// mount.
var _ = Describe("VectorReconciler secret-assets mount detection ignores a same-named user volume", func() {
	It("still takes the mount-gaining write order when a pre-existing user volume merely shares the reserved name", func() {
		ns := "tmpl-order-user-volume"
		vectorName := "tmpl-order-user-volume-vector"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"cert": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		vector := &v1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: vectorName, Namespace: "default"},
			Spec: v1alpha1.VectorSpec{
				Agent: &v1alpha1.VectorAgent{
					VectorCommon: v1alpha1.VectorCommon{
						ConfigCheck: v1alpha1.ConfigCheck{Disabled: true},
						// Volumes is set explicitly (not left nil) so SetDefault does not fill
						// in the required var-log/journal/var-lib set on its own - it only does
						// that when Volumes is nil. The required three are reproduced here
						// alongside the extra one, so the only thing under test is the extra
						// "secret-assets" entry: a user volume that squats the reserved name
						// for their own, unrelated purpose - a supported input (see
						// TestDaemonSetSecretAssetsVolumeIsAuthoritative) as long as no
						// pipeline references a secret yet.
						Volumes: []corev1.Volume{
							{Name: "var-log", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/log/"}}},
							{Name: "journal", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/log/journal"}}},
							{Name: "var-lib", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/"}}},
							{Name: "secret-assets", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, vector)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, vector) })

		plainReconciler := &VectorReconciler{
			Client:             k8sClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}

		By("reconcile #1: establishes the DaemonSet with the user's own, unrelated secret-assets volume - no pipeline uses a secret yet")
		_, err := plainReconciler.Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		daemonSetName := vectorName + "-agent"
		daemonSet := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: daemonSetName}, daemonSet)).To(Succeed())
		foundUserVolume := false
		for _, v := range daemonSet.Spec.Template.Spec.Volumes {
			if v.Name == "secret-assets" {
				foundUserVolume = true
				Expect(v.EmptyDir).NotTo(BeNil(), "must still be the user's own EmptyDir, not the operator's Secret source")
			}
		}
		Expect(foundUserVolume).To(BeTrue())

		By("creating a pipeline that references a secret for the first time")
		pl := &v1alpha1.VectorPipeline{
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

		By("reconcile #2 (DaemonSet write fails): if hasSecretAssetsMount mistook the user's volume for ours, this round would take the " +
			"default order (config before DaemonSet) and publish the config before the failing write - the assertion below catches exactly that")
		injected := errors.New("injected daemonset write failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if ds, ok := obj.(*appsv1.DaemonSet); ok && ds.Name == daemonSetName {
					return injected
				}
				return c.Update(ctx, obj, opts...)
			},
		})
		faultyReconciler := &VectorReconciler{
			Client:             faultyClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
		_, err = faultyReconciler.Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected DaemonSet write failure must surface as a Reconcile error")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent-secret-assets"}, assetsSecret)).To(Succeed(),
			"assets must already be published - it comes first in the mount-gaining order regardless of the pre-existing user volume")
		Expect(assetsSecret.Data).To(HaveKey("tmpl_order_user_volume_app_es_cert"))

		// Round #1 (before any pipeline used a secret) already created a config
		// Secret with no SECRET[] reference in it - so the assertion here is that its
		// CONTENT was never updated to reference the new key, not that the object is
		// absent outright.
		configSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: vectorName + "-agent"}, configSecret)).To(Succeed())
		Expect(string(configSecret.Data["agent.json"])).NotTo(ContainSubstring("tmpl_order_user_volume_app_es_cert"),
			"the config must not yet reference the new key - hasSecretAssetsMount must have reported no OPERATOR mount yet (the user's "+
				"same-named volume does not count), taking the assets -> DaemonSet -> config order, so the failing DaemonSet write must "+
				"abort before config is ever rewritten to reference it")
	})
})
