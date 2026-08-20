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
// cleanup leaves them orphaned forever (nothing owns the root Secret). The sweep
// must delete exactly the labeled Secrets older than the configcheck timeout -
// never an unrelated Secret.
func TestSweepOrphansDeletesOnlyStaleConfigCheckSecrets(t *testing.T) {
	now := time.Now()
	stale := configCheckSecret("configcheck-old", now.Add(-time.Hour), true)
	staleAssets := configCheckSecret("configcheck-secret-assets-old", now.Add(-time.Hour), true)
	unrelated := configCheckSecret("user-creds", now.Add(-time.Hour), false)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cl := crfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(stale, staleAssets, unrelated).Build()

	require.NoError(t, SweepOrphans(context.Background(), cl, cl, 5*time.Minute))

	for _, gone := range []string{"configcheck-old", "configcheck-secret-assets-old"} {
		err := cl.Get(context.Background(), types.NamespacedName{Namespace: "vector", Name: gone}, &corev1.Secret{})
		require.True(t, api_errors.IsNotFound(err), "%s must be swept", gone)
	}
	require.NoError(t,
		cl.Get(context.Background(), types.NamespacedName{Namespace: "vector", Name: "user-creds"}, &corev1.Secret{}),
		"an unrelated Secret must survive the sweep")
}

// A configcheck cannot outlive its own timeout, so a younger Secret always belongs
// to a running check - including one owned by another operator process. Two processes
// overlap on every rolling update of the operator Deployment (the chart sets no
// strategy and does not enable leader election), and sweeping the older process'
// live Secret garbage-collects its pod: the check then fails with "pod was deleted
// before producing a result" and the pipeline is left invalid.
func TestSweepOrphansKeepsConfigChecksYoungerThanTimeout(t *testing.T) {
	now := time.Now()
	live := configCheckSecret("configcheck-live", now.Add(-30*time.Second), true)
	liveAssets := configCheckSecret("configcheck-secret-assets-live", now.Add(-30*time.Second), true)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cl := crfake.NewClientBuilder().WithScheme(scheme).WithObjects(live, liveAssets).Build()

	require.NoError(t, SweepOrphans(context.Background(), cl, cl, 5*time.Minute))

	for _, kept := range []string{"configcheck-live", "configcheck-secret-assets-live"} {
		require.NoError(t,
			cl.Get(context.Background(), types.NamespacedName{Namespace: "vector", Name: kept}, &corev1.Secret{}),
			"%s belongs to a running configcheck and must survive the sweep", kept)
	}
}

// The window is the configcheck timeout, which is operator-configurable down to zero.
// A zero or near-zero window would sweep every running check - the very bug the window
// exists to prevent - so it is floored.
func TestSweepOrphansFloorsTheWindowForTinyTimeouts(t *testing.T) {
	live := configCheckSecret("configcheck-live", time.Now().Add(-5*time.Second), true)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cl := crfake.NewClientBuilder().WithScheme(scheme).WithObjects(live).Build()

	require.NoError(t, SweepOrphans(context.Background(), cl, cl, 0))

	require.NoError(t,
		cl.Get(context.Background(), types.NamespacedName{Namespace: "vector", Name: "configcheck-live"}, &corev1.Secret{}),
		"a running configcheck must survive even when the configured timeout is degenerate")
}

// A single sweep at startup can only see what is already older than the window, so
// an orphan left behind moments before this process started would survive forever.
// The sweeper repeats, and the orphan ages into a later pass.
func TestRunOrphanSweeperSweepsSecretsThatAgeIntoTheWindow(t *testing.T) {
	fresh := configCheckSecret("configcheck-just-orphaned", time.Now(), true)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cl := crfake.NewClientBuilder().WithScheme(scheme).WithObjects(fresh).Build()

	// Drop the floor so the whole scenario fits in a few seconds; the floor itself is
	// covered by TestSweepOrphansFloorsTheWindowForTinyTimeouts.
	defer func(f time.Duration) { minOrphanAge = f }(minOrphanAge)
	minOrphanAge = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The window is wider than the one-second resolution of metav1.Time, so the Secret
	// is certainly too young for the first pass: only a repeated pass sweeps it. The
	// interval is independent of the window - sweeping is a cluster-wide List, too
	// expensive to repeat every window.
	go RunOrphanSweeper(ctx, cl, cl, 2*time.Second, 100*time.Millisecond)

	require.Never(t, func() bool {
		err := cl.Get(ctx, types.NamespacedName{Namespace: "vector", Name: "configcheck-just-orphaned"}, &corev1.Secret{})
		return api_errors.IsNotFound(err)
	}, time.Second, 100*time.Millisecond, "the first pass must not touch a Secret younger than the window")

	require.Eventually(t, func() bool {
		err := cl.Get(ctx, types.NamespacedName{Namespace: "vector", Name: "configcheck-just-orphaned"}, &corev1.Secret{})
		return api_errors.IsNotFound(err)
	}, 15*time.Second, 100*time.Millisecond, "a Secret that ages past the window must be swept by a later pass")
}
