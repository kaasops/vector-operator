package vector

type VectorConfig struct {
	DataDir    string                 `json:"data_dir,omitempty"`
	Api        *ApiSpec               `json:"api,omitempty"`
	Sources    map[string]interface{} `json:"sources,omitempty"`
	Transforms map[string]interface{} `json:"transforms,omitempty"`
	Sinks      map[string]interface{} `json:"sinks,omitempty"`
}

type ApiSpec struct {
	Enabled    *bool   `json:"enabled,omitempty"`
	Address    *string `json:"address,omitempty"`
	Playground *bool   `json:"playground,omitempty"`
}

type Source struct {
	Name    string
	Type    string                 `mapper:"type"`
	Options map[string]interface{} `mapstructure:",remain"`
}

type Transform struct {
	Name    string
	Type    string                 `mapper:"type"`
	Inputs  []string               `mapper:"inputs"`
	Options map[string]interface{} `mapstructure:",remain"`
}

type Sink struct {
	Name    string
	Type    string                 `mapper:"type"`
	Inputs  []string               `mapper:"inputs"`
	Options map[string]interface{} `mapstructure:",remain"`
}

type ConfigComponent interface {
	GetOptions() map[string]interface{}
}
