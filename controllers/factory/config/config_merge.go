package config

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Experemental
func (b *Builder) optimizeVectorConfig(config *VectorConfig) error {
	var optimizedSource []*Source
	var optimizationRequired bool
	for _, source := range config.Sources {
		if source.ExtraNamespaceLabelSelector != "" && source.Type == KubernetesSourceType && source.ExtraLabelSelector != "" {
			if source.ExtraFieldSelector != "" {
				optimizedSource = append(optimizedSource, source)
				continue
			}
			optimizationRequired = true

			config.Transforms = append(config.Transforms, &Transform{
				Name:      source.Name,
				Inputs:    []string{OptimizedKubernetesSourceName},
				Type:      FilterTransformType,
				Condition: generateVrlFilter(source.ExtraLabelSelector, PodSelectorType) + "&&" + generateVrlFilter(source.ExtraNamespaceLabelSelector, NamespaceSelectorType),
			})
			continue
		}
		optimizedSource = append(optimizedSource, source)
	}

	if optimizationRequired {
		optimizedSource = append(optimizedSource, &Source{
			Name: OptimizedKubernetesSourceName,
			Type: KubernetesSourceType,
		})
		config.Sources = optimizedSource
	}

	merge(config)

	return nil
}

func merge(config *VectorConfig) {
	optimizedSink := mergeSync(config.Sinks)

	if len(optimizedSink) == 0 {
		return
	}

	sort.Slice(optimizedSink, func(i, j int) bool {
		return optimizedSink[i].Name > optimizedSink[j].Name
	})

	config.Sinks = optimizedSink

	t_map := transformsToMap(config.Transforms)
	t_opts := make(map[string]*Transform)

	var optimizedTransforms []*Transform
	for _, sink := range config.Sinks {
		hash, ok := isMergable(t_map, sink.Inputs)
		if !ok {
			continue
		}
		for _, i := range sink.Inputs {
			t := t_map[i]
			t_v, ok := t_opts[hash]
			if ok {
				t_v.Inputs = append(t_v.Inputs, t.Inputs...)
				sort.Strings(t_v.Inputs)
				t_v.Name = hash
				t_map[hash] = t_v
				delete(t_map, i)
				continue
			}
			t.Name = hash
			t_map[hash] = t
			t_opts[hash] = t
			delete(t_map, i)
		}
		sink.Inputs = nil
		sink.Inputs = append(sink.Inputs, hash)
	}
	for _, v := range t_map {
		optimizedTransforms = append(optimizedTransforms, v)
	}

	sort.Slice(optimizedTransforms, func(i, j int) bool {
		return optimizedTransforms[i].Name > optimizedTransforms[j].Name
	})

	if len(optimizedTransforms) > 0 {
		config.Transforms = optimizedTransforms
	}
}

// Fucntion tries to merge syncs if options hash is equal
func mergeSync(sinks []*Sink) []*Sink {
	uniqOpts := make(map[string]*Sink)
	var optimizedSink []*Sink

	for _, sink := range sinks {
		sink_copy := *sink
		v, ok := uniqOpts[sink_copy.OptionsHash]
		if ok {
			// If sink spec already exists rename and merge inputs
			v.Name = v.OptionsHash
			v.Inputs = append(v.Inputs, sink_copy.Inputs...)
			sort.Strings(v.Inputs)
			continue
		}
		uniqOpts[sink_copy.OptionsHash] = &sink_copy
		optimizedSink = append(optimizedSink, &sink_copy)
	}
	return optimizedSink
}

func isMergable(t_map map[string]*Transform, transforms []string) (string, bool) {
	var hash string
	if len(transforms) < 2 {
		return "", false
	}
	for _, t := range transforms {
		v, ok := t_map[t]
		if !ok {
			return "", false
		}
		if hash != "" {
			if v.OptionsHash != hash {
				return "", false
			}
		}
		hash = v.OptionsHash
	}
	if hash == "" {
		return "", false
	}
	return hash, true
}

func transformsToMap(transforms []*Transform) map[string]*Transform {
	result := make(map[string]*Transform)
	for _, t := range transforms {
		t_copy := *t
		result[t_copy.Name] = &t_copy
	}
	return result
}

func generateVrlFilter(selector string, selectorType string) string {
	buffer := new(bytes.Buffer)

	labels := strings.Split(selector, ",")

	for _, label := range labels {
		label = formatVrlSelector(label)
		switch selectorType {
		case PodSelectorType:
			fmt.Fprintf(buffer, ".kubernetes.pod_labels.%s&&", label)
		case NamespaceSelectorType:
			fmt.Fprintf(buffer, ".kubernetes.namespace_labels.%s&&", label)
		}
	}

	vrlstring := buffer.String()
	return strings.TrimSuffix(vrlstring, "&&")
}

func formatVrlSelector(label string) string {
	l := strings.Split(label, "!=")
	if len(l) == 2 {
		return fmt.Sprintf("\"%s\" != \"%s\"", l[0], l[1])
	}

	l = strings.Split(label, "=")
	if len(l) != 2 {
		return label
	}
	return fmt.Sprintf("\"%s\" == \"%s\"", l[0], l[1])
}
