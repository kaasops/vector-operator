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

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	crdDir = "../../config/crd/bases"

	// Client-side kubectl apply keeps a copy of the whole object in an annotation, so a CRD
	// that outgrows this limit can no longer be applied that way.
	annotationLimit = apivalidation.TotalAnnotationSizeLimitB
	// annotationBudget leaves room to notice the growth before kubectl apply breaks.
	annotationBudget = annotationLimit * 8 / 10
)

type fakeKind struct {
	Spec   fakeSpec   `json:"spec,omitempty"`
	Status fakeStatus `json:"status,omitempty"`
}

type fakeSpec struct {
	fakeInline                  `json:",inline"`
	corev1.LocalObjectReference `json:",inline"`
	Own                         string                `json:"own,omitempty"`
	Affinity                    *corev1.Affinity      `json:"affinity,omitempty"`
	Volumes                     []corev1.Volume       `json:"volumes,omitempty"`
	Nested                      fakeNested            `json:"nested,omitempty"`
	Items                       map[string]fakeNested `json:"items,omitempty"`
	Skipped                     *corev1.Affinity      `json:"-"`
}

type fakeInline struct {
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

type fakeNested struct {
	Name      string                       `json:"name,omitempty"`
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

type fakeStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

const fakeSchema = `
type: object
properties:
  spec:
    description: spec
    type: object
    properties:
      own:
        description: own
        type: string
      affinity:
        description: affinity
        type: object
        properties:
          nodeAffinity:
            description: k8s
            type: object
            properties:
              requiredDuringSchedulingIgnoredDuringExecution:
                description: k8s
                type: object
      volumes:
        description: volumes
        type: array
        items:
          description: k8s
          type: object
          properties:
            name:
              description: k8s
              type: string
            description:
              description: k8s
              type: string
      tolerations:
        description: tolerations
        type: array
        items:
          description: k8s
          type: object
      nested:
        description: nested
        type: object
        properties:
          name:
            description: nested.name
            type: string
          resources:
            description: nested.resources
            type: object
            properties:
              limits:
                description: k8s
                type: object
                additionalProperties:
                  anyOf:
                  - type: integer
                  - type: string
                  x-kubernetes-int-or-string: true
      items:
        description: items
        type: object
        additionalProperties:
          description: items.value
          type: object
          properties:
            resources:
              description: items.value.resources
              type: object
              properties:
                claims:
                  description: k8s
                  type: array
  status:
    type: object
    properties:
      conditions:
        description: conditions
        type: array
        items:
          description: k8s
          type: object
`

func TestK8sRoots(t *testing.T) {
	g := NewWithT(t)

	var roots []string
	k8sRoots(reflect.TypeOf(fakeSpec{}), "spec", &roots)

	g.Expect(roots).To(Equal([]string{
		"spec.tolerations",
		"spec.affinity",
		"spec.volumes",
		"spec.nested.resources",
		"spec.items.resources",
	}))
}

func TestStripSchemaKeepsOwnDescriptions(t *testing.T) {
	g := NewWithT(t)

	var schema node
	g.Expect(yaml.Unmarshal([]byte(fakeSchema), &schema)).To(Succeed())
	g.Expect(stripSchema(schema, reflect.TypeOf(fakeKind{}))).To(Succeed())

	var descriptions []string
	collectDescriptions(schema, &descriptions)

	// Every "k8s" description sat below a field of a k8s.io type, and only those go.
	g.Expect(descriptions).To(ConsistOf(
		"spec", "own", "affinity", "volumes", "tolerations",
		"nested", "nested.name", "nested.resources",
		"items", "items.value", "items.value.resources",
		"conditions",
	))
	volume := nodeAt(schema, "spec.volumes")
	g.Expect(elemSchema(volume)["properties"]).To(HaveKey("description"),
		"a property named description is a field, not a description")
}

func TestStripSchemaFailsOnMissingField(t *testing.T) {
	g := NewWithT(t)

	var schema node
	g.Expect(yaml.Unmarshal([]byte("type: object\nproperties:\n  spec:\n    type: object\n"), &schema)).To(Succeed())

	g.Expect(stripSchema(schema, reflect.TypeOf(fakeKind{}))).To(MatchError(ContainSubstring("spec.tolerations is not in the schema")))
}

func TestGeneratedCRDsFitClientSideApply(t *testing.T) {
	g := NewWithT(t)

	files, err := filepath.Glob(filepath.Join(crdDir, "*.yaml"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(files).NotTo(BeEmpty())

	for _, file := range files {
		raw, err := os.ReadFile(file)
		g.Expect(err).NotTo(HaveOccurred())
		size, err := lastAppliedSize(raw)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(size).To(BeNumerically("<=", annotationBudget),
			"%s takes %d of the %d annotation bytes on client-side kubectl apply", filepath.Base(file), size, annotationLimit)
	}
}

// lastAppliedSize estimates the annotation bytes of an object after client-side kubectl
// apply: its own annotations plus the last-applied-configuration annotation, which
// holds the object as JSON with a trailing newline.
func lastAppliedSize(raw []byte) (int, error) {
	data, err := sigsyaml.YAMLToJSON(raw)
	if err != nil {
		return 0, err
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return 0, err
	}
	value, err := json.Marshal(obj)
	if err != nil {
		return 0, err
	}
	size := len("kubectl.kubernetes.io/last-applied-configuration") + len(value) + 1
	metadata, _ := obj["metadata"].(map[string]interface{})
	annotations, _ := metadata["annotations"].(map[string]interface{})
	for key, val := range annotations {
		s, _ := val.(string)
		size += len(key) + len(s)
	}
	return size, nil
}

func collectDescriptions(n node, out *[]string) {
	if d, ok := n["description"].(string); ok {
		*out = append(*out, d)
	}
	for _, child := range children(n) {
		collectDescriptions(child, out)
	}
}
