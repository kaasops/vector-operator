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
	"testing"

	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// The two temporary Secrets a check creates: the config, and - when pipelines reference
// secrets - the assets one, which holds their plaintext values. Whatever happens to the
// check, neither may survive it.
func cleanupFixtures() (*corev1.Secret, *corev1.Secret) {
	cfg := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "configcheck-abc", Namespace: "test-ns"},
		Data:       map[string][]byte{"agent.json": []byte("{}")},
	}
	assets := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "configcheck-secret-assets-abc", Namespace: "test-ns"},
		Data:       map[string][]byte{"team_a_es_password": []byte("hunter2")},
	}
	return cfg, assets
}

func cleanupScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return s
}

func secretGone(t *testing.T, c client.Client, name string) bool {
	t.Helper()
	err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "test-ns"}, &corev1.Secret{})
	return api_errors.IsNotFound(err)
}

// A cache scoped by --watch-name does not hold configcheck objects at all: the selector
// is the operator's own app.kubernetes.io/name, while configcheck objects carry
// vector-configcheck. A cleanup that looked the Secret up through the manager's client
// first therefore got NotFound for a Secret that really exists and skipped the delete,
// leaving the plaintext assets copy on the cluster until the next operator start swept
// it. Deleting straight by name has nothing to be blinded by.
func TestCleanupDeletesDespiteCacheNotSeeingTheSecrets(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupScheme(t)
	cfg, assets := cleanupFixtures()

	var deleted []string
	blindCache := interceptor.Funcs{
		// Every Secret read answers NotFound, exactly as a --watch-name-scoped cache
		// does for configcheck objects.
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Secret); ok {
				return api_errors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
		// Deletes are recorded rather than verified by reading back: a read-back would
		// go through the same blinded Get and report NotFound whether or not anything
		// was actually deleted.
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if s, ok := obj.(*corev1.Secret); ok {
				deleted = append(deleted, s.Name)
			}
			return c.Delete(ctx, obj, opts...)
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cfg.DeepCopy(), assets.DeepCopy()).
		WithInterceptorFuncs(blindCache).Build()
	cc := &ConfigCheck{Client: cl}

	if err := cc.cleanup(ctx, cfg, assets); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	for _, name := range []string{cfg.Name, assets.Name} {
		found := false
		for _, d := range deleted {
			if d == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("cleanup never issued a delete for %s (deleted: %v)", name, deleted)
		}
	}
}

// The deferred cleanup runs on the way out of Run, and the reconcile context may already
// be cancelled by then - which is precisely the interrupted case where the Secrets are
// most likely to be left behind. Cleanup must therefore not delete through the caller's
// context.
func TestCleanupRunsWithCancelledContext(t *testing.T) {
	scheme := cleanupScheme(t)
	cfg, assets := cleanupFixtures()

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cfg.DeepCopy(), assets.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				// The fake client ignores context state, so model a real client:
				// a cancelled context fails the call before it reaches the server.
				if err := ctx.Err(); err != nil {
					return err
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()
	cc := &ConfigCheck{Client: cl}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := cc.cleanup(ctx, cfg, assets); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, name := range []string{cfg.Name, assets.Name} {
		if !secretGone(t, cl, name) {
			t.Fatalf("secret %s survived cleanup under a cancelled context", name)
		}
	}
}

// One failing delete must not strand the other Secret - the assets copy is the one
// holding plaintext credentials.
func TestCleanupContinuesAfterAFailedDelete(t *testing.T) {
	ctx := context.Background()
	scheme := cleanupScheme(t)
	cfg, assets := cleanupFixtures()

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cfg.DeepCopy(), assets.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if s, ok := obj.(*corev1.Secret); ok && s.Name == cfg.Name {
					return api_errors.NewInternalError(context.DeadlineExceeded)
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()
	cc := &ConfigCheck{Client: cl}

	err := cc.cleanup(ctx, cfg, assets)
	if err == nil {
		t.Fatal("expected the failed delete to be reported")
	}
	if !secretGone(t, cl, assets.Name) {
		t.Fatal("the plaintext assets secret must still be deleted when the config secret's delete fails")
	}
}
