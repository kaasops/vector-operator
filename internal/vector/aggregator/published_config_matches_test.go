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

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/utils/compression"
)

// See the agent's identical TestPublishedConfigMatches for why this is covered
// directly: it gates a safety-critical decision (secretAssetsPruneDecision), not
// just a status field.
func TestPublishedConfigMatches(t *testing.T) {
	newAggregator := func(compress bool) *vectorv1alpha1.VectorAggregator {
		return &vectorv1alpha1.VectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: "va", Namespace: "ns"},
			Spec: vectorv1alpha1.VectorAggregatorSpec{
				VectorAggregatorCommon: vectorv1alpha1.VectorAggregatorCommon{
					VectorCommon: vectorv1alpha1.VectorCommon{CompressConfigFile: compress},
				},
			},
		}
	}

	t.Run("absent config Secret counts as not published, not an error", func(t *testing.T) {
		g := NewWithT(t)
		va := newAggregator(false)
		ctrl := NewController(va, newFakeClient(g), k8sfake.NewSimpleClientset())
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), []byte(`{"a":1}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse())
	})

	t.Run("config Secret present but missing the config.json key does not match a non-empty config", func(t *testing.T) {
		g := NewWithT(t)
		va := newAggregator(false)
		existing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "ns"},
			Data:       map[string][]byte{"other-key": []byte("irrelevant")},
		}
		ctrl := NewController(va, newFakeClient(g, existing), k8sfake.NewSimpleClientset())
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), []byte(`{"a":1}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse())
	})

	t.Run("byte-identical plain config matches", func(t *testing.T) {
		g := NewWithT(t)
		va := newAggregator(false)
		byteConfig := []byte(`{"a":1}`)
		existing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "ns"},
			Data:       map[string][]byte{"config.json": byteConfig},
		}
		ctrl := NewController(va, newFakeClient(g, existing), k8sfake.NewSimpleClientset())
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), byteConfig)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeTrue())
	})

	t.Run("different plain config does not match", func(t *testing.T) {
		g := NewWithT(t)
		va := newAggregator(false)
		existing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "ns"},
			Data:       map[string][]byte{"config.json": []byte(`{"a":1}`)},
		}
		ctrl := NewController(va, newFakeClient(g, existing), k8sfake.NewSimpleClientset())
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), []byte(`{"a":2}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse())
	})

	t.Run("compressed config matches only against the compressed form, not the plain bytes", func(t *testing.T) {
		g := NewWithT(t)
		va := newAggregator(true)
		byteConfig := []byte(`{"a":1,"b":"a reasonably long value so compression has something to do"}`)
		compressed := compression.Compress(byteConfig, logr.Discard())

		plainStored := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "ns"},
			Data:       map[string][]byte{"config.json": byteConfig},
		}
		ctrlPlain := NewController(va, newFakeClient(g, plainStored), k8sfake.NewSimpleClientset())
		ctrlPlain.APIReader = ctrlPlain.Client
		matches, err := ctrlPlain.PublishedConfigMatches(context.Background(), byteConfig)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse(), "CompressConfigFile is on, so the stored plain bytes must not be mistaken for a match")

		compressedStored := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "va-aggregator", Namespace: "ns"},
			Data:       map[string][]byte{"config.json": compressed},
		}
		ctrlCompressed := NewController(va, newFakeClient(g, compressedStored), k8sfake.NewSimpleClientset())
		ctrlCompressed.APIReader = ctrlCompressed.Client
		matches, err = ctrlCompressed.PublishedConfigMatches(context.Background(), byteConfig)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeTrue())
	})
}
