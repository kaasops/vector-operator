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
	"time"

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

// backdateAggregatorConfigPublishedAt moves the aggregator's publish mark a full
// grace period into the past, the aggregator counterpart of
// backdateVectorConfigPublishedAt - the only way an envtest spec can reach the
// far side of SecretAssetsPruneGracePeriod without actually sleeping 90 seconds.
func backdateAggregatorConfigPublishedAt(namespace, name string) {
	agg := &v1alpha1.VectorAggregator{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, agg)).To(Succeed())
	past := metav1.NewTime(time.Now().Add(-SecretAssetsPruneGracePeriod - time.Second))
	agg.Status.LastConfigPublishedAt = &past
	Expect(k8sClient.Status().Update(ctx, agg)).To(Succeed())
}

// This suite pins the aggregator-only half of the mount gate, a bug that no
// other spec can reach: hasSecretAssetsMount used to read only the workload kind
// persistenceEnabled() currently selects. Toggling
// persistence switches between a Deployment and a StatefulSet that share a name, and
// the OLD kind is only removed by ensureWorkload's deleteObsoleteWorkload - which
// runs strictly after the gate has already chosen this round's write order. So on a
// round that BOTH toggles persistence AND loses the mount (the last secret reference
// was dropped a round earlier and the grace period has now elapsed), asking only the
// new kind finds nothing at all, reports hadMount=false, and takes the ordinary
// order - which publishes the pruned (here: deleted) assets Secret FIRST, while the
// old kind's pods are still running and still mounting it. Any pod either kind
// creates in that window gets a FailedMount, which is the same class of breakage the
// whole ordering exists to prevent.
//
// The failure is only observable with a fault injected at that exact boundary: with
// every write succeeding, both orders converge on the same end state and a
// regression would pass silently.
var _ = Describe("VectorAggregatorReconciler secret-assets mount gate across a persistence toggle", func() {
	It("does not delete the assets Secret before the workload write when the last key is dropped on the same round persistence flips", func() {
		ns := "va-mount-gate-toggle"
		aggName := "va-mount-gate-toggle-agg"
		flatKey := "va_mount_gate_toggle_app_es_cert"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns},
			Data:       map[string][]byte{"cert": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		vp := &v1alpha1.VectorPipeline{
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

		agg := &v1alpha1.VectorAggregator{
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
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: aggName}}

		By("reconcile #1: the first round of a fresh aggregator only adds the finalizer")
		_, err := newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		By("reconcile #2: publishes the Deployment WITH the operator's mount, the assets Secret and a config that references the key")
		_, err = newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		workloadName := aggName + "-aggregator"
		assetsName := types.NamespacedName{Namespace: ns, Name: workloadName + "-secret-assets"}
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: workloadName}, deployment)).To(Succeed())
		Expect(hasOperatorSecretAssetsVolume(deployment.Spec.Template.Spec, workloadName+"-secret-assets")).To(BeTrue())
		assetsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, assetsName, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey(flatKey))

		By("dropping the pipeline's secret reference entirely - the workload's last one")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vp), vp)).To(Succeed())
		vp.Spec.Secret = nil
		vp.Spec.Sinks = &runtime.RawExtension{Raw: []byte(
			`{"out": {"type": "elasticsearch", "inputs": ["in"]}}`,
		)}
		Expect(k8sClient.Update(ctx, vp)).To(Succeed())
		vp.SetRole(&aggregatorRole)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, vp, pipelineStatusBase(vp))).To(Succeed())

		By("reconcile #3: the config stops referencing the key, but the key itself is kept - the grace period has not elapsed")
		_, err = newReconciler(k8sClient).Reconcile(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, assetsName, assetsSecret)).To(Succeed())
		Expect(assetsSecret.Data).To(HaveKey(flatKey),
			"nothing may be pruned while a pod may still be running the previous config")

		By("waiting out the grace period and flipping persistence on in the same breath")
		backdateAggregatorConfigPublishedAt(ns, aggName)
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agg), agg)).To(Succeed())
		agg.Spec.Persistence.Enabled = true
		Expect(k8sClient.Update(ctx, agg)).To(Succeed())

		By("reconcile #4 (StatefulSet write fails): the round both loses the mount and switches kind")
		injected := errors.New("injected statefulset write failure")
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		faultyClient := interceptor.NewClient(watchClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if sts, ok := obj.(*appsv1.StatefulSet); ok && sts.Name == workloadName {
					return injected
				}
				return c.Create(ctx, obj, opts...)
			},
		})
		_, err = newReconciler(faultyClient).Reconcile(context.Background(), req)
		Expect(err).To(HaveOccurred(), "the injected StatefulSet write failure must surface as a Reconcile error")

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: workloadName}, deployment)).To(Succeed(),
			"the old kind is still there - deleteObsoleteWorkload only runs once the new kind is written, which is the write that just failed")
		Expect(hasOperatorSecretAssetsVolume(deployment.Spec.Template.Spec, workloadName+"-secret-assets")).To(BeTrue(),
			"and it still mounts the assets Secret, so its running pods still depend on that Secret existing")

		Expect(k8sClient.Get(ctx, assetsName, assetsSecret)).To(Succeed(),
			"a gate that only looked at the NEW kind would have found no workload at all, reported hadMount=false, and taken the ordinary "+
				"order - deleting the assets Secret before the workload write that just failed, leaving the still-live Deployment's pods "+
				"mounting a Secret that no longer exists")
		Expect(assetsSecret.Data).To(HaveKey(flatKey))
	})
})

// hasOperatorSecretAssetsVolume mirrors k8s.HasOperatorSecretAssetsMount's volume
// half, kept local to the spec so the assertions read against the pod template the
// API server actually holds rather than through the helper under test.
func hasOperatorSecretAssetsVolume(spec corev1.PodSpec, secretName string) bool {
	for _, v := range spec.Volumes {
		if v.Name == "secret-assets" && v.Secret != nil && v.Secret.SecretName == secretName {
			return true
		}
	}
	return false
}
