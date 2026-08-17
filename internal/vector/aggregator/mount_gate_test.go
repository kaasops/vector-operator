package aggregator

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

const (
	gateAggregatorName = "va"
	gateWorkloadName   = "va-aggregator"
	gateAssetsName     = "va-aggregator-secret-assets"
)

// gateWorkloadPodSpec builds a pod template that either does or does not carry the
// operator's own secret-assets mount, so a spec can seed a workload in either state.
func gateWorkloadPodSpec(mounted bool) corev1.PodSpec {
	spec := corev1.PodSpec{Containers: []corev1.Container{{Name: "vector", Image: "vector"}}}
	if mounted {
		spec.Volumes = []corev1.Volume{{
			Name: k8s.SecretAssetsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: gateAssetsName},
			},
		}}
		spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
			Name:      k8s.SecretAssetsVolumeName,
			MountPath: config.SecretsMountPath,
		}}
	}
	return spec
}

func gateDeployment(mounted bool) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: gateWorkloadName, Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "vector"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "vector"}},
				Spec:       gateWorkloadPodSpec(mounted),
			},
		},
	}
}

func gateStatefulSet(mounted bool) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: gateWorkloadName, Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "vector"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "vector"}},
				Spec:       gateWorkloadPodSpec(mounted),
			},
		},
	}
}

// recordingClient wraps a fake client and appends a label for every write that
// matters to the secret-assets write order, so a spec can assert their sequence.
func recordingClient(g *WithT, order *[]string, objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())

	note := func(obj client.Object, verb string) {
		switch o := obj.(type) {
		case *corev1.Secret:
			switch o.Name {
			case gateAssetsName:
				*order = append(*order, "assets-"+verb)
			case gateWorkloadName:
				*order = append(*order, "config-"+verb)
			}
		case *appsv1.Deployment:
			if o.Name == gateWorkloadName {
				*order = append(*order, "workload-"+verb)
			}
		case *appsv1.StatefulSet:
			if o.Name == gateWorkloadName {
				*order = append(*order, "workload-"+verb)
			}
		}
	}

	return crfake.NewClientBuilder().WithScheme(s).WithObjects(objs...).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			note(obj, "write")
			return c.Create(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			note(obj, "write")
			return c.Update(ctx, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			note(obj, "delete")
			return c.Delete(ctx, obj, opts...)
		},
	}).Build()
}

func indexOf(order []string, entry string) int {
	for i, e := range order {
		if e == entry {
			return i
		}
	}
	return -1
}

// The mount gate reads both workload kinds, and the two directions it feeds need
// DIFFERENT answers out of that pair - see secretAssetsMountState's doc comment.
// Folding them with OR ("somebody has the mount") is right for the losing direction
// and wrong for the gaining one: with one kind mounted and the other not, the round
// that adds a secret would conclude the mount is already in place, take the ordinary
// order, and publish a config referencing the new key while the unmounted kind's pods
// have nothing to resolve it from.
//
// The mixed pair is reachable because ensureWorkload writes the new kind before
// deleting the old one, so a failed or not-yet-run delete leaves both alive with
// independent templates. Both mixed combinations are covered, in both persistence
// directions, because which kind is "current" is exactly what a single-kind or
// OR-folded gate gets away with.
func TestEnsureVectorAggregator_GainingMountWithMixedWorkloadKinds(t *testing.T) {
	cases := []struct {
		name        string
		persistence bool
		existing    []client.Object
	}{
		{
			name:        "deployment mounted, leftover statefulset not, persistence off",
			persistence: false,
			existing:    []client.Object{gateDeployment(true), gateStatefulSet(false)},
		},
		{
			name:        "deployment mounted, leftover statefulset not, persistence on",
			persistence: true,
			existing:    []client.Object{gateDeployment(true), gateStatefulSet(false)},
		},
		{
			name:        "statefulset mounted, leftover deployment not, persistence off",
			persistence: false,
			existing:    []client.Object{gateDeployment(false), gateStatefulSet(true)},
		},
		{
			name:        "statefulset mounted, leftover deployment not, persistence on",
			persistence: true,
			existing:    []client.Object{gateDeployment(false), gateStatefulSet(true)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			var order []string
			cl := recordingClient(g, &order, tc.existing...)

			va := &vectorv1alpha1.VectorAggregator{
				ObjectMeta: metav1.ObjectMeta{Name: gateAggregatorName, Namespace: "default"},
			}
			va.Spec.Persistence.Enabled = tc.persistence

			ctrl := NewController(va, cl, k8sfake.NewSimpleClientset())
			ctrl.APIReader = cl
			ctrl.Config = &config.VectorConfig{}
			ctrl.SecretAssets = map[string][]byte{"app_es_cert": []byte("v1")}

			g.Expect(ctrl.EnsureVectorAggregator(context.Background(), false)).To(Succeed())

			assets := indexOf(order, "assets-write")
			workload := indexOf(order, "workload-write")
			cfg := indexOf(order, "config-write")
			g.Expect(assets).To(BeNumerically(">=", 0))
			g.Expect(workload).To(BeNumerically(">=", 0))
			g.Expect(cfg).To(BeNumerically(">=", 0))

			g.Expect(assets).To(BeNumerically("<", workload), "assets must be staged before the template that mounts them")
			g.Expect(workload).To(BeNumerically("<", cfg),
				"one unmounted workload kind is still one workload whose pods cannot resolve the key, so this round must take the "+
					"mount-gaining order and fix the template BEFORE the config starts referencing it")
		})
	}
}

