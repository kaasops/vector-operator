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

package vectoragent

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// ExistingSecretAssets and PublishedConfigMatches both feed the bridge/prune decision,
// so both have to answer from the uncached reader - the same contract hasSecretAssetsMount
// already has. A cache that has not caught up understates what the assets Secret holds and
// what config is live, and either understatement makes a prune look free while a live
// config still references the key being dropped. These specs hand the controller two
// readers that DISAGREE and assert the decision followed the API one.

func readerDivergenceVector() *vectorv1alpha1.Vector {
	return &vectorv1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "vector"}}
}

func TestExistingSecretAssets_FollowsAPIReaderNotCache(t *testing.T) {
	g := NewWithT(t)

	staleCached := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"a": []byte("1")},
	}
	// What the cluster actually holds: a preceding round added "b" and the cache has
	// not observed that write yet.
	current := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"a": []byte("1"), "b": []byte("2")},
	}

	ctrl := NewController(readerDivergenceVector(), newFakeClient(g, staleCached), nil)
	ctrl.APIReader = newFakeClient(g, current)

	primary, _, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(primary).To(Equal(map[string][]byte{"a": []byte("1"), "b": []byte("2")}),
		"the plan must see the key the cluster already holds, not the cache's older view")
}

// Under checkpoint migration both variants are read, and the alt one is exactly the
// variant a not-yet-rolled pod may still be mounting - reading it from the cache would
// understate it the same way.
func TestExistingSecretAssets_AltVariantFollowsAPIReaderNotCache(t *testing.T) {
	g := NewWithT(t)

	activeCached := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-opt-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"active": []byte("a")},
	}
	staleStandby := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"standby": []byte("b")},
	}
	currentStandby := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-secret-assets", Namespace: "vector"},
		Data:       map[string][]byte{"standby": []byte("b"), "standby_new": []byte("c")},
	}

	ctrl := NewController(readerDivergenceVector(), newFakeClient(g, activeCached, staleStandby), nil)
	ctrl.APIReader = newFakeClient(g, activeCached.DeepCopy(), currentStandby)
	ctrl.CheckpointMigration = true
	ctrl.OptimizeSources = true

	_, alt, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(alt).To(HaveKey("standby_new"))
}

func TestPublishedConfigMatches_FollowsAPIReaderNotCache(t *testing.T) {
	g := NewWithT(t)
	byteConfig := []byte(`{"sinks":{"out":{}}}`)

	// The cache still shows the config this round is about to write, so a cached read
	// would answer "unchanged" and open the prune gate.
	cached := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "vector"},
		Data:       map[string][]byte{"agent.json": byteConfig},
	}
	// The cluster actually holds a different config, which may still reference keys
	// this round would prune.
	current := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "vector"},
		Data:       map[string][]byte{"agent.json": []byte(`{"sinks":{"other":{}}}`)},
	}

	ctrl := NewController(readerDivergenceVector(), newFakeClient(g, cached), nil)
	ctrl.APIReader = newFakeClient(g, current)

	matches, err := ctrl.PublishedConfigMatches(context.Background(), byteConfig)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(matches).To(BeFalse(), "the prune gate must compare against the published config, not the cached one")
}

func TestSecretAssetsSafeguards_FailLoudlyWithoutAPIReader(t *testing.T) {
	g := NewWithT(t)

	// APIReader deliberately left nil: a caller that forgets to wire it must fail here
	// rather than silently fall back to the cache.
	ctrl := NewController(readerDivergenceVector(), newFakeClient(g), nil)

	_, _, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).To(MatchError(ContainSubstring("APIReader is not set")))

	_, err = ctrl.PublishedConfigMatches(context.Background(), []byte(`{}`))
	g.Expect(err).To(MatchError(ContainSubstring("APIReader is not set")))

	_, err = ctrl.AltPublishedConfigMatches(context.Background(), []byte(`{}`))
	g.Expect(err).To(MatchError(ContainSubstring("APIReader is not set")))
}
