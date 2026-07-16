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

package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNamespaceIsTerminating(t *testing.T) {
	ctx := context.Background()
	build := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().WithObjects(objs...).Build()
	}

	t.Run("active namespace", func(t *testing.T) {
		c := build(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}})
		got, err := NamespaceIsTerminating(ctx, c, "ns")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("active namespace reported terminating")
		}
	})

	t.Run("terminating namespace", func(t *testing.T) {
		now := metav1.Now()
		c := build(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:              "ns",
			DeletionTimestamp: &now,
			Finalizers:        []string{"kubernetes"},
		}})
		got, err := NamespaceIsTerminating(ctx, c, "ns")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatalf("terminating namespace reported active")
		}
	})

	t.Run("missing namespace treated as terminating", func(t *testing.T) {
		c := build()
		got, err := NamespaceIsTerminating(ctx, c, "ns")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatalf("missing namespace should be treated as terminating")
		}
	})
}
