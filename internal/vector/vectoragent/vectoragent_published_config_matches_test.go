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

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/utils/compression"
)

// PublishedConfigMatches/AltPublishedConfigMatches gate a safety-critical decision
// (secretAssetsPruneDecision, via allConfigsUnchanged in vector_controller.go) - see
// their own doc comments for why a false "unchanged" there is a real, demonstrated
// pruning hazard, not just a cosmetic status flicker. This suite covers them
// directly, without going through a full reconcile.
func TestPublishedConfigMatches(t *testing.T) {
	newVector := func(compress bool) *vectorv1alpha1.Vector {
		return &vectorv1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: vectorv1alpha1.VectorSpec{
				Agent: &vectorv1alpha1.VectorAgent{
					VectorCommon: vectorv1alpha1.VectorCommon{CompressConfigFile: compress},
				},
			},
		}
	}

	t.Run("absent config Secret counts as not published, not an error", func(t *testing.T) {
		g := NewWithT(t)
		v := newVector(false)
		ctrl := NewController(v, newFakeClient(g), nil)
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), []byte(`{"a":1}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse())
	})

	t.Run("config Secret present but missing the agent.json key does not match a non-empty config", func(t *testing.T) {
		g := NewWithT(t)
		v := newVector(false)
		existing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "ns"},
			Data:       map[string][]byte{"other-key": []byte("irrelevant")},
		}
		ctrl := NewController(v, newFakeClient(g, existing), nil)
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), []byte(`{"a":1}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse())
	})

	t.Run("byte-identical plain config matches", func(t *testing.T) {
		g := NewWithT(t)
		v := newVector(false)
		byteConfig := []byte(`{"a":1}`)
		existing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "ns"},
			Data:       map[string][]byte{"agent.json": byteConfig},
		}
		ctrl := NewController(v, newFakeClient(g, existing), nil)
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), byteConfig)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeTrue())
	})

	t.Run("different plain config does not match", func(t *testing.T) {
		g := NewWithT(t)
		v := newVector(false)
		existing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "ns"},
			Data:       map[string][]byte{"agent.json": []byte(`{"a":1}`)},
		}
		ctrl := NewController(v, newFakeClient(g, existing), nil)
		ctrl.APIReader = ctrl.Client

		matches, err := ctrl.PublishedConfigMatches(context.Background(), []byte(`{"a":2}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse())
	})

	t.Run("compressed config matches only against the compressed form, not the plain bytes", func(t *testing.T) {
		g := NewWithT(t)
		v := newVector(true)
		byteConfig := []byte(`{"a":1,"b":"a reasonably long value so compression has something to do"}`)
		compressed := compression.Compress(byteConfig, logr.Discard())

		plainStored := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "ns"},
			Data:       map[string][]byte{"agent.json": byteConfig},
		}
		ctrlPlain := NewController(v, newFakeClient(g, plainStored), nil)
		ctrlPlain.APIReader = ctrlPlain.Client
		matches, err := ctrlPlain.PublishedConfigMatches(context.Background(), byteConfig)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeFalse(), "CompressConfigFile is on, so the stored plain bytes must not be mistaken for a match")

		compressedStored := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "ns"},
			Data:       map[string][]byte{"agent.json": compressed},
		}
		ctrlCompressed := NewController(v, newFakeClient(g, compressedStored), nil)
		ctrlCompressed.APIReader = ctrlCompressed.Client
		matches, err = ctrlCompressed.PublishedConfigMatches(context.Background(), byteConfig)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(matches).To(BeTrue())
	})

	t.Run("AltPublishedConfigMatches checks the alt-named Secret, independently of the active one", func(t *testing.T) {
		g := NewWithT(t)
		v := newVector(false)
		activeConfig := []byte(`{"active":true}`)
		altConfig := []byte(`{"alt":true}`)

		active := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "ns"},
			Data:       map[string][]byte{"agent.json": activeConfig},
		}
		alt := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-agent-opt", Namespace: "ns"},
			Data:       map[string][]byte{"agent.json": altConfig},
		}
		ctrl := NewController(v, newFakeClient(g, active, alt), nil)
		ctrl.APIReader = ctrl.Client
		ctrl.CheckpointMigration = true

		g.Expect(ctrl.PublishedConfigMatches(context.Background(), activeConfig)).To(BeTrue())
		g.Expect(ctrl.PublishedConfigMatches(context.Background(), altConfig)).To(BeFalse(), "must not cross-match against the alt Secret's content")

		g.Expect(ctrl.AltPublishedConfigMatches(context.Background(), altConfig)).To(BeTrue())
		g.Expect(ctrl.AltPublishedConfigMatches(context.Background(), activeConfig)).To(BeFalse(), "must not cross-match against the active Secret's content")
	})
}
