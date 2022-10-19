package vector

import (
	"encoding/json"
	"log"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	sourceDefault = v1alpha1.SourceSpec{
		Type: "kubernetes_logs",
	}

	rate        int32 = 100
	sinkDefault       = map[string]interface{}{
		"type":              "blackhole",
		"inputs":            []string{"defaultSource"},
		"rate":              rate,
		"printIntervalSecs": 60,
	}
)

func GenerateConfig(
	cr *v1alpha1.Vector,
	vp map[string]*v1alpha1.VectorPipeline,
) ([]byte, error) {
	cfg := NewVectorConfig(cr.Spec.Agent.DataDir, cr.Spec.Agent.ApiEnabled)
	sources, transforms, sinks, err := getComponents(vp)
	if err != nil {
		log.Fatal(err)
	}
	if len(sources) == 0 {
		sources = map[string]*v1alpha1.SourceSpec{
			"defaultSource": &sourceDefault,
		}
	}
	if len(sinks) == 0 {
		sinks = sinkDefault
	}

	cfg.Sinks = sinks
	cfg.Sources = sources
	cfg.Transforms = transforms

	return vectorConfigToJson(cfg)
}

func NewVectorConfig(dataDir string, apiEnabled bool) *VectorConfig {
	sources := make(map[string]*v1alpha1.SourceSpec)
	sinks := make(map[string]interface{})

	return &VectorConfig{
		DataDir: dataDir,
		Api: &ApiSpec{
			Enabled: &apiEnabled,
		},
		Sources: sources,
		Sinks:   sinks,
	}
}

func getComponents(vps map[string]*v1alpha1.VectorPipeline) (map[string]*v1alpha1.SourceSpec, map[string]interface{}, map[string]interface{}, error) {
	sources := make(map[string]*v1alpha1.SourceSpec)
	transforms := make(map[string]interface{})
	sinks := make(map[string]interface{})

	for name, vp := range vps {
		for sourceName, source := range vp.Spec.Source {
			sources[addPrefix(name, sourceName)] = &source
		}
		if vp.Spec.Transforms != nil {
			data, err := decode(vp.Spec.Transforms)
			if err != nil {
				return nil, nil, nil, err
			}
			vp_transforms := uniqWithPrefix(data, name)
			for transform_k, transform_v := range vp_transforms {
				transforms[transform_k] = transform_v
			}
		}
		if vp.Spec.Sinks != nil {
			data, err := decode(vp.Spec.Sinks)
			if err != nil {
				return nil, nil, nil, err
			}
			vp_sinks := uniqWithPrefix(data, name)
			for sink_k, sink_v := range vp_sinks {
				sinks[sink_k] = sink_v
			}
		}
	}
	return sources, transforms, sinks, nil
}

func vectorConfigToJson(conf *VectorConfig) ([]byte, error) {
	data, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func uniqWithPrefix(in map[string]interface{}, prefix string) map[string]interface{} {
	out := make(map[string]interface{})
	for k_in, v_in := range in {
		spec := v_in.(map[string]interface{})
		out[addPrefix(prefix, k_in)] = spec
		for k_spec, v_spec := range spec {
			if k_spec == "inputs" {
				inputs := make([]string, 0)
				for _, i := range v_spec.([]interface{}) {
					newInput := addPrefix(prefix, i.(string))
					inputs = append(inputs, newInput)
				}
				spec[k_spec] = inputs
				continue
			}
		}
	}
	return out
}

func addPrefix(prefix string, name string) string {
	return prefix + "-" + name
}

func decode(data *runtime.RawExtension) (map[string]interface{}, error) {
	out := make(map[string]interface{})
	err := json.Unmarshal(data.Raw, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
