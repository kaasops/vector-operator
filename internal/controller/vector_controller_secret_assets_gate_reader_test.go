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

// stripSecretAssetsMount removes the operator's secret-assets volume and its
// container mounts from spec, turning a pod template that HAS the mount into the
// one an informer cache that has not yet observed the round which added it would
// still be serving.
func stripSecretAssetsMount(spec *corev1.PodSpec) {
	volumes := spec.Volumes[:0]
	for _, v := range spec.Volumes {
		if v.Name != "secret-assets" {
			volumes = append(volumes, v)
		}
	}
	spec.Volumes = volumes
	for i := range spec.Containers {
		mounts := spec.Containers[i].VolumeMounts[:0]
		for _, m := range spec.Containers[i].VolumeMounts {
			if m.Name != "secret-assets" {
				mounts = append(mounts, m)
			}
		}
		spec.Containers[i].VolumeMounts = mounts
	}
}

// This suite pins WHICH READER answers the secret-assets write-order gate. The gate
// picks this round's write order, so it is a safeguard - and a safeguard decided off
// an informer cache that has not caught up yet is no safeguard: the DaemonSet write
// of a preceding round may simply not have been observed. hasSecretAssetsMount
// therefore reads through the reconciler's APIReader (mgr.GetAPIReader()), never
// through the cached client the rest of the reconcile uses.
//
// The spec deliberately hands the reconciler TWO different readers that DISAGREE -
// a cached client whose DaemonSet Get is doctored to return the pre-mount template,
// and an APIReader serving the real one - so the assertion is about which of the two
// the decision followed, not merely that the code compiles with an APIReader field
// wired in. With both readers agreeing (as they do in every other spec) a regression
// back to ctrl.Get would pass silently.
var _ = Describe("VectorReconciler secret-assets write-order gate reader", func() {
	It("decides from the uncached reader when the cached client is still serving a pre-mount template", func() {
		ns := "gate-reader-stale-cache"
		vectorName := "gate-reader-stale-cache-vector"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"k1": []byte("v1"), "k2": []byte("v2")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		pl := &v1alpha1.VectorPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
			Spec: v1alpha1.VectorPipelineSpec{
				Secret: map[string]v1alpha1.PipelineSecretBackend{
					"es": {Type: "kubernetes_secret", Name: "creds"},
				},
				Sources: &runtime.RawExtension{Raw: []byte(`{"logs": {"type": "kubernetes_logs"}}`)},
				Sinks: &runtime.RawExtension{Raw: []byte(
					`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k1]"}}}`,
				)},
			},
		}
		Expect(k8sClient.Create(ctx, pl)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })
		agentRole := v1alpha1.VectorPipelineRoleAgent
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

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

		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: vectorName}}
		daemonSetName := vectorName + "-agent"
		configSecretName := vectorName + "-agent"

		By("reconcile #1: the mount is really added - DaemonSet, assets and config all reference k1")
		_, err := (&VectorReconciler{
			Client:             k8sClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		daemonSet := &appsv1.DaemonSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: daemonSetName}, daemonSet)).To(Succeed())
		Expect(hasOperatorSecretAssetsVolume(daemonSet.Spec.Template.Spec, daemonSetName+"-secret-assets")).To(BeTrue())

		By("editing the pipeline to reference k2 as well, so this round genuinely (re-)writes the config")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pl), pl)).To(Succeed())
		pl.Spec.Sinks = &runtime.RawExtension{Raw: []byte(
			`{"out": {"type": "elasticsearch", "inputs": ["logs"], "auth": {"user": "SECRET[es.k1]", "password": "SECRET[es.k2]"}}}`,
		)}
		Expect(k8sClient.Update(ctx, pl)).To(Succeed())
		pl.SetRole(&agentRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, pl, pipelineStatusBase(pl))).To(Succeed())

		By("reconcile #2: the cached client lies about the DaemonSet (no mount), the APIReader tells the truth, and the DaemonSet write fails")
		injected := errors.New("injected daemonset write failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		staleCachedClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := c.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				if ds, ok := obj.(*appsv1.DaemonSet); ok && ds.Name == daemonSetName {
					stripSecretAssetsMount(&ds.Spec.Template.Spec)
				}
				return nil
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if ds, ok := obj.(*appsv1.DaemonSet); ok && ds.Name == daemonSetName {
					return injected
				}
				return c.Update(ctx, obj, opts...)
			},
		})

		_, err = (&VectorReconciler{
			Client:             staleCachedClient,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			DiscoveryClient:    clientset.DiscoveryClient,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected DaemonSet write failure must surface as a Reconcile error")

		configSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: configSecretName}, configSecret)).To(Succeed())
		Expect(string(configSecret.Data["agent.json"])).To(ContainSubstring("gate_reader_stale_cache_app_es_k2"),
			"the mount is already there according to the API server, so this round is not crossing the boundary at all and takes the "+
				"ordinary order (assets -> config -> DaemonSet): the config write happens BEFORE the failing DaemonSet write. Had the "+
				"gate believed the cached client's pre-mount template, it would have taken the mount-gaining order instead and aborted "+
				"on the DaemonSet write, leaving the config without k2")

		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: daemonSetName + "-secret-assets"}, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey("gate_reader_stale_cache_app_es_k2"),
			"assets are written first in either order, so this holds regardless - it only rules out the round having failed even earlier")
	})
})
