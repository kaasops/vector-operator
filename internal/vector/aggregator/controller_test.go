package aggregator

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	policyv1 "k8s.io/api/policy/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
)

// newFakeClient returns a controller-runtime fake client with the schemes the
// aggregator manages, pre-populated with the given objects.
func newFakeClient(g *WithT, objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())
	return crfake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

// A rejected PodDisruptionBudget write (admission policy, quota, RBAC) must not
// keep the aggregator from getting its HPA and event collector.
func TestEnsureVectorAggregator_PDBWriteFailureDoesNotBlockHPA(t *testing.T) {
	g := NewWithT(t)

	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())

	pdbDenied := errors.New("pdb write denied")
	cl := crfake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*policyv1.PodDisruptionBudget); ok {
				return pdbDenied
			}
			return c.Create(ctx, obj, opts...)
		},
	}).Build()

	va := &vectorv1alpha1.VectorAggregator{
		ObjectMeta: metav1.ObjectMeta{Name: "va", Namespace: "default"},
		Spec: vectorv1alpha1.VectorAggregatorSpec{
			VectorAggregatorCommon: vectorv1alpha1.VectorAggregatorCommon{
				Autoscaling: vectorv1alpha1.VectorAggregatorAutoscaling{
					Enabled:     true,
					MinReplicas: ptr.To[int32](2),
					MaxReplicas: 3,
				},
				PodDisruptionBudget: vectorv1alpha1.PodDisruptionBudget{Enabled: true},
			},
		},
	}

	cs := k8sfake.NewSimpleClientset()
	cs.Resources = []*metav1.APIResourceList{{GroupVersion: "monitoring.coreos.com/v1"}}

	ctrl := NewController(va, cl, cs)
	ctrl.Config = &config.VectorConfig{}

	err := ctrl.EnsureVectorAggregator(context.Background())
	g.Expect(err).To(MatchError(pdbDenied), "the PDB failure itself must still be reported")

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: "va-aggregator", Namespace: "default"}, hpa)).
		To(Succeed(), "HPA must be ensured even when the PDB write fails")
}

// A workload left over from the previous persistence mode must be removed so its
// pods stop serving alongside the new workload.
func TestDeleteObsoleteWorkload_DeletesLeftover(t *testing.T) {
	g := NewWithT(t)

	stale := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "default"}}
	cl := newFakeClient(g, stale)

	va := &vectorv1alpha1.VectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "va", Namespace: "default"}}
	ctrl := NewController(va, cl, k8sfake.NewSimpleClientset())

	g.Expect(ctrl.deleteObsoleteWorkload(context.Background(), &appsv1.Deployment{})).To(Succeed())

	err := cl.Get(context.Background(), types.NamespacedName{Name: "va-aggregator", Namespace: "default"}, &appsv1.Deployment{})
	g.Expect(api_errors.IsNotFound(err)).To(BeTrue(), "the leftover workload must be deleted")
}

// In steady state the opposite workload does not exist, so the reconcile must not
// issue a DELETE that just comes back NotFound.
func TestDeleteObsoleteWorkload_SkipsDeleteWhenAbsent(t *testing.T) {
	g := NewWithT(t)

	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())

	deleteCalls := 0
	cl := crfake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			deleteCalls++
			return c.Delete(ctx, obj, opts...)
		},
	}).Build()

	va := &vectorv1alpha1.VectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "va", Namespace: "default"}}
	ctrl := NewController(va, cl, k8sfake.NewSimpleClientset())

	g.Expect(ctrl.deleteObsoleteWorkload(context.Background(), &appsv1.Deployment{})).To(Succeed())
	g.Expect(deleteCalls).To(Equal(0), "no DELETE should be issued when the workload is absent")
}

// The reason can be written by a reconcile the current one read the aggregator before, so
// the success patch has to clear a reason that was never in hand. Both kinds share the path.
func TestSetSuccessStatusClearsUnobservedReason(t *testing.T) {
	testCases := []struct {
		name  string
		seed  client.Object
		empty func() client.Object
		read  func(client.Object) *vectorv1alpha1.VectorCommonStatus
	}{
		{
			name:  "VectorAggregator",
			seed:  &vectorv1alpha1.VectorAggregator{ObjectMeta: metav1.ObjectMeta{Name: "va", Namespace: "default"}},
			empty: func() client.Object { return &vectorv1alpha1.VectorAggregator{} },
			read: func(o client.Object) *vectorv1alpha1.VectorCommonStatus {
				return &o.(*vectorv1alpha1.VectorAggregator).Status.VectorCommonStatus
			},
		},
		{
			name: "ClusterVectorAggregator",
			seed: &vectorv1alpha1.ClusterVectorAggregator{
				ObjectMeta: metav1.ObjectMeta{Name: "cva"},
				Spec: vectorv1alpha1.ClusterVectorAggregatorSpec{
					ResourceNamespace: "default",
				},
			},
			empty: func() client.Object { return &vectorv1alpha1.ClusterVectorAggregator{} },
			read: func(o client.Object) *vectorv1alpha1.VectorCommonStatus {
				return &o.(*vectorv1alpha1.ClusterVectorAggregator).Status.VectorCommonStatus
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.Background()

			s := runtime.NewScheme()
			g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
			g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())
			cl := crfake.NewClientBuilder().
				WithScheme(s).
				WithStatusSubresource(&vectorv1alpha1.VectorAggregator{}, &vectorv1alpha1.ClusterVectorAggregator{}).
				WithObjects(tc.seed).
				Build()
			key := client.ObjectKeyFromObject(tc.seed)

			stale := tc.empty()
			g.Expect(cl.Get(ctx, key, stale)).To(Succeed())
			g.Expect(tc.read(stale).Reason).To(BeNil(), "the read must predate the failure")

			failing := NewController(stale.DeepCopyObject().(client.Object), cl, k8sfake.NewSimpleClientset())
			g.Expect(failing.SetFailedStatus(ctx, "config check failed")).To(Succeed())

			ctrl := NewController(stale, cl, k8sfake.NewSimpleClientset())
			cfgHash, globalHash := int64(1), int64(2)
			g.Expect(ctrl.SetSuccessStatus(ctx, &cfgHash, &globalHash)).To(Succeed())

			result := tc.empty()
			g.Expect(cl.Get(ctx, key, result)).To(Succeed())
			g.Expect(tc.read(result).ConfigCheckResult).To(HaveValue(BeTrue()))
			g.Expect(tc.read(result).Reason).To(BeNil(), "a success must not keep a failure reason")
		})
	}
}
