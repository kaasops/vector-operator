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

	config.merge()

	return nil
}

func (c *VectorConfig) merge() {
	optimizedSink := mergeSync(c.Sinks)

	if len(optimizedSink) == 0 {
		return
	}

	sort.Slice(optimizedSink, func(i, j int) bool {
		return optimizedSink[i].Name > optimizedSink[j].Name
	})

	transforms := transformsToMap(c.Transforms)

	var optimizedTransforms []*Transform

	for _, sink := range optimizedSink {
		if len(sink.Inputs) < 2 {
			continue
		}
		toAdd, toDelete, mergedInputs := c.mergeSinkInputs(sink.Inputs, sink.Name)
		sink.Inputs = mergedInputs
		optimizedTransforms = append(optimizedTransforms, toAdd...)
		for _, d := range toDelete {
			delete(transforms, d)
		}
	}

	for _, v := range transforms {
		optimizedTransforms = append(optimizedTransforms, v)
	}

	sort.Slice(optimizedTransforms, func(i, j int) bool {
		return optimizedTransforms[i].Name > optimizedTransforms[j].Name
	})

	c.Sinks = optimizedSink

	if len(optimizedTransforms) > 0 {
		c.Transforms = optimizedTransforms
	}
}

// Fucntion tries to merge syncs if options hash is equal
func mergeSync(sinks []*Sink) []*Sink {
	uniqOpts := make(map[string]*Sink)
	var optimizedSink []*Sink

	for _, sink := range sinks {
		if sink.Type != ElasticsearchSinkType {
			optimizedSink = append(optimizedSink, sink)
			continue
		}
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

func (c *VectorConfig) mergeSinkInputs(inputs []string, prefix string) (toAdd []*Transform, toDelete []string, mergedInputs []string) {
	t_map := transformsToMap(c.Transforms)
	t_opt := make(map[string]*Transform)
	for _, i := range inputs {
		t, ok := t_map[i]
		// Not in transform list, add without modification
		if !ok {
			mergedInputs = append(mergedInputs, t.Name)
			continue
		}

		// If options hash is empty transform is not mergable
		if t.OptionsHash == "" {
			mergedInputs = append(mergedInputs, t.Name)
			continue
		}

		t_merged, ok := t_opt[t.OptionsHash]
		name := t.OptionsHash + "-" + prefix
		if ok {
			t_merged.Name = name
			t_merged.Inputs = append(t_merged.Inputs, t.Inputs...)
			sort.Strings(t_merged.Inputs)
			t_merged.Merged = true
			toDelete = append(toDelete, t.Name)
			continue
		}
		t_opt[t.OptionsHash] = t
	}
	for _, v := range t_opt {
		if v.Merged {
			toAdd = append(toAdd, v)
		}
		mergedInputs = append(mergedInputs, v.Name)
	}
	return toAdd, toDelete, mergedInputs
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