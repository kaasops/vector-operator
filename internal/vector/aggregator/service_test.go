package aggregator

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
)

// serviceTestController returns a controller for the aggregator "va" with the pipeline Services va-aggregator-web and va-aggregator-other; failCreate can fail a Service create.
func serviceTestController(g *WithT, common vectorv1alpha1.VectorAggregatorCommon, failCreate func(*corev1.Service) error) (*Controller, client.Client) {
	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())
	g.Expect(monitorv1.AddToScheme(s)).To(Succeed())

	seed := &vectorv1alpha1.VectorAggregator{
		ObjectMeta: metav1.ObjectMeta{Name: "va", Namespace: "default"},
		Spec:       vectorv1alpha1.VectorAggregatorSpec{VectorAggregatorCommon: common},
	}
	cl := crfake.NewClientBuilder().
		WithScheme(s).
		WithObjects(seed).
		WithStatusSubresource(&vectorv1alpha1.VectorAggregator{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if svc, ok := obj.(*corev1.Service); ok {
					if err := failCreate(svc); err != nil {
						return err
					}
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	va := &vectorv1alpha1.VectorAggregator{}
	g.Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(seed), va)).To(Succeed())

	cs := k8sfake.NewSimpleClientset()
	cs.Resources = []*metav1.APIResourceList{{
		GroupVersion: monitorv1.SchemeGroupVersion.String(),
		APIResources: []metav1.APIResource{{Name: "podmonitors", Kind: monitorv1.PodMonitorsKind}},
	}}

	ctrl := NewController(va, cl, cs)
	ctrl.APIReader = cl
	pipelines := []*vectorv1alpha1.VectorPipeline{
		{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}, Spec: vectorv1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9000"}}`)},
		}},
		{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"}, Spec: vectorv1alpha1.VectorPipelineSpec{
			Sources: &runtime.RawExtension{Raw: []byte(`{"in": {"type": "socket", "mode": "tcp", "address": "0.0.0.0:9001"}}`)},
		}},
	}
	cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{
		AggregatorName:  ctrl.Name,
		InternalMetrics: common.InternalMetrics,
	}, pipelines[0], pipelines[1])
	g.Expect(err).NotTo(HaveOccurred())
	ctrl.Config = cfg
	return ctrl, cl
}

// A Service write that fails for another reason than Invalid can work on the next try, so it still fails the round and the reconcile is retried.
func TestEnsureVectorAggregator_ServiceWriteErrorFailsTheRound(t *testing.T) {
	services := schema.GroupResource{Resource: "services"}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"Conflict", api_errors.NewConflict(services, "va-aggregator-web", errors.New("injected"))},
		{"Forbidden", api_errors.NewForbidden(services, "va-aggregator-web", errors.New("injected"))},
		{"InternalError", api_errors.NewInternalError(errors.New("injected"))},
		{"Timeout", api_errors.NewTimeoutError("injected", 1)},
		{"TooManyRequests", api_errors.NewTooManyRequests("injected", 1)},
		{"BadRequest", api_errors.NewBadRequest("injected")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctrl, cl := serviceTestController(g, vectorv1alpha1.VectorAggregatorCommon{
				Autoscaling: vectorv1alpha1.VectorAggregatorAutoscaling{Enabled: true, MaxReplicas: 3},
			}, func(svc *corev1.Service) error {
				if svc.Name == "va-aggregator-web" {
					return tc.err
				}
				return nil
			})

			g.Expect(ctrl.EnsureVectorAggregator(context.Background(), false)).To(MatchError(tc.err), "the error must fail the round")

			hpaErr := cl.Get(context.Background(), types.NamespacedName{Name: "va-aggregator", Namespace: "default"}, &autoscalingv2.HorizontalPodAutoscaler{})
			g.Expect(api_errors.IsNotFound(hpaErr)).To(BeTrue(), "the round stops at the Service step")
		})
	}
}

