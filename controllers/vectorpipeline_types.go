package controllers

type VectorConfig struct {
	DataDir    string                     `yaml:"data_dir,omitempty"`
	Api        VectorApiSpec              `yaml:"api,omitempty"`
	Sources    map[string]VectorSource    `yaml:"sources,omitempty"`
	Transforms map[string]VectorTransform `yaml:"transforms,omitempty"`
	Sinks      map[string]VectorSink      `yaml:"sinks,omitempty"`
}

type VectorApiSpec struct {
	Enabled    bool   `yaml:"enabled,omitempty"`
	Address    string `yaml:"address,omitempty"`
	Playground bool   `yaml:"playground,omitempty"`
}

type VectorTransform struct {
}

type VectorSource struct {
	Type          string `yaml:"type,omitempty"`
	LabelSelector string `yaml:"extra_label_selector,omitempty"`
}

type VectorSink struct {
	Type   string   `yaml:"type,omitempty"`
	Inputs []string `yaml:"inputs,omitempty"`
}