// The losing direction keeps the OR reading: the assets Secret must survive as long as
// ANY live workload still mounts it, so a mixed pair still takes the mount-losing
// order (config first, template next, assets deleted last).
func TestEnsureVectorAggregator_LosingMountWithMixedWorkloadKinds(t *testing.T) {
	cases := []struct {
		name        string
		persistence bool
		existing    []client.Object
	}{
		{
			name:        "leftover deployment still mounted, current statefulset not",
			persistence: true,
			existing:    []client.Object{gateDeployment(true), gateStatefulSet(false)},
		},
		{
			name:        "leftover statefulset still mounted, current deployment not",
			persistence: false,
			existing:    []client.Object{gateDeployment(false), gateStatefulSet(true)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			existing := append([]client.Object{}, tc.existing...)
			existing = append(existing, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: gateAssetsName, Namespace: "default"},
				Data:       map[string][]byte{"app_es_cert": []byte("v1")},
			})

			var order []string
			cl := recordingClient(g, &order, existing...)

			va := &vectorv1alpha1.VectorAggregator{
				ObjectMeta: metav1.ObjectMeta{Name: gateAggregatorName, Namespace: "default"},
			}
			va.Spec.Persistence.Enabled = tc.persistence

			ctrl := NewController(va, cl, k8sfake.NewSimpleClientset())
			ctrl.APIReader = cl
			ctrl.Config = &config.VectorConfig{}
			ctrl.SecretAssets = nil

			g.Expect(ctrl.EnsureVectorAggregator(context.Background(), false)).To(Succeed())

			cfg := indexOf(order, "config-write")
			workload := indexOf(order, "workload-write")
			assetsDelete := indexOf(order, "assets-delete")
			g.Expect(cfg).To(BeNumerically(">=", 0))
			g.Expect(workload).To(BeNumerically(">=", 0))
			g.Expect(assetsDelete).To(BeNumerically(">=", 0))

			g.Expect(cfg).To(BeNumerically("<", workload))
			g.Expect(workload).To(BeNumerically("<", assetsDelete),
				"the assets Secret may only be deleted after the template that mounted it is gone - and one still-mounted kind is enough "+
					"to require that order, whichever kind persistence currently selects")
		})
	}
}

// A fresh aggregator - no workload of either kind yet - must take the gaining order:
// "every existing workload has the mount" has to be false over an empty set, not
// vacuously true.
func TestEnsureVectorAggregator_FirstEverReconcileTakesGainingOrder(t *testing.T) {
	g := NewWithT(t)

	var order []string
	cl := recordingClient(g, &order)

	va := &vectorv1alpha1.VectorAggregator{
		ObjectMeta: metav1.ObjectMeta{Name: gateAggregatorName, Namespace: "default"},
	}
	ctrl := NewController(va, cl, k8sfake.NewSimpleClientset())
	ctrl.APIReader = cl
	ctrl.Config = &config.VectorConfig{}
	ctrl.SecretAssets = map[string][]byte{"app_es_cert": []byte("v1")}

	g.Expect(ctrl.EnsureVectorAggregator(context.Background(), false)).To(Succeed())

	assets := indexOf(order, "assets-write")
	workload := indexOf(order, "workload-write")
	cfg := indexOf(order, "config-write")
	g.Expect(assets).To(BeNumerically("<", workload))
	g.Expect(workload).To(BeNumerically("<", cfg))
}

