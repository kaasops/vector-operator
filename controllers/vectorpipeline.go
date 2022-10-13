package controllers

import (
	"bytes"
	"fmt"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"gopkg.in/yaml.v3"
)

func (r *VectorPipelineReconciler) NewVectorConfig(pipelineCR *vectorv1alpha1.VectorPipeline) VectorConfig {
	sources := make(map[string]VectorSource)
	sources[pipelineCR.Name] = VectorSource{
		Type:          "kubernetes_logs",
		LabelSelector: createKeyValuePairs(pipelineCR.Spec.Source.LabelSelector),
	}
	sinks := make(map[string]VectorSink)
	var inputs []string
	sinks[pipelineCR.Name] = VectorSink{
		Type:   pipelineCR.Spec.Sinks.Type,
		Inputs: append(inputs, pipelineCR.Name),
	}
	vectorConf := VectorConfig{
		DataDir: "/vector-data-dir",
		Api: VectorApiSpec{
			Enabled: false,
		},
		Sources: sources,
		Sinks:   sinks,
	}
	return vectorConf
}

func (r *VectorPipelineReconciler) VectorConfigToYaml(conf *VectorConfig) ([]byte, error) {
	data, err := yaml.Marshal(conf)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func createKeyValuePairs(m map[string]string) string {
	b := new(bytes.Buffer)
	for key, value := range m {
		fmt.Fprintf(b, "%s=\"%s\",", key, value)
	}
	return b.String()
}
