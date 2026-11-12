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
	"fmt"
	"slices"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

// The aggregator skips a pipeline Service the API server rejects, reconciles the rest and names the Service in its status; it is created after its pipelines are valid.
var _ = Describe("Aggregator pipeline Services the API server rejects", func() {
	const longPipelineName = "payments-otel-collector-ingest-production-eu-west-1"

	// The result of a reconcile that skipped a Service.
	retry := reconcile.Result{RequeueAfter: skippedServiceRetryDelay}

	newVAReconciler := func(c client.Client) *VectorAggregatorReconciler {
		return &VectorAggregatorReconciler{
			Client:             c,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
	}

	newCVAReconciler := func(c client.Client) *ClusterVectorAggregatorReconciler {
		return &ClusterVectorAggregatorReconciler{
			Client:             c,
			Scheme:             k8sClient.Scheme(),
			Clientset:          clientset,
			ConfigCheckTimeout: configCheckTimeout,
			EventChan:          make(chan event.GenericEvent, 1),
			APIReader:          k8sClient,
		}
	}

	createNamespace := func(ns string) {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}) })
	}

	pipelineSpec := func(sources string) v1alpha1.VectorPipelineSpec {
		return v1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(sources)},
			Sinks:   &runtime.RawExtension{Raw: []byte(`{"out": {"type": "console", "inputs": ["in"], "encoding": {"codec": "json"}}}`)},
		}
	}

	// createValidPipeline creates p with the status the pipeline controller gives a valid aggregator pipeline.
	createValidPipeline := func(p pipeline.Pipeline) {
		Expect(k8sClient.Create(ctx, p)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, p) })
		role := v1alpha1.VectorPipelineRoleAggregator
		p.SetRole(&role)
		Expect(pipeline.SetSuccessStatus(ctx, k8sClient, p, pipelineStatusBase(p))).To(Succeed())
	}

	vectorPipeline := func(ns, name, sources string) *v1alpha1.VectorPipeline {
		return &v1alpha1.VectorPipeline{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: pipelineSpec(sources)}
	}

	// createVectorAggregator creates the aggregator and runs the first reconcile, which only adds the finalizer.
	createVectorAggregator := func(ns, name string, common v1alpha1.VectorAggregatorCommon) types.NamespacedName {
		common.ConfigCheck = v1alpha1.ConfigCheck{Disabled: true}
		agg := &v1alpha1.VectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       v1alpha1.VectorAggregatorSpec{VectorAggregatorCommon: common},
		}
		Expect(k8sClient.Create(ctx, agg)).To(Succeed())
		key := client.ObjectKeyFromObject(agg)
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, agg)).To(Succeed())
			// envtest runs no controller, so the aggregator would otherwise keep its finalizer.
			_, err := newVAReconciler(k8sClient).Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
		})
		_, err := newVAReconciler(k8sClient).Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		return key
	}

	reconcileVA := func(c client.Client, key types.NamespacedName) (reconcile.Result, error) {
		return newVAReconciler(c).Reconcile(ctx, reconcile.Request{NamespacedName: key})
	}

	setPersistence := func(key types.NamespacedName, enabled bool) {
		agg := &v1alpha1.VectorAggregator{}
		Expect(k8sClient.Get(ctx, key, agg)).To(Succeed())
		agg.Spec.Persistence.Enabled = enabled
		Expect(k8sClient.Update(ctx, agg)).To(Succeed())
	}

	aggregatorStatus := func(key types.NamespacedName) v1alpha1.VectorCommonStatus {
		agg := &v1alpha1.VectorAggregator{}
		Expect(k8sClient.Get(ctx, key, agg)).To(Succeed())
		return agg.Status.VectorCommonStatus
	}

	serviceNames := func(ns string) []string {
		list := &corev1.ServiceList{}
		Expect(k8sClient.List(ctx, list, client.InNamespace(ns))).To(Succeed())
		names := make([]string, 0, len(list.Items))
		for _, svc := range list.Items {
			names = append(names, svc.Name)
		}
		return names
	}

	getService := func(ns, name string) *corev1.Service {
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, svc)).To(Succeed())
		return svc
	}

	// objectVersions returns the resourceVersion of the aggregator and of every object of a kind it writes, in its namespace or cluster wide.
	objectVersions := func(key types.NamespacedName) map[string]string {
		agg := &v1alpha1.VectorAggregator{}
		Expect(k8sClient.Get(ctx, key, agg)).To(Succeed())
		versions := map[string]string{"aggregator": agg.ResourceVersion}
		add := func(list client.ObjectList, opts ...client.ListOption) {
			Expect(k8sClient.List(ctx, list, opts...)).To(Succeed())
			Expect(meta.EachListItem(list, func(o runtime.Object) error {
				obj := o.(client.Object)
				versions[fmt.Sprintf("%T %s", obj, obj.GetName())] = obj.GetResourceVersion()
				return nil
			})).To(Succeed())
		}
		for _, list := range []client.ObjectList{
			&corev1.ServiceList{}, &corev1.SecretList{}, &corev1.ConfigMapList{}, &corev1.ServiceAccountList{},
			&appsv1.DeploymentList{}, &appsv1.StatefulSetList{},
			&autoscalingv2.HorizontalPodAutoscalerList{}, &policyv1.PodDisruptionBudgetList{},
		} {
			add(list, client.InNamespace(key.Namespace))
		}
		add(&rbacv1.ClusterRoleList{})
		add(&rbacv1.ClusterRoleBindingList{})
		return versions
	}

	// staleCache returns a client whose Service list still shows svc as it was read.
	staleCache := func(svc *corev1.Service) client.Client {
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		return interceptor.NewClient(watchClient, interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if err := c.List(ctx, list, opts...); err != nil {
					return err
				}
				if svcs, ok := list.(*corev1.ServiceList); ok {
					for i := range svcs.Items {
						if svcs.Items[i].Name == svc.Name {
							svcs.Items[i] = *svc.DeepCopy()
						}
					}
				}
				return nil
			},
		})
	}

	// The labels the aggregator selects its own Services by.
	aggregatorServiceLabels := func(aggName string) map[string]string {
		return map[string]string{
			k8s.ManagedByLabelKey: "vector-operator",
			k8s.NameLabelKey:      "vector",
			k8s.ComponentLabelKey: "Aggregator",
			k8s.InstanceLabelKey:  aggName,
		}
	}

	It("skips a pipeline Service with a too long name and reconciles the rest of a VectorAggregator", func() {
		ns := "svc-skip-long"
		aggName := "observability"
		createNamespace(ns)

		longPipeline := vectorPipeline(ns, longPipelineName, `{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`)
		createValidPipeline(longPipeline)
		createValidPipeline(vectorPipeline(ns, "web", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`))
		createValidPipeline(vectorPipeline(ns, "events", `{"in": {"type": "kubernetes_events"}}`))

		// Services from an earlier reconcile: one for a pipeline that is gone now, and one with a port the web pipeline no longer uses.
		for name, port := range map[string]int32{aggName + "-aggregator-gone": 7000, aggName + "-aggregator-web": 9999} {
			Expect(k8sClient.Create(ctx, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: aggregatorServiceLabels(aggName)},
				Spec: corev1.ServiceSpec{
					Selector: aggregatorServiceLabels(aggName),
					Ports:    []corev1.ServicePort{{Name: "old", Port: port, TargetPort: intstr.FromInt32(port)}},
				},
			})).To(Succeed())
		}

		key := createVectorAggregator(ns, aggName, v1alpha1.VectorAggregatorCommon{
			VectorCommon:        v1alpha1.VectorCommon{Api: v1alpha1.ApiSpec{Enabled: true}},
			Autoscaling:         v1alpha1.VectorAggregatorAutoscaling{Enabled: true, MaxReplicas: 3},
			PodDisruptionBudget: v1alpha1.PodDisruptionBudget{Enabled: true},
		})
		Expect(reconcileVA(k8sClient, key)).To(Equal(retry), "a rejected Service must not fail the reconcile, and is retried later")

		longServiceName := aggName + "-aggregator-" + longPipelineName
		Expect(serviceNames(ns)).To(ConsistOf(
			aggName+"-aggregator",
			aggName+"-aggregator-web",
			aggName+"-aggregator-events",
			aggName+"-event-collector",
		), "the other Services are written, the stale one is deleted, the rejected one does not exist")
		Expect(getService(ns, aggName+"-aggregator-web").Spec.Ports).To(ConsistOf(HaveField("Port", int32(9000))))

		By("running the steps after the Service step")
		objKey := types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator"}
		Expect(k8sClient.Get(ctx, objKey, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed())
		Expect(k8sClient.Get(ctx, objKey, &policyv1.PodDisruptionBudget{})).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-event-collector"}, &appsv1.Deployment{})).To(Succeed())

		By("reporting the Service in the aggregator status")
		status := aggregatorStatus(key)
		Expect(status.ConfigCheckResult).To(HaveValue(BeTrue()), "the config is valid and published")
		Expect(status.LastAppliedConfigHash).NotTo(BeNil())
		Expect(status.Reason).To(HaveValue(SatisfyAll(
			HavePrefix(fmt.Sprintf("Service %s is skipped: ", longServiceName)),
			ContainSubstring("must be no more than 63 characters"),
		)))

		got := &v1alpha1.VectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(longPipeline), got)).To(Succeed())
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "the pipeline is not invalidated")

		By("changing nothing on a retry while the Service is still rejected")
		before := objectVersions(key)
		Expect(reconcileVA(k8sClient, key)).To(Equal(retry))
		Expect(objectVersions(key)).To(Equal(before))

		By("retrying also when reconciled through a cluster wide object")
		Expect(newVAReconciler(k8sClient).Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: aggName}})).To(Equal(retry))

		By("clearing the reason once the pipeline is gone")
		Expect(k8sClient.Delete(ctx, longPipeline)).To(Succeed())
		Expect(reconcileVA(k8sClient, key)).To(BeZero(), "nothing is skipped, so nothing is retried")
		Expect(aggregatorStatus(key).Reason).To(BeNil())
	})

	It("skips a pipeline Service with a too long name on a ClusterVectorAggregator", func() {
		ns := "svc-skip-cluster"
		aggName := "observability-cluster"
		longName := "payments-ingest-production-eu-west-1"
		createNamespace(ns)

		selector := map[string]string{"svc-skip-test": "cluster"}
		clusterPipeline := func(name, sources string) *v1alpha1.ClusterVectorPipeline {
			return &v1alpha1.ClusterVectorPipeline{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: selector}, Spec: pipelineSpec(sources)}
		}
		longPipeline := clusterPipeline(longName, `{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`)
		createValidPipeline(longPipeline)
		createValidPipeline(clusterPipeline("svc-skip-web", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`))

		agg := &v1alpha1.ClusterVectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: aggName},
			Spec: v1alpha1.ClusterVectorAggregatorSpec{
				ResourceNamespace: ns,
				VectorAggregatorCommon: v1alpha1.VectorAggregatorCommon{
					Selector:     &v1alpha1.VectorSelectorSpec{MatchLabels: selector},
					VectorCommon: v1alpha1.VectorCommon{ConfigCheck: v1alpha1.ConfigCheck{Disabled: true}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agg)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agg) })

		Expect(newCVAReconciler(k8sClient).Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(agg)})).
			To(Equal(retry), "a rejected Service must not fail the reconcile, and is retried later")

		web := getService(ns, aggName+"-aggregator-svc-skip-web")
		Expect(web.OwnerReferences).To(ContainElement(SatisfyAll(
			HaveField("Kind", "ClusterVectorAggregator"),
			HaveField("Name", aggName),
		)), "the Service is in spec.resourceNamespace and owned by the ClusterVectorAggregator")

		got := &v1alpha1.ClusterVectorAggregator{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agg), got)).To(Succeed())
		Expect(got.Status.ConfigCheckResult).To(HaveValue(BeTrue()))
		Expect(got.Status.Reason).To(HaveValue(SatisfyAll(
			HavePrefix(fmt.Sprintf("Service %s-aggregator-%s is skipped: ", aggName, longName)),
			ContainSubstring("must be no more than 63 characters"),
		)))

		gotPipeline := &v1alpha1.ClusterVectorPipeline{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(longPipeline), gotPipeline)).To(Succeed())
		Expect(gotPipeline.Status.ConfigCheckResult).To(HaveValue(BeTrue()), "the pipeline is not invalidated")

		By("clearing the reason and not retrying once the pipeline is gone")
		Expect(k8sClient.Delete(ctx, longPipeline)).To(Succeed())
		Expect(newCVAReconciler(k8sClient).Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(agg)})).To(BeZero(), "nothing is skipped, so nothing is retried")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agg), got)).To(Succeed())
		Expect(got.Status.Reason).To(BeNil())
	})

	It("keeps an existing pipeline Service whose update the API server rejects", func() {
		ns := "svc-skip-update"
		aggName := "svc-update"
		createNamespace(ns)
		web := vectorPipeline(ns, "web", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`)
		createValidPipeline(web)

		key := createVectorAggregator(ns, aggName, v1alpha1.VectorAggregatorCommon{})
		Expect(reconcileVA(k8sClient, key)).To(BeZero())
		written := getService(ns, aggName+"-aggregator-web")

		By("changing the pipeline so that the update of its Service is invalid")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(web), web)).To(Succeed())
		web.Spec = pipelineSpec(`{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:0"}}`)
		Expect(k8sClient.Update(ctx, web)).To(Succeed())
		Expect(reconcileVA(k8sClient, key)).To(Equal(retry))

		kept := getService(ns, written.Name)
		Expect(kept.UID).To(Equal(written.UID), "the Service is not deleted as stale")
		Expect(kept.Spec.Ports).To(ConsistOf(HaveField("Port", int32(9000))))
		Expect(aggregatorStatus(key).Reason).To(HaveValue(SatisfyAll(
			HavePrefix(fmt.Sprintf("Service %s is skipped: ", written.Name)),
			ContainSubstring("must be between 1 and 65535"),
		)))
	})

	expectHeadless := func(svc *corev1.Service) {
		Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		Expect(svc.Spec.Ports).To(BeEmpty())
		Expect(svc.Spec.PublishNotReadyAddresses).To(BeTrue())
	}

	headlessReason := func(aggName string) string {
		return fmt.Sprintf("Service %s-aggregator-headless is skipped: with persistence enabled, this name is used by the headless Service of the StatefulSet", aggName)
	}

	It("keeps the headless Service when a pipeline Service has its name", func() {
		ns := "svc-skip-headless"
		aggName := "svc-fresh"
		createNamespace(ns)
		createValidPipeline(vectorPipeline(ns, "headless", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`))
		createValidPipeline(vectorPipeline(ns, "web", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9001"}}`))

		key := createVectorAggregator(ns, aggName, v1alpha1.VectorAggregatorCommon{
			Persistence: v1alpha1.VectorAggregatorPersistence{Enabled: true},
		})
		Expect(reconcileVA(k8sClient, key)).To(Equal(retry))

		headless := getService(ns, aggName+"-aggregator-headless")
		expectHeadless(headless)
		Expect(serviceNames(ns)).To(ConsistOf(aggName+"-aggregator-headless", aggName+"-aggregator-web"))
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator"}, sts)).To(Succeed())
		Expect(sts.Spec.ServiceName).To(Equal(headless.Name))
		Expect(aggregatorStatus(key).Reason).To(HaveValue(Equal(headlessReason(aggName))))

		By("not writing the headless Service again on the next reconcile")
		Expect(reconcileVA(k8sClient, key)).To(Equal(retry))
		Expect(getService(ns, headless.Name).ResourceVersion).To(Equal(headless.ResourceVersion))
	})

	It("replaces a pipeline Service with the headless Service when persistence is enabled later", func() {
		ns := "svc-skip-late"
		aggName := "svc-late"
		createNamespace(ns)
		createValidPipeline(vectorPipeline(ns, "headless", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`))

		key := createVectorAggregator(ns, aggName, v1alpha1.VectorAggregatorCommon{})
		Expect(reconcileVA(k8sClient, key)).To(BeZero())
		pipelineService := getService(ns, aggName+"-aggregator-headless")
		Expect(pipelineService.Spec.ClusterIP).NotTo(Equal(corev1.ClusterIPNone), "without persistence the pipeline Service gets the name")
		Expect(pipelineService.Spec.Ports).To(HaveLen(1))
		Expect(aggregatorStatus(key).Reason).To(BeNil())

		By("enabling persistence")
		setPersistence(key, true)
		Expect(reconcileVA(k8sClient, key)).To(Equal(retry))

		headless := getService(ns, aggName+"-aggregator-headless")
		Expect(headless.UID).NotTo(Equal(pipelineService.UID), "a cluster IP cannot be removed in place")
		expectHeadless(headless)
		workload := types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator"}
		Expect(k8sClient.Get(ctx, workload, &appsv1.StatefulSet{})).To(Succeed())
		Expect(api_errors.IsNotFound(k8sClient.Get(ctx, workload, &appsv1.Deployment{}))).To(BeTrue())
		Expect(aggregatorStatus(key).Reason).To(HaveValue(Equal(headlessReason(aggName))))

		By("not deleting a newer Service than the one it read")
		_, err := reconcileVA(staleCache(pipelineService), key)
		Expect(api_errors.IsConflict(err)).To(BeTrue(), "the delete precondition must fail, got %v", err)
		Expect(getService(ns, headless.Name).UID).To(Equal(headless.UID))
	})

	It("replaces the headless Service with a pipeline Service when persistence is disabled", func() {
		ns := "svc-skip-off"
		aggName := "svc-off"
		createNamespace(ns)
		createValidPipeline(vectorPipeline(ns, "headless", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`))

		key := createVectorAggregator(ns, aggName, v1alpha1.VectorAggregatorCommon{
			Persistence: v1alpha1.VectorAggregatorPersistence{Enabled: true},
		})
		Expect(reconcileVA(k8sClient, key)).To(Equal(retry))
		headless := getService(ns, aggName+"-aggregator-headless")
		expectHeadless(headless)

		By("disabling persistence")
		setPersistence(key, false)
		Expect(reconcileVA(k8sClient, key)).To(BeZero())

		pipelineService := getService(ns, headless.Name)
		Expect(pipelineService.UID).NotTo(Equal(headless.UID), "a cluster IP cannot be added in place")
		Expect(pipelineService.Spec.ClusterIP).NotTo(Equal(corev1.ClusterIPNone))
		Expect(pipelineService.Spec.Ports).To(ConsistOf(HaveField("Port", int32(9000))))
		Expect(pipelineService.Spec.PublishNotReadyAddresses).To(BeFalse())
		workload := types.NamespacedName{Namespace: ns, Name: aggName + "-aggregator"}
		Expect(k8sClient.Get(ctx, workload, &appsv1.Deployment{})).To(Succeed())
		Expect(api_errors.IsNotFound(k8sClient.Get(ctx, workload, &appsv1.StatefulSet{}))).To(BeTrue())
		Expect(aggregatorStatus(key).Reason).To(BeNil())

		By("not deleting a newer Service than the one it read")
		_, err := reconcileVA(staleCache(headless), key)
		Expect(api_errors.IsConflict(err)).To(BeTrue(), "the delete precondition must fail, got %v", err)
		Expect(getService(ns, headless.Name).UID).To(Equal(pipelineService.UID))
	})

	It("keeps a headless Service with ports under the headless name when persistence is off", func() {
		ns := "svc-skip-user-headless"
		aggName := "svc-user"
		createNamespace(ns)
		createValidPipeline(vectorPipeline(ns, "headless", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`))
		// Not the governing Service of a StatefulSet: it has a port, like a pipeline Service a user made headless.
		Expect(k8sClient.Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: aggName + "-aggregator-headless", Namespace: ns, Labels: aggregatorServiceLabels(aggName)},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Selector:  aggregatorServiceLabels(aggName),
				Ports:     []corev1.ServicePort{{Name: "in", Port: 9000, TargetPort: intstr.FromInt32(9000)}},
			},
		})).To(Succeed())
		before := getService(ns, aggName+"-aggregator-headless")

		key := createVectorAggregator(ns, aggName, v1alpha1.VectorAggregatorCommon{})
		Expect(reconcileVA(k8sClient, key)).To(BeZero())

		after := getService(ns, before.Name)
		Expect(after.UID).To(Equal(before.UID), "only the governing Service of a StatefulSet is replaced")
		Expect(after.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
	})

	It("writes the same Services and status as before when no Service is rejected", func() {
		ns := "svc-skip-plain"
		aggName := "svc-plain"
		createNamespace(ns)
		createValidPipeline(vectorPipeline(ns, "web", `{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`))
		createValidPipeline(vectorPipeline(ns, "api-in", `{"in": {"type": "http_server", "address": "0.0.0.0:8080"}}`))
		stale := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: aggName + "-aggregator-gone", Namespace: ns, Labels: aggregatorServiceLabels(aggName)},
			Spec: corev1.ServiceSpec{
				Selector: aggregatorServiceLabels(aggName),
				Ports:    []corev1.ServicePort{{Name: "old", Port: 7000, TargetPort: intstr.FromInt32(7000)}},
			},
		}
		Expect(k8sClient.Create(ctx, stale)).To(Succeed())
		// A user's headless Service with the name of a pipeline Service is written over in place, so it stays headless.
		userHeadless := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: aggName + "-aggregator-web", Namespace: ns},
			Spec:       corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, Selector: map[string]string{"app": "mine"}},
		}
		Expect(k8sClient.Create(ctx, userHeadless)).To(Succeed())

		key := createVectorAggregator(ns, aggName, v1alpha1.VectorAggregatorCommon{
			VectorCommon: v1alpha1.VectorCommon{Api: v1alpha1.ApiSpec{Enabled: true}},
			Persistence:  v1alpha1.VectorAggregatorPersistence{Enabled: true},
		})

		// Writes to Services and to the aggregator status; without a rejected Service they must stay exactly these.
		var writes []string
		record := func(verb string, obj client.Object) {
			switch obj.(type) {
			case *corev1.Service, *v1alpha1.VectorAggregator:
				writes = append(writes, fmt.Sprintf("%s %T %s", verb, obj, obj.GetName()))
			}
		}
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		recording := interceptor.NewClient(watchClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				record("create", obj)
				return c.Create(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				record("update", obj)
				return c.Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				record("patch", obj)
				return c.Patch(ctx, obj, patch, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				record("delete", obj)
				return c.Delete(ctx, obj, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				record("update "+subResourceName, obj)
				return c.SubResource(subResourceName).Update(ctx, obj, opts...)
			},
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				record("patch "+subResourceName, obj)
				return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		})
		round := func() []string {
			writes = nil
			Expect(reconcileVA(recording, key)).To(BeZero(), "nothing is skipped, so nothing is retried")
			slices.Sort(writes)
			return writes
		}

		Expect(round()).To(Equal([]string{
			"create *v1.Service svc-plain-aggregator",
			"create *v1.Service svc-plain-aggregator-api-in",
			"create *v1.Service svc-plain-aggregator-headless",
			"delete *v1.Service svc-plain-aggregator-gone",
			"delete *v1.Service svc-plain-event-collector",
			"patch status *v1alpha1.VectorAggregator svc-plain",
			"patch status *v1alpha1.VectorAggregator svc-plain",
			"update *v1.Service svc-plain-aggregator-web",
		}), "the first reconcile")
		Expect(round()).To(Equal([]string{
			"delete *v1.Service svc-plain-event-collector",
			"patch status *v1alpha1.VectorAggregator svc-plain",
			"update *v1.Service svc-plain-aggregator",
			"update *v1.Service svc-plain-aggregator-api-in",
			"update *v1.Service svc-plain-aggregator-headless",
			"update *v1.Service svc-plain-aggregator-web",
		}), "a steady state reconcile")

		Expect(serviceNames(ns)).To(ConsistOf(
			aggName+"-aggregator", aggName+"-aggregator-api-in", aggName+"-aggregator-headless", aggName+"-aggregator-web",
		))
		expectHeadless(getService(ns, aggName+"-aggregator-headless"))
		web := getService(ns, userHeadless.Name)
		Expect(web.UID).To(Equal(userHeadless.UID))
		Expect(web.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		Expect(web.Spec.Ports).To(ConsistOf(HaveField("Port", int32(9000))))
		status := aggregatorStatus(key)
		Expect(status.ConfigCheckResult).To(HaveValue(BeTrue()))
		Expect(status.Reason).To(BeNil())
	})
})

// A skipped Service brings the requeue forward to skippedServiceRetryDelay, but never delays a sooner one.
func TestWithSkippedServiceRetry(t *testing.T) {
	g := NewWithT(t)
	for _, tc := range []struct {
		requeueAfter time.Duration
		skipped      bool
		want         time.Duration
	}{
		{0, false, 0},
		{30 * time.Second, false, 30 * time.Second},
		{0, true, skippedServiceRetryDelay},
		{30 * time.Second, true, 30 * time.Second},
		{2 * skippedServiceRetryDelay, true, skippedServiceRetryDelay},
	} {
		g.Expect(withSkippedServiceRetry(tc.requeueAfter, tc.skipped)).To(Equal(tc.want), "requeueAfter %v, skipped %v", tc.requeueAfter, tc.skipped)
	}
}