// deleteObsoleteWorkload used to preflight with a CACHED Get and return early on
// NotFound, which let a lagging informer decide a safety step: a leftover the API
// server still has, but the cache has not observed, was reported gone, no DELETE was
// sent, and the round carried on deleting the assets Secret out from under that
// leftover's still-running, still-mounting pods. The guard now lives at the caller and
// is fed by the round's uncached snapshot, so this spec hands the controller two
// readers that DISAGREE - a cached client that answers NotFound for the leftover
// Deployment, and an APIReader that sees the real one - and asserts the decision
// followed the second.
func TestEnsureVectorAggregator_ObsoleteWorkloadDeleteIgnoresCachedNotFound(t *testing.T) {
	g := NewWithT(t)

	s := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	g.Expect(vectorv1alpha1.AddToScheme(s)).To(Succeed())

	leftover := gateDeployment(true)
	assets := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: gateAssetsName, Namespace: "default"},
		Data:       map[string][]byte{"app_es_cert": []byte("v1")},
	}

	var order []string
	truthfulReader := crfake.NewClientBuilder().WithScheme(s).WithObjects(leftover.DeepCopy(), assets.DeepCopy()).Build()

	staleCachedClient := crfake.NewClientBuilder().WithScheme(s).WithObjects(leftover, assets).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*appsv1.Deployment); ok && key.Name == gateWorkloadName {
				return api_errors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if sts, ok := obj.(*appsv1.StatefulSet); ok && sts.Name == gateWorkloadName {
				order = append(order, "workload-write")
			}
			return c.Create(ctx, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			switch o := obj.(type) {
			case *appsv1.Deployment:
				if o.Name == gateWorkloadName {
					order = append(order, "leftover-delete")
				}
			case *corev1.Secret:
				if o.Name == gateAssetsName {
					order = append(order, "assets-delete")
				}
			}
			return c.Delete(ctx, obj, opts...)
		},
	}).Build()

	va := &vectorv1alpha1.VectorAggregator{
		ObjectMeta: metav1.ObjectMeta{Name: gateAggregatorName, Namespace: "default"},
	}
	va.Spec.Persistence.Enabled = true

	ctrl := NewController(va, staleCachedClient, k8sfake.NewSimpleClientset())
	ctrl.APIReader = truthfulReader
	ctrl.Config = &config.VectorConfig{}
	ctrl.SecretAssets = nil

	g.Expect(ctrl.EnsureVectorAggregator(context.Background(), false)).To(Succeed())

	err := staleCachedClient.Get(context.Background(), client.ObjectKey{Name: gateWorkloadName, Namespace: "default"}, &appsv1.Deployment{})
	g.Expect(api_errors.IsNotFound(err)).To(BeTrue(),
		"the leftover Deployment must actually be deleted - a cached NotFound must not be allowed to skip the DELETE and leave it "+
			"running while the assets Secret it mounts is removed")

	leftoverDelete := indexOf(order, "leftover-delete")
	assetsDelete := indexOf(order, "assets-delete")
	g.Expect(leftoverDelete).To(BeNumerically(">=", 0))
	g.Expect(assetsDelete).To(BeNumerically(">=", 0))
	g.Expect(leftoverDelete).To(BeNumerically("<", assetsDelete),
		"and it must be gone before the assets Secret it mounted is deleted, not after")
}

// And a fully converged workload - the current kind mounted, no leftover of the other
// - must stay on the ordinary order, so the fix does not turn every steady-state
// round into a template-first write.
func TestEnsureVectorAggregator_ConvergedWorkloadKeepsOrdinaryOrder(t *testing.T) {
	g := NewWithT(t)

	var order []string
	cl := recordingClient(g, &order, gateDeployment(true))

	va := &vectorv1alpha1.VectorAggregator{
		ObjectMeta: metav1.ObjectMeta{Name: gateAggregatorName, Namespace: "default"},
	}
	ctrl := NewController(va, cl, k8sfake.NewSimpleClientset())
	ctrl.APIReader = cl
	ctrl.Config = &config.VectorConfig{}
	ctrl.SecretAssets = map[string][]byte{"app_es_cert": []byte("v1")}

	g.Expect(ctrl.EnsureVectorAggregator(context.Background(), false)).To(Succeed())

	assets := indexOf(order, "assets-write")
	cfg := indexOf(order, "config-write")
	workload := indexOf(order, "workload-write")
	g.Expect(assets).To(BeNumerically("<", cfg))
	g.Expect(cfg).To(BeNumerically("<", workload), "nothing is crossing the mount boundary, so the template write stays last")
}
