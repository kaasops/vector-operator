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

package configcheck

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newCCWithNamespace(objs ...client.Object) *ConfigCheck {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &ConfigCheck{Client: c, Namespace: "target"}
}

// A ConfigCheck launched into a terminating (or already gone) namespace can never
// create its pod/secret — the API server rejects new content — so it must be skipped
// instead of blocking a reconcile worker until ConfigCheckTimeout.
func TestNamespaceIsTerminating(t *testing.T) {
	ctx := context.Background()

	t.Run("active namespace is not terminating", func(t *testing.T) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "target"}}
		terminating, err := newCCWithNamespace(ns).namespaceIsTerminating(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if terminating {
			t.Fatalf("active namespace reported as terminating")
		}
	})

	t.Run("namespace with deletion timestamp is terminating", func(t *testing.T) {
		now := metav1.Now()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:              "target",
			DeletionTimestamp: &now,
			Finalizers:        []string{"kubernetes"},
		}}
		terminating, err := newCCWithNamespace(ns).namespaceIsTerminating(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !terminating {
			t.Fatalf("terminating namespace reported as active")
		}
	})

	t.Run("missing namespace is treated as terminating", func(t *testing.T) {
		terminating, err := newCCWithNamespace().namespaceIsTerminating(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !terminating {
			t.Fatalf("missing namespace should be treated as terminating (skip)")
		}
	})
}

// Run must bail out early with ErrConfigcheckSkipped when the target namespace is
// terminating, before creating any pod/secret — otherwise it blocks a reconcile
// worker until ConfigCheckTimeout waiting for a pod the API server won't admit.
func TestRunSkipsTerminatingNamespace(t *testing.T) {
	now := metav1.Now()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:              "target",
		DeletionTimestamp: &now,
		Finalizers:        []string{"kubernetes"},
	}}
	cc := newCCWithNamespace(ns)
	cc.Name = "agent"
	cc.SecretAssets = make(map[string][]byte)

	_, err := cc.Run(context.Background())
	if !errors.Is(err, ErrConfigcheckSkipped) {
		t.Fatalf("want ErrConfigcheckSkipped, got %v", err)
	}
}

// The budget must cover the preparation calls, not just the wait for the result: the
// root Secret is already on the cluster while the pod is being created, and its age is
// what the orphan sweep judges. A preparation call that hangs must end the check on the
// deadline instead of holding the reconcile worker while that Secret ages past its
// window - and past the point where another operator process may sweep it.
func TestRunBudgetCoversPreparationCalls(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "target"}}

	hangOnPod := interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*corev1.Pod); ok {
				<-ctx.Done()
				return ctx.Err()
			}
			return c.Create(ctx, obj, opts...)
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).
		WithInterceptorFuncs(hangOnPod).Build()

	cc := &ConfigCheck{
		Client:             cl,
		Namespace:          "target",
		Name:               "agent",
		ConfigCheckTimeout: 200 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		_, err := cc.Run(context.Background())
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want the budget to end the check, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run outlived its budget while a preparation call hung")
	}
}
