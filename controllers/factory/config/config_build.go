package config

import (
	"context"
	"encoding/json"
	"log"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
	"github.com/kaasops/vector-operator/controllers/factory/vectorpipeline"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	sourceDefault = map[string]interface{}{
		"defaultSource": map[string]string{
			"type": "kubernetes_logs",
		},
	}

	rate        int32 = 100
	sinkDefault       = map[string]interface{}{
		"defaultSink": map[string]interface{}{
			"type":                "blackhole",
			"inputs":              []string{"defaultSource"},
			"rate":                rate,
			"print_interval_secs": 60,
		},
	}
)

func Get(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client) ([]byte, error) {
	vps, err := vectorpipeline.SelectSucceesed(ctx, c)
	if err != nil {
		return nil, err
	}

	cfg, err := GenerateConfig(v, vps)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func GenerateConfig(
	v *vectorv1alpha1.Vector,
	vps map[string]*vectorv1alpha1.VectorPipeline,
) ([]byte, error) {
	cfg := vector.New(v.Spec.Agent.DataDir, v.Spec.Agent.ApiEnabled)
	sources, transforms, sinks, err := getComponents(vps)
	if err != nil {
		log.Fatal(err)
	}
	if len(sources) == 0 {
		sources = sourceDefault
	}
	if len(sinks) == 0 {
		sinks = sinkDefault
	}

	cfg.Sinks = sinks
	cfg.Sources = sources
	cfg.Transforms = transforms

	return vectorConfigToJson(cfg)
}

func getComponents(vps map[string]*vectorv1alpha1.VectorPipeline) (map[string]interface{}, map[string]interface{}, map[string]interface{}, error) {
	sources := make(map[string]interface{})
	transforms := make(map[string]interface{})
	sinks := make(map[string]interface{})

	for name, vp := range vps {
		if vp.Spec.Sources != nil {
			data, err := decode(vp.Spec.Sources)
			if err != nil {
				return nil, nil, nil, err
			}
			vp_sources := uniqWithPrefix(data, name)
			for source_k, source_v := range vp_sources {
				sources[source_k] = source_v
			}
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

func vectorConfigToJson(conf *vector.VectorConfig) ([]byte, error) {
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
