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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	observabilityv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

var _ = Describe("ClusterVectorAggregator Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		clustervectoraggregator := &observabilityv1alpha1.ClusterVectorAggregator{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterVectorAggregator")
			err := k8sClient.Get(ctx, typeNamespacedName, clustervectoraggregator)
			if err != nil && errors.IsNotFound(err) {
				resource := &observabilityv1alpha1.ClusterVectorAggregator{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &observabilityv1alpha1.ClusterVectorAggregator{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ClusterVectorAggregator")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterVectorAggregatorReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				EventChan:          make(chan event.GenericEvent, 1),
				ConfigCheckTimeout: configCheckTimeout,
				Clientset:          clientset,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("When running under a manager", func() {
		const resourceName = "cva-predicate"

		namespacedName := types.NamespacedName{Name: resourceName}

		// A status-only update must not retrigger the reconcile: without the
		// generation predicate the controller's own Status().Update feeds back
		// into it and CVAs reconcile in a hot conflict loop.
		It("does not reconcile on status-only updates", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:  k8sClient.Scheme(),
				Metrics: metricsserver.Options{BindAddress: "0"},
			})
			Expect(err).NotTo(HaveOccurred())

			reconciler := &ClusterVectorAggregatorReconciler{
				Client:             mgr.GetClient(),
				Scheme:             mgr.GetScheme(),
				EventChan:          make(chan event.GenericEvent, 10),
				ConfigCheckTimeout: configCheckTimeout,
				Clientset:          clientset,
			}
			Expect(reconciler.SetupWithManager(mgr)).To(Succeed())

			mgrCtx, mgrCancel := context.WithCancel(ctx)
			defer mgrCancel()
			go func() {
				defer GinkgoRecover()
				Expect(mgr.Start(mgrCtx)).To(Succeed())
			}()

			resource := &observabilityv1alpha1.ClusterVectorAggregator{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName},
				Spec: observabilityv1alpha1.ClusterVectorAggregatorSpec{
					ResourceNamespace: "default",
					VectorAggregatorCommon: observabilityv1alpha1.VectorAggregatorCommon{
						VectorCommon: observabilityv1alpha1.VectorCommon{
							ConfigCheck: observabilityv1alpha1.ConfigCheck{Disabled: true},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}()

			By("waiting for the first reconcile to write status")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, namespacedName, resource)).To(Succeed())
				g.Expect(resource.Status.ConfigCheckResult).NotTo(BeNil())
			}, "10s", "250ms").Should(Succeed())

			By("waiting for the create-time reconcile chain to settle")
			Eventually(func(g Gomega) {
				before := &observabilityv1alpha1.ClusterVectorAggregator{}
				g.Expect(k8sClient.Get(ctx, namespacedName, before)).To(Succeed())
				time.Sleep(time.Second)
				after := &observabilityv1alpha1.ClusterVectorAggregator{}
				g.Expect(k8sClient.Get(ctx, namespacedName, after)).To(Succeed())
				g.Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
				resource = after
			}, "20s", "100ms").Should(Succeed())

			By("touching only the status")
			touch := "external-status-touch"
			resource.Status.Reason = &touch
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("verifying the touch is not reverted by a reconcile echo")
			Consistently(func(g Gomega) {
				got := &observabilityv1alpha1.ClusterVectorAggregator{}
				g.Expect(k8sClient.Get(ctx, namespacedName, got)).To(Succeed())
				g.Expect(got.Status.Reason).To(HaveValue(Equal(touch)))
			}, "4s", "500ms").Should(Succeed())
		})
	})
})
