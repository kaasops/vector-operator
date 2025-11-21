package config

import (
	"net"
	"strconv"
)

const (
	// types
	BlackholeSinkType      = "blackhole"
	PrometheusExporterType = "prometheus_exporter"

	// default names
	DefaultSourceName                = "defaultSource"
	DefaultSinkName                  = "defaultSink"
	DefaultInternalMetricsSourceName = "internalMetricsSource"
	DefaultInternalMetricsSinkName   = "internalMetricsSink"
	DefaultInternalMetricsSinkPort   = 9598
	DefaultAggregatorSourcePort      = 8989
	DefaultNamespace                 = "default"
	DefaultPipelineName              = "default-pipeline"
)

var (
	defaultAgentSource = &Source{
		Name: DefaultSourceName,
		Type: KubernetesLogsType,
	}
	defaultAggregatorSource = &Source{
		Name: DefaultSourceName,
		Type: VectorType,
		Options: map[string]any{
			"address": net.JoinHostPort(net.IPv6zero.String(), strconv.Itoa(DefaultAggregatorSourcePort)),
		},
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
	defaultAgentPipelineConfig = PipelineConfig{
		Sources: map[string]*Source{
			DefaultSourceName: defaultAgentSource,
		},
		Sinks: map[string]*Sink{
			DefaultSinkName: defaultSink,
		},
	}
	defaultAggregatorPipelineConfig = PipelineConfig{
		Sources: map[string]*Source{
			DefaultSourceName: defaultAggregatorSource,
		},
		Sinks: map[string]*Sink{
			DefaultSinkName: defaultSink,
		},
	}

	defaultInternalMetricsSource = &Source{
		Name: DefaultInternalMetricsSourceName,
		Type: InternalMetricsType,
	}
	defaultInternalMetricsSink = &Sink{
		Name:   DefaultInternalMetricsSinkName,
		Type:   PrometheusExporterType,
		Inputs: []string{DefaultInternalMetricsSourceName},
		Options: map[string]any{
			"address": net.JoinHostPort(net.IPv6zero.String(), strconv.Itoa(DefaultInternalMetricsSinkPort)),
		},
	}
)
