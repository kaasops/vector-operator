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

package aggregator

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// Both reads below feed the bridge/prune decision, so both have to answer from the
// uncached reader. A cache that has not caught up understates what the assets Secret
// holds and what config is live, and either understatement makes a prune look free
// while a live config still references the key being dropped - so these specs hand the
// controller two readers that DISAGREE and assert the decision followed the API one.
// Same contract the write-order gate already has (see mount_gate_test.go).

func readerDivergenceAggregator() *vectorv1alpha1.VectorAggregator {
	return &vectorv1alpha1.VectorAggregator{
		ObjectMeta: metav1.ObjectMeta{Name: "va", Namespace: "default"},
	}
}

func TestExistingSecretAssets_FollowsAPIReaderNotCache(t *testing.T) {
	g := NewWithT(t)

	staleCached := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator-secret-assets", Namespace: "default"},
		Data:       map[string][]byte{"a": []byte("1")},
	}
	// What the cluster actually holds: the round before added "b" and the cache has
	// not observed it yet.
	current := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator-secret-assets", Namespace: "default"},
		Data:       map[string][]byte{"a": []byte("1"), "b": []byte("2")},
	}

	ctrl := NewController(readerDivergenceAggregator(), newFakeClient(g, staleCached), k8sfake.NewSimpleClientset())
	ctrl.APIReader = newFakeClient(g, current)

	got, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(HaveKey("b"), "the plan must see the key the cluster already holds, not the cache's older view")
	g.Expect(got).To(Equal(map[string][]byte{"a": []byte("1"), "b": []byte("2")}))
}

func TestPublishedConfigMatches_FollowsAPIReaderNotCache(t *testing.T) {
	g := NewWithT(t)
	byteConfig := []byte(`{"sinks":{"out":{}}}`)

	// The cache still shows the config this round is about to write, so a cached read
	// would answer "unchanged" and open the prune gate.
	cached := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "default"},
		Data:       map[string][]byte{"config.json": byteConfig},
	}
	// The cluster actually holds a different config, which may still reference keys
	// this round would prune.
	current := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "default"},
		Data:       map[string][]byte{"config.json": []byte(`{"sinks":{"other":{}}}`)},
	}

	ctrl := NewController(readerDivergenceAggregator(), newFakeClient(g, cached), k8sfake.NewSimpleClientset())
	ctrl.APIReader = newFakeClient(g, current)

	matches, err := ctrl.PublishedConfigMatches(context.Background(), byteConfig)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(matches).To(BeFalse(), "the prune gate must compare against the published config, not the cached one")
}

func TestSecretAssetsSafeguards_FailLoudlyWithoutAPIReader(t *testing.T) {
	g := NewWithT(t)

	// APIReader deliberately left nil: a caller that forgets to wire it must fail here
	// rather than silently fall back to the cache.
	ctrl := NewController(readerDivergenceAggregator(), newFakeClient(g), k8sfake.NewSimpleClientset())

	_, err := ctrl.ExistingSecretAssets(context.Background())
	g.Expect(err).To(MatchError(ContainSubstring("APIReader is not set")))

	_, err = ctrl.PublishedConfigMatches(context.Background(), []byte(`{}`))
	g.Expect(err).To(MatchError(ContainSubstring("APIReader is not set")))
}