// A Service the API server rejects as invalid is skipped: the later steps still run, the status names the Service, and the skip is logged as an error.
func TestEnsureVectorAggregator_RejectedServiceDoesNotStopLaterSteps(t *testing.T) {
	g := NewWithT(t)
	var logs []string
	ctx := log.IntoContext(context.Background(), funcr.New(func(_, args string) { logs = append(logs, args) }, funcr.Options{}))

	invalid := api_errors.NewInvalid(schema.GroupKind{Kind: "Service"}, "va-aggregator-web", field.ErrorList{
		field.Invalid(field.NewPath("spec", "ports").Index(0).Child("port"), 0, "must be between 1 and 65535, inclusive"),
	})
	ctrl, cl := serviceTestController(g, vectorv1alpha1.VectorAggregatorCommon{
		VectorCommon: vectorv1alpha1.VectorCommon{InternalMetrics: true},
		Autoscaling:  vectorv1alpha1.VectorAggregatorAutoscaling{Enabled: true, MaxReplicas: 3},
	}, func(svc *corev1.Service) error {
		if svc.Name == "va-aggregator-web" {
			return invalid
		}
		return nil
	})

	g.Expect(ctrl.EnsureVectorAggregator(ctx, false)).To(Succeed())
	// funcr starts an info line with its level and an error line with the message, so the prefix proves the error level.
	g.Expect(logs).To(ContainElement(SatisfyAll(
		HavePrefix(`"msg"="Service is skipped" "error"="Service \"va-aggregator-web\" is invalid`),
		ContainSubstring(`"service"="va-aggregator-web"`),
	)), "the skip is logged at the error level")

	key := types.NamespacedName{Name: "va-aggregator", Namespace: "default"}
	g.Expect(cl.Get(ctx, types.NamespacedName{Name: "va-aggregator-other", Namespace: "default"}, &corev1.Service{})).To(Succeed())
	g.Expect(cl.Get(ctx, key, &monitorv1.PodMonitor{})).To(Succeed(), "the PodMonitor step runs")
	g.Expect(cl.Get(ctx, key, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed(), "the HPA step runs")

	cfgHash, globalHash := int64(1), int64(2)
	g.Expect(ctrl.SetSuccessStatus(ctx, &cfgHash, &globalHash, false)).To(Succeed())
	got := &vectorv1alpha1.VectorAggregator{}
	g.Expect(cl.Get(ctx, types.NamespacedName{Name: "va", Namespace: "default"}, got)).To(Succeed())
	g.Expect(got.Status.ConfigCheckResult).To(HaveValue(BeTrue()))
	g.Expect(got.Status.Reason).To(HaveValue(Equal(
		`Service va-aggregator-web is skipped: Service "va-aggregator-web" is invalid: spec.ports[0].port: Invalid value: 0: must be between 1 and 65535, inclusive`,
	)))
}

// The API Service and the headless Service of the StatefulSet are skipped like a pipeline Service when the API server rejects them.
func TestEnsureVectorAggregator_RejectedOwnServiceIsSkipped(t *testing.T) {
	for _, tc := range []struct {
		name    string
		service string
		common  vectorv1alpha1.VectorAggregatorCommon
	}{
		{"api", "va-aggregator", vectorv1alpha1.VectorAggregatorCommon{
			VectorCommon: vectorv1alpha1.VectorCommon{Api: vectorv1alpha1.ApiSpec{Enabled: true}},
			Autoscaling:  vectorv1alpha1.VectorAggregatorAutoscaling{Enabled: true, MaxReplicas: 3},
		}},
		{"headless", "va-aggregator-headless", vectorv1alpha1.VectorAggregatorCommon{
			Persistence: vectorv1alpha1.VectorAggregatorPersistence{Enabled: true},
			Autoscaling: vectorv1alpha1.VectorAggregatorAutoscaling{Enabled: true, MaxReplicas: 3},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			invalid := api_errors.NewInvalid(schema.GroupKind{Kind: "Service"}, tc.service, field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), tc.service, "rejected"),
			})
			ctrl, cl := serviceTestController(g, tc.common, func(svc *corev1.Service) error {
				if svc.Name == tc.service {
					return invalid
				}
				return nil
			})

			g.Expect(ctrl.EnsureVectorAggregator(context.Background(), false)).To(Succeed())
			g.Expect(ctrl.HasSkippedServices()).To(BeTrue())
			g.Expect(ctrl.skippedServicesReason()).To(HaveValue(HavePrefix("Service " + tc.service + " is skipped: ")))
			key := types.NamespacedName{Name: "va-aggregator", Namespace: "default"}
			g.Expect(cl.Get(context.Background(), key, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed(), "the HPA step runs")
		})
	}
}

// The reason is sorted by Service name, so it does not change between reconciles that skip the same Services.
func TestSkippedServicesReasonIsSorted(t *testing.T) {
	g := NewWithT(t)

	ctrl := &Controller{}
	for _, name := range []string{"svc-e", "svc-b", "svc-d", "svc-a", "svc-c"} {
		ctrl.skipService(name, "reason "+name)
	}
	want := "Service svc-a is skipped: reason svc-a; Service svc-b is skipped: reason svc-b; " +
		"Service svc-c is skipped: reason svc-c; Service svc-d is skipped: reason svc-d; Service svc-e is skipped: reason svc-e"
	for range 20 {
		g.Expect(ctrl.skippedServicesReason()).To(HaveValue(Equal(want)))
	}
}

// The skipped Services are found again on every round, so a Service written now is no longer reported, even by a controller that ran an earlier round.
func TestEnsureVectorAggregatorService_ForgetsEarlierRounds(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	rejected := true
	ctrl, _ := serviceTestController(g, vectorv1alpha1.VectorAggregatorCommon{}, func(svc *corev1.Service) error {
		if rejected && svc.Name == "va-aggregator-web" {
			return api_errors.NewInvalid(schema.GroupKind{Kind: "Service"}, svc.Name, field.ErrorList{
				field.TooLong(field.NewPath("metadata", "name"), svc.Name, 63),
			})
		}
		return nil
	})

	g.Expect(ctrl.ensureVectorAggregatorService(ctx)).To(Succeed())
	g.Expect(ctrl.skippedServicesReason()).NotTo(BeNil())

	rejected = false
	g.Expect(ctrl.ensureVectorAggregatorService(ctx)).To(Succeed())
	g.Expect(ctrl.skippedServicesReason()).To(BeNil())
}
