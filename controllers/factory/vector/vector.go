package vector

import (
	"github.com/mitchellh/mapstructure"
)

func New(dataDir string, apiEnabled bool) *VectorConfig {
	sources := make(map[string]interface{})
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

func Mapper(c ConfigComponent) (map[string]interface{}, error) {
	spec := make(map[string]interface{})
	spec = c.GetOptions()
	config := &mapstructure.DecoderConfig{
		Result:               &spec,
		ZeroFields:           false,
		TagName:              "mapper",
		IgnoreUntaggedFields: true,
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, err
	}
	err = decoder.Decode(c)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func (t Source) GetOptions() map[string]interface{} {
	return t.Options
}

func (t Transform) GetOptions() map[string]interface{} {
	return t.Options
}

func (t Sink) GetOptions() map[string]interface{} {
	return t.Options
}
