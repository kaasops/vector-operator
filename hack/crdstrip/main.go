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

// Command crdstrip removes the descriptions nested inside CRD fields of k8s.io types and
// keeps the description of the field itself. It runs in make manifests after controller-gen
// and expects its output: one CRD per file.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

type node = map[interface{}]interface{}

func main() {
	dir := flag.String("dir", "config/crd/bases", "directory with the generated CRDs")
	flag.Parse()

	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string) error {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := stripFile(scheme, file); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	return nil
}

func stripFile(scheme *runtime.Scheme, file string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var crd map[string]interface{}
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		return err
	}
	if crd["kind"] != "CustomResourceDefinition" {
		return nil
	}

	spec, _ := crd["spec"].(node)
	names, _ := spec["names"].(node)
	group, _ := spec["group"].(string)
	kind, _ := names["kind"].(string)
	versions, _ := spec["versions"].([]interface{})
	for _, v := range versions {
		version, _ := v.(node)
		name, _ := version["name"].(string)
		obj, err := scheme.New(schema.GroupVersionKind{Group: group, Version: name, Kind: kind})
		if err != nil {
			return err
		}
		versionSchema, _ := version["schema"].(node)
		openAPI, _ := versionSchema["openAPIV3Schema"].(node)
		if openAPI == nil {
			return fmt.Errorf("%s %s has no openAPIV3Schema", kind, name)
		}
		if err := stripSchema(openAPI, reflect.TypeOf(obj).Elem()); err != nil {
			return fmt.Errorf("%s %s: %w", kind, name, err)
		}
	}

	out, err := yaml.Marshal(crd)
	if err != nil {
		return err
	}
	return os.WriteFile(file, append([]byte("---\n"), out...), 0o644)
}

// stripSchema removes the descriptions below every spec and status field of t whose
// type comes from k8s.io.
func stripSchema(openAPI node, t reflect.Type) error {
	var roots []string
	for _, top := range []string{"Spec", "Status"} {
		if f, ok := t.FieldByName(top); ok {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			k8sRoots(f.Type, name, &roots)
		}
	}
	for _, root := range roots {
		n := nodeAt(openAPI, root)
		if n == nil {
			return fmt.Errorf("%s is not in the schema", root)
		}
		stripBelow(n)
	}
	return nil
}

// k8sRoots collects the JSON paths of the fields of t whose type comes from k8s.io. It
// does not descend into those types and follows the controller-gen rules for fields:
// no json tag or "-" skips a field, an empty name or the inline option inlines it.
func k8sRoots(t reflect.Type, prefix string, roots *[]string) {
	t = elem(t)
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("json")
		if !ok || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		inline := name == "" || slices.Contains(strings.Split(opts, ","), "inline")
		path := prefix
		if !inline {
			path = strings.TrimPrefix(prefix+"."+name, ".")
		}
		ft := elem(f.Type)
		if strings.HasPrefix(ft.PkgPath(), "k8s.io/") {
			if !inline {
				*roots = append(*roots, path)
			}
			continue
		}
		k8sRoots(ft, path, roots)
	}
}

func elem(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	return t
}

// nodeAt returns the schema of the field at the dot-separated path, stepping through
// the element schema of arrays and maps on the way.
func nodeAt(n node, path string) node {
	for _, seg := range strings.Split(path, ".") {
		n = elemSchema(n)
		props, _ := n["properties"].(node)
		n, _ = props[seg].(node)
		if n == nil {
			return nil
		}
	}
	return n
}

func elemSchema(n node) node {
	for {
		if items, ok := n["items"].(node); ok {
			n = items
			continue
		}
		if _, ok := n["properties"]; !ok {
			if values, ok := n["additionalProperties"].(node); ok {
				n = values
				continue
			}
		}
		return n
	}
}

// stripBelow removes the descriptions of every schema below n and keeps the one of n.
// A structural schema has no descriptions inside allOf, anyOf, oneOf or not.
func stripBelow(n node) {
	for _, child := range children(n) {
		delete(child, "description")
		stripBelow(child)
	}
}

func children(n node) []node {
	var kids []node
	if props, ok := n["properties"].(node); ok {
		for _, v := range props {
			if child, ok := v.(node); ok {
				kids = append(kids, child)
			}
		}
	}
	for _, key := range []string{"items", "additionalProperties"} {
		if child, ok := n[key].(node); ok {
			kids = append(kids, child)
		}
	}
	return kids
}
