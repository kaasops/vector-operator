package vector

import (
	"encoding/json"
	"log"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

var (
	sourceDefault = v1alpha1.SourceSpec{
		Type: "kubernetes_logs",
	}

	rate        int32 = 100
	sinkDefault       = v1alpha1.SinkSpec{
		Type:              "blackhole",
		Inputs:            []string{"defaultSource"},
		Rate:              &rate,
		PrintIntervalSecs: 60,
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
		sinks = map[string]*v1alpha1.SinkSpec{
			"defaultSink": &sinkDefault,
		}
	}

	cfg.Sinks = sinks
	cfg.Sources = sources
	cfg.Transforms = transforms

	return vectorConfigToJson(cfg)
}

func NewVectorConfig(dataDir string, apiEnabled bool) *VectorConfig {
	sources := make(map[string]*v1alpha1.SourceSpec)
	sinks := make(map[string]*v1alpha1.SinkSpec)

	return &VectorConfig{
		DataDir: dataDir,
		Api: &ApiSpec{
			Enabled: &apiEnabled,
		},
		Sources: sources,
		Sinks:   sinks,
	}
}

func getComponents(vps map[string]*v1alpha1.VectorPipeline) (map[string]*v1alpha1.SourceSpec, map[string]interface{}, map[string]*v1alpha1.SinkSpec, error) {
	sources := make(map[string]*v1alpha1.SourceSpec)
	transforms := make(map[string]interface{})
	sinks := make(map[string]*v1alpha1.SinkSpec)

	for name, vp := range vps {
		for sourceName, source := range vp.Spec.Source {
			sources[name+"-"+sourceName] = &source
		}
		if vp.Spec.Transforms != nil {
			transform := make(map[string]interface{})
			err := json.Unmarshal(vp.Spec.Transforms.Raw, &transform)
			if err != nil {
				return nil, nil, nil, err
			}
			for transformName, transformspec := range transform {
				transforms[name+"-"+transformName] = transformspec
			}
		}
		for sinkName, sink := range vp.Spec.Sink {
			inputs := make([]string, 0)
			for _, i := range sink.Inputs {
				newInput := name + "-" + i
				inputs = append(inputs, newInput)
			}

			sink.Inputs = inputs
			sinks[name+"-"+sinkName] = &sink
		}
	}
	return sources, transforms, sinks, nil
}

// func createKeyValuePairs(m map[string]string) string {
// 	b := new(bytes.Buffer)
// 	for key, value := range m {
// 		fmt.Fprintf(b, "%s=\"%s\",", key, value)
// 	}
// 	return b.String()
// }

func vectorConfigToJson(conf *VectorConfig) ([]byte, error) {
	data, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	return data, nil
}
