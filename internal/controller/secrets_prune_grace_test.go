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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSecretAssetsPruneDecision covers secretAssetsPruneDecision's own math in
// isolation (no envtest needed): the anchor-vs-remaining logic that
// vector_controller_secret_assets_deferred_prune_test.go and
// vector_controller_secret_size_test.go exercise end to end through real
// reconciles.
func TestSecretAssetsPruneDecision(t *testing.T) {
	t.Run("config changing this round never prunes, regardless of publishedAt", func(t *testing.T) {
		prune, requeueAfter := secretAssetsPruneDecision(false, nil)
		assert.False(t, prune)
		assert.Zero(t, requeueAfter)

		recentlyPublished := metav1.NewTime(time.Now())
		prune, requeueAfter = secretAssetsPruneDecision(false, &recentlyPublished)
		assert.False(t, prune)
		assert.Zero(t, requeueAfter)
	})

	t.Run("unchanged config but no publish record yet defers a full grace period and does not prune", func(t *testing.T) {
		prune, requeueAfter := secretAssetsPruneDecision(true, nil)
		assert.False(t, prune, "absent is 'not safe yet', never 'nothing to protect'")
		assert.Equal(t, SecretAssetsPruneGracePeriod, requeueAfter)
	})

	t.Run("unchanged config within the grace window defers by the REMAINING time, not a fresh interval", func(t *testing.T) {
		publishedAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
		prune, requeueAfter := secretAssetsPruneDecision(true, &publishedAt)
		assert.False(t, prune)
		// ~60s left (grace - 30s elapsed), not the full 90s - proves the remaining
		// time is anchored to publishedAt, not recomputed as a fresh interval.
		assert.InDelta(t, (SecretAssetsPruneGracePeriod - 30*time.Second).Seconds(), requeueAfter.Seconds(), 1)
	})

	t.Run("calling again later against the SAME publishedAt only ever shrinks the remaining wait, never resets it", func(t *testing.T) {
		publishedAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
		_, first := secretAssetsPruneDecision(true, &publishedAt)

		// Simulate a later reconcile against the identical, untouched status mark -
		// exactly what happens when nothing else triggers a fresh reconcile in
		// between and SetSuccessStatus leaves LastConfigPublishedAt alone (see
		// vectoragent.Controller.SetSuccessStatus / aggregator.Controller.SetSuccessStatus).
		// A caller that instead restarted the countdown on every call (e.g. always
		// requeuing after a fresh SecretAssetsPruneGracePeriod) would let a workload
		// reconciled more often than the grace period defer pruning forever - the
		// exact hot-loop-of-deferral this anchoring exists to rule out.
		publishedAtSameAnchor := publishedAt
		time.Sleep(50 * time.Millisecond)
		_, second := secretAssetsPruneDecision(true, &publishedAtSameAnchor)

		assert.LessOrEqual(t, second, first, "the remaining wait must shrink (or stay put), never grow, against an unchanged anchor")
	})

	t.Run("unchanged config once the grace period has fully elapsed is safe to prune", func(t *testing.T) {
		publishedAt := metav1.NewTime(time.Now().Add(-SecretAssetsPruneGracePeriod - time.Second))
		prune, requeueAfter := secretAssetsPruneDecision(true, &publishedAt)
		assert.True(t, prune)
		assert.Zero(t, requeueAfter)
	})

	t.Run("exactly at the boundary is already safe (grace is a minimum, not a strict lower bound)", func(t *testing.T) {
		publishedAt := metav1.NewTime(time.Now().Add(-SecretAssetsPruneGracePeriod))
		prune, _ := secretAssetsPruneDecision(true, &publishedAt)
		assert.True(t, prune)
	})
}

// TestAssetsWouldDropAKey covers the fast path: a caller must be able to tell "nothing
// existing would be dropped by the target" so it can skip secretAssetsPruneDecision's
// grace/requeue dance entirely for workloads that never trigger an actual removal.
func TestAssetsWouldDropAKey(t *testing.T) {
	t.Run("both empty - no drop", func(t *testing.T) {
		assert.False(t, assetsWouldDropAKey(nil, nil), "the common no-secrets-used case must never look like a drop")
	})

	t.Run("existing empty, target non-empty - purely additive, no drop", func(t *testing.T) {
		assert.False(t, assetsWouldDropAKey(nil, map[string][]byte{"k1": []byte("v1")}))
	})

	t.Run("existing a strict subset of target - no drop", func(t *testing.T) {
		existing := map[string][]byte{"k1": []byte("v1")}
		target := map[string][]byte{"k1": []byte("v1"), "k2": []byte("v2")}
		assert.False(t, assetsWouldDropAKey(existing, target))
	})

	t.Run("same key, different value (rotation) - not a drop", func(t *testing.T) {
		existing := map[string][]byte{"k1": []byte("old")}
		target := map[string][]byte{"k1": []byte("new")}
		assert.False(t, assetsWouldDropAKey(existing, target), "a value changing is a rotation, not a removal - it never needs to wait")
	})

	t.Run("target missing a key existing has - a real drop", func(t *testing.T) {
		existing := map[string][]byte{"k1": []byte("v1"), "k2": []byte("v2")}
		target := map[string][]byte{"k2": []byte("v2")}
		assert.True(t, assetsWouldDropAKey(existing, target))
	})

	t.Run("target empty, existing non-empty - dropping everything is still a drop", func(t *testing.T) {
		existing := map[string][]byte{"k1": []byte("v1")}
		assert.True(t, assetsWouldDropAKey(existing, nil))
	})
}
