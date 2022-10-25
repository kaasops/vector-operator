package config

import (
	"context"
	"encoding/json"
	"log"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/vector"
	"github.com/kaasops/vector-operator/controllers/factory/vectorpipeline"
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
	vps []*vectorv1alpha1.VectorPipeline,
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

func getComponents(vps []*vectorv1alpha1.VectorPipeline) (map[string]interface{}, map[string]interface{}, map[string]interface{}, error) {
	sourcesMap := make(map[string]interface{})
	transformsMap := make(map[string]interface{})
	sinksMap := make(map[string]interface{})

	for _, vp := range vps {
		sources, err := vectorpipeline.GetSources(vp, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, source := range sources {
			spec, err := vector.Decoder(source)
			if err != nil {
				return nil, nil, nil, err
			}
			sourcesMap[source.Name] = spec
		}
		transforms, err := vectorpipeline.GetTransforms(vp)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, transform := range transforms {
			spec, err := vector.Decoder(transform)
			if err != nil {
				return nil, nil, nil, err
			}
			transformsMap[transform.Name] = spec
		}
		sinks, err := vectorpipeline.GetSinks(vp)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, sink := range sinks {
			spec, err := vector.Decoder(sink)
			if err != nil {
				return nil, nil, nil, err
			}
			sinksMap[sink.Name] = spec
		}
	}
	return sourcesMap, transformsMap, sinksMap, nil
}

func vectorConfigToJson(conf *vector.VectorConfig) ([]byte, error) {
	data, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	return data, nil
}
