package vectoragent

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func newStatusTestClient(g *WithT, objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())
	return crfake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&vectorv1alpha1.Vector{}).
		WithObjects(objs...).
		Build()
}

// The reconcile reads the Vector, then holds it across configcheck and the DaemonSet
// write, so the status must still land after someone else has edited the CR.
func TestSetSuccessStatusStaleVector(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	reason := "config check failed"
	failed := false
	v := &vectorv1alpha1.Vector{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "vector"},
		Spec:       vectorv1alpha1.VectorSpec{Agent: &vectorv1alpha1.VectorAgent{}},
		Status: vectorv1alpha1.VectorStatus{
			VectorCommonStatus: vectorv1alpha1.VectorCommonStatus{
				ConfigCheckResult: &failed,
				Reason:            &reason,
			},
		},
	}
	cl := newStatusTestClient(g, v)
	key := client.ObjectKeyFromObject(v)

	stale := &vectorv1alpha1.Vector{}
	g.Expect(cl.Get(ctx, key, stale)).To(Succeed())

	edited := &vectorv1alpha1.Vector{}
	g.Expect(cl.Get(ctx, key, edited)).To(Succeed())
	edited.Annotations = map[string]string{"edited": "by-someone-else"}
	g.Expect(cl.Update(ctx, edited)).To(Succeed())

	ctrl := NewController(stale, cl, nil)
	cfgHash, globalHash := int64(1), int64(2)
	g.Expect(ctrl.SetSuccessStatus(ctx, &cfgHash, &globalHash)).To(Succeed())

	result := &vectorv1alpha1.Vector{}
	g.Expect(cl.Get(ctx, key, result)).To(Succeed())
	g.Expect(result.Status.ConfigCheckResult).To(HaveValue(BeTrue()))
	g.Expect(result.Status.Reason).To(BeNil(), "the previous failure reason must be cleared")
	g.Expect(result.Status.LastAppliedConfigHash).To(HaveValue(Equal(cfgHash)))
	g.Expect(result.Annotations).To(HaveKeyWithValue("edited", "by-someone-else"))
}

// The reason can be written by a reconcile the current one read the Vector before, so the
// success patch has to clear a reason that was never in hand.
func TestSetSuccessStatusClearsUnobservedReason(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	v := &vectorv1alpha1.Vector{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "vector"},
		Spec:       vectorv1alpha1.VectorSpec{Agent: &vectorv1alpha1.VectorAgent{}},
	}
	cl := newStatusTestClient(g, v)
	key := client.ObjectKeyFromObject(v)

	stale := &vectorv1alpha1.Vector{}
	g.Expect(cl.Get(ctx, key, stale)).To(Succeed())
	g.Expect(stale.Status.Reason).To(BeNil(), "the read must predate the failure")

	failing := NewController(stale.DeepCopy(), cl, nil)
	g.Expect(failing.SetFailedStatus(ctx, "config check failed")).To(Succeed())

	ctrl := NewController(stale, cl, nil)
	cfgHash, globalHash := int64(1), int64(2)
	g.Expect(ctrl.SetSuccessStatus(ctx, &cfgHash, &globalHash)).To(Succeed())

	result := &vectorv1alpha1.Vector{}
	g.Expect(cl.Get(ctx, key, result)).To(Succeed())
	g.Expect(result.Status.ConfigCheckResult).To(HaveValue(BeTrue()))
	g.Expect(result.Status.Reason).To(BeNil(), "a success must not keep a failure reason")
}
