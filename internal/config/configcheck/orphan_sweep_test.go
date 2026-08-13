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
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

func configCheckSecret(name string, created time.Time, labeled bool) *corev1.Secret {
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		Namespace:         "vector",
		CreationTimestamp: metav1.NewTime(created),
	}}
	if labeled {
		s.Labels = map[string]string{
			k8s.ManagedByLabelKey: "vector-operator",
			k8s.NameLabelKey:      "vector-configcheck",
			k8s.ComponentLabelKey: "ConfigCheck",
		}
	}
	return s
}

// A crash between creating configcheck Secrets and the process-local deferred
// cleanup leaves them orphaned forever (nothing owns the root Secret). The startup
// sweep must delete exactly the labeled Secrets from before this process started -
// never a concurrently running configcheck's (created after start), never an
// unrelated Secret.
func TestSweepOrphansDeletesOnlyStaleConfigCheckSecrets(t *testing.T) {
	start := time.Now()
	stale := configCheckSecret("configcheck-old", start.Add(-time.Hour), true)
	staleAssets := configCheckSecret("configcheck-secret-assets-old", start.Add(-time.Hour), true)
	fresh := configCheckSecret("configcheck-live", start.Add(time.Minute), true)
	unrelated := configCheckSecret("user-creds", start.Add(-time.Hour), false)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cl := crfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(stale, staleAssets, fresh, unrelated).Build()

	require.NoError(t, SweepOrphans(context.Background(), cl, cl, start))

	for _, gone := range []string{"configcheck-old", "configcheck-secret-assets-old"} {
		err := cl.Get(context.Background(), types.NamespacedName{Namespace: "vector", Name: gone}, &corev1.Secret{})
		require.True(t, api_errors.IsNotFound(err), "%s must be swept", gone)
	}
	for _, kept := range []string{"configcheck-live", "user-creds"} {
		require.NoError(t,
			cl.Get(context.Background(), types.NamespacedName{Namespace: "vector", Name: kept}, &corev1.Secret{}),
			"%s must survive the sweep", kept)
	}
}
