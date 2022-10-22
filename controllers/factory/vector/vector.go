package vector

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
