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

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

// Labels select the agents and aggregators a pipeline goes to, and a label-only edit
// keeps the generation, so the predicate must pass it or the pipeline stays on the old
// aggregator.
func TestSpecAndAnnotationsPredicate_LabelOnlyEditPasses(t *testing.T) {
	g := NewWithT(t)

	old := &v1alpha1.ClusterVectorPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "cvp", Generation: 3, Labels: map[string]string{"team": "one"}},
	}
	relabeled := old.DeepCopy()
	relabeled.Labels = map[string]string{"team": "two"}

	g.Expect(specAndAnnotationsPredicate.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: relabeled})).To(BeTrue())

	unchanged := relabeled.DeepCopy()
	g.Expect(specAndAnnotationsPredicate.Update(event.UpdateEvent{ObjectOld: relabeled, ObjectNew: unchanged})).To(BeFalse())
}

// A workload from the initial list must not map: the pipelines get their own first
// reconcile on start, and mapping every workload would list all pipelines again and again.
func TestWorkloadCreatedSkipsInitialList(t *testing.T) {
	g := NewWithT(t)
	vector := &v1alpha1.Vector{ObjectMeta: metav1.ObjectMeta{Name: "v", Namespace: "default"}}

	g.Expect(workloadCreated.Create(event.CreateEvent{Object: vector, IsInInitialList: true})).To(BeFalse())
	g.Expect(workloadCreated.Create(event.CreateEvent{Object: vector})).To(BeTrue())
	g.Expect(workloadCreated.Update(event.UpdateEvent{ObjectOld: vector, ObjectNew: vector})).To(BeFalse())
}
