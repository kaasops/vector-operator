package vector

import "github.com/kaasops/vector-operator/api/v1alpha1"

type VectorConfig struct {
	DataDir string                          `json:"data_dir,omitempty"`
	Api     *ApiSpec                        `json:"api,omitempty"`
	Sources map[string]*v1alpha1.SourceSpec `json:"sources,omitempty"`
	// Transforms map[string]VectorTransform `yaml:"transforms,omitempty"`
	Sinks map[string]*v1alpha1.SinkSpec `json:"sinks,omitempty"`
}

type ApiSpec struct {
	Enabled    *bool   `json:"enabled,omitempty"`
	Address    *string `json:"address,omitempty"`
	Playground *bool   `json:"playground,omitempty"`
}

type Transform struct {
}

// type Source struct {
// 	Type          string `yaml:"type,omitempty"`
// 	LabelSelector string `yaml:"extra_label_selector,omitempty"`
// }

// type Sink struct {
// 	Type              string   `yaml:"type,omitempty"`
// 	Inputs            []string `yaml:"inputs,omitempty"`
// 	Encoding          Encoding `yaml:"encoding,omitempty"`
// 	Rate              int32    `yaml:"rate,omitempty"`
// 	PrintIntervalSecs int32    `yaml:"print_interval_secs,omitempty"`
// }

// type Encoding struct {
// 	Codec string `yaml:"codec,omitempty"`
// }
