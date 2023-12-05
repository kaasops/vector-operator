package config

const (
	// types
	KubernetesSourceType      = "kubernetes_logs"
	BlackholeSinkType         = "blackhole"
	InternalMetricsSourceType = "internal_metrics"
	InternalMetricsSinkType   = "prometheus_exporter"

	// default names
	DefaultSourceName                = "defaultSource"
	DefaultSinkName                  = "defaultSink"
	DefaultInternalMetricsSourceName = "internalMetricsSource"
	DefaultInternalMetricsSinkName   = "internalMetricsSink"
)

var (
	defaultSource = &Source{
		Name: DefaultSourceName,
		Type: KubernetesSourceType,
	}
	defaultSink = &Sink{
		Name:   DefaultSinkName,
		Type:   BlackholeSinkType,
		Inputs: []string{DefaultSourceName},
		Options: map[string]interface{}{
			"rate":                100,
			"print_interval_secs": 60,
		},
	}
	defaultPipelineConfig = PipelineConfig{
		Sources: map[string]*Source{
			DefaultSourceName: defaultSource,
		},
		Sinks: map[string]*Sink{
			DefaultSourceName: defaultSink,
		},
	}

	defaultInternalMetricsSource = &Source{
		Name: DefaultInternalMetricsSourceName,
		Type: InternalMetricsSourceType,
	}
	defaultInternalMetricsSink = &Sink{
		Name:   DefaultInternalMetricsSinkName,
		Type:   InternalMetricsSinkType,
		Inputs: []string{DefaultInternalMetricsSourceName},
	}
)
