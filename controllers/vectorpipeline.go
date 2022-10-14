package controllers

import (
	"bytes"
	"fmt"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"gopkg.in/yaml.v3"
)

func NewVectorConfigFromCR(pipelineCR *vectorv1alpha1.VectorPipeline) *VectorConfig {
	name := generateName(pipelineCR)

	config := NewVectorConfig()

	config.Sources[name] = VectorSource{
		Type:          "kubernetes_logs",
		LabelSelector: createKeyValuePairs(pipelineCR.Spec.Source.LabelSelector),
	}

	config.Sinks[name] = VectorSink{
		Type:   pipelineCR.Spec.Sinks.Type,
		Inputs: []string{name},
	}

	return config
}

func NewVectorConfig() *VectorConfig {
	sources := make(map[string]VectorSource)
	sinks := make(map[string]VectorSink)

	return &VectorConfig{
		DataDir: "/vector-data-dir",
		Api: VectorApiSpec{
			Enabled: false,
		},
		Sources: sources,
		Sinks:   sinks,
	}
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

func generateName(pipelineCR *vectorv1alpha1.VectorPipeline) string {
	return pipelineCR.Name + "-" + pipelineCR.Namespace
}

func AppendToMainConfig(main *VectorConfig, pipelineCR *vectorv1alpha1.VectorPipeline) *VectorConfig {
	configToAdd := NewVectorConfigFromCR(pipelineCR)
	for k, v := range configToAdd.Sources {
		main.Sources[k] = v
	}
	for i, j := range configToAdd.Sinks {
		main.Sinks[i] = j
	}
	return main
}
