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

	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

var _ = Describe("VectorPipeline Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		vectorpipeline := &v1alpha1.VectorPipeline{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind VectorPipeline")
			err := k8sClient.Get(ctx, typeNamespacedName, vectorpipeline)
			if err != nil && errors.IsNotFound(err) {
				resource := &v1alpha1.VectorPipeline{
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
			resource := &v1alpha1.VectorPipeline{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance VectorPipeline")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PipelineReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				Clientset:          clientset,
				ConfigCheckTimeout: configCheckTimeout,
				VectorAgentEventCh: make(chan event.GenericEvent, 1),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("When the pipeline pins an unknown spec.role", func() {
		It("should be rejected by the apiserver", func() {
			bogus := v1alpha1.VectorPipelineRole("custom-role")
			pipeline := &v1alpha1.ClusterVectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "bogus-role-pipeline"},
				Spec:       v1alpha1.VectorPipelineSpec{Role: &bogus},
			}

			err := k8sClient.Create(context.Background(), pipeline)
			Expect(errors.IsInvalid(err)).To(BeTrue(), "expected an Invalid error, got %v", err)
		})
	})

	Context("When resolving the pipeline role", func() {
		ctx := context.Background()
		aggregatorName := "unselected-aggregator"
		// prometheus_remote_write is an agent source type, so inference alone never sends a
		// pipeline carrying it to an aggregator.
		prwSources := &runtime.RawExtension{Raw: []byte(`{"prw_in":{"type":"prometheus_remote_write","address":"0.0.0.0:9098"}}`)}
		sinks := &runtime.RawExtension{Raw: []byte(`{"out":{"type":"blackhole","inputs":["prw_in"]}}`)}
		aggregatorRole := v1alpha1.VectorPipelineRoleAggregator

		reconciler := func() *PipelineReconciler {
			return &PipelineReconciler{
				Client:                          k8sClient,
				Scheme:                          k8sClient.Scheme(),
				Clientset:                       clientset,
				ConfigCheckTimeout:              configCheckTimeout,
				VectorAgentEventCh:              make(chan event.GenericEvent, 1),
				VectorAggregatorsEventCh:        make(chan event.GenericEvent, 1),
				ClusterVectorAggregatorsEventCh: make(chan event.GenericEvent, 1),
			}
		}

		// The aggregator selects labels no pipeline here carries, so the reconcile skips
		// configcheck (no vector pod to run in envtest) and still resolves the role.
		BeforeEach(func() {
			aggregator := &v1alpha1.ClusterVectorAggregator{
				ObjectMeta: metav1.ObjectMeta{Name: aggregatorName},
				Spec: v1alpha1.ClusterVectorAggregatorSpec{
					VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{
						Selector: &v1alpha1.VectorSelectorSpec{MatchLabels: map[string]string{"pipeline": "other"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aggregator)).To(Succeed())
		})

		AfterEach(func() {
			aggregator := &v1alpha1.ClusterVectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: aggregatorName}}
			Expect(k8sClient.Delete(ctx, aggregator)).To(Succeed())
		})

		It("should use the pinned role instead of the inferred one", func() {
			name := types.NamespacedName{Name: "pinned-cluster-pipeline"}
			pipeline := &v1alpha1.ClusterVectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name},
				Spec:       v1alpha1.VectorPipelineSpec{Role: &aggregatorRole, Sources: prwSources, Sinks: sinks},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed()) })

			_, err := reconciler().Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			reconciled := &v1alpha1.ClusterVectorPipeline{}
			Expect(k8sClient.Get(ctx, name, reconciled)).To(Succeed())
			Expect(reconciled.GetRole()).To(Equal(v1alpha1.VectorPipelineRoleAggregator))
			Expect(reconciled.IsValid()).To(BeTrue())
		})

		It("should infer the role when spec.role is empty", func() {
			name := types.NamespacedName{Name: "unpinned-cluster-pipeline"}
			pipeline := &v1alpha1.ClusterVectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name},
				Spec:       v1alpha1.VectorPipelineSpec{Sources: prwSources, Sinks: sinks},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed()) })

			_, err := reconciler().Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			reconciled := &v1alpha1.ClusterVectorPipeline{}
			Expect(k8sClient.Get(ctx, name, reconciled)).To(Succeed())
			Expect(reconciled.GetRole()).To(Equal(v1alpha1.VectorPipelineRoleAgent))
		})

		It("should let a namespaced pipeline pin a network listener to the aggregator", func() {
			name := types.NamespacedName{Name: "pinned-namespaced-pipeline", Namespace: "default"}
			pipeline := &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec:       v1alpha1.VectorPipelineSpec{Role: &aggregatorRole, Sources: prwSources, Sinks: sinks},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed()) })

			_, err := reconciler().Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			reconciled := &v1alpha1.VectorPipeline{}
			Expect(k8sClient.Get(ctx, name, reconciled)).To(Succeed())
			Expect(reconciled.GetRole()).To(Equal(v1alpha1.VectorPipelineRoleAggregator))
			Expect(reconciled.IsValid()).To(BeTrue())
		})

		It("should reject a host source on a namespaced aggregator pipeline", func() {
			name := types.NamespacedName{Name: "host-source-pipeline", Namespace: "default"}
			pipeline := &v1alpha1.VectorPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: v1alpha1.VectorPipelineSpec{
					Role:    &aggregatorRole,
					Sources: &runtime.RawExtension{Raw: []byte(`{"logs":{"type":"kubernetes_logs"}}`)},
					Sinks:   &runtime.RawExtension{Raw: []byte(`{"out":{"type":"blackhole","inputs":["logs"]}}`)},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed()) })

			_, err := reconciler().Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			reconciled := &v1alpha1.VectorPipeline{}
			Expect(k8sClient.Get(ctx, name, reconciled)).To(Succeed())
			Expect(reconciled.IsValid()).To(BeFalse())
			Expect(*reconciled.Status.Reason).To(ContainSubstring("host source types not allowed"))
		})
	})
})
