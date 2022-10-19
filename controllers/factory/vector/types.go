package vector

import (
	"github.com/kaasops/vector-operator/api/v1alpha1"
)

type VectorConfig struct {
	DataDir    string                          `json:"data_dir,omitempty"`
	Api        *ApiSpec                        `json:"api,omitempty"`
	Sources    map[string]*v1alpha1.SourceSpec `json:"sources,omitempty"`
	Transforms map[string]interface{}          `json:"transforms,omitempty"`
	Sinks      map[string]interface{}          `json:"sinks,omitempty"`
}

type ApiSpec struct {
	Enabled    *bool   `json:"enabled,omitempty"`
	Address    *string `json:"address,omitempty"`
	Playground *bool   `json:"playground,omitempty"`
}
