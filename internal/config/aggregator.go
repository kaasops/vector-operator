package config

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/stoewer/go-strcase"
	corev1 "k8s.io/api/core/v1"
)

func BuildAggregatorConfig(params VectorConfigParams, pipelines ...pipeline.Pipeline) (*VectorConfig, error) {
	cfg := newVectorConfig(params)

	cfg.Sources = make(map[string]*Source)
	cfg.Transforms = make(map[string]*Transform)
	cfg.Sinks = make(map[string]*Sink)

	cfg.internal.servicePort = make(map[string]*ServicePort)

	var kubernetesEventsPort int32 = 42000

	for _, pipeline := range pipelines {
		kubernetesEventsAlreadyExists := false
		p := &PipelineConfig{}
		if err := UnmarshalJson(pipeline.GetSpec(), p); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pipeline %s: %w", pipeline.GetName(), err)
		}
		for k, v := range p.Sources {
			settings := v

			switch v.Type {
			case kubernetesEventsType:
				{
					if kubernetesEventsAlreadyExists {
						return nil, fmt.Errorf("pipeline can only contain one source with the type kubernetes_events")
					}
					kubernetesEventsAlreadyExists = true
					address := net.JoinHostPort(net.IPv4zero.String(), strconv.Itoa(int(kubernetesEventsPort)))
					settings = &Source{
						Name: k,
						Type: VectorType,
						Options: map[string]any{
							"address": address,
						},
					}
					err := cfg.internal.addServicePort(&ServicePort{
						IsKubernetesEvents: true,
						Port:               kubernetesEventsPort,
						Protocol:           corev1.ProtocolTCP,
						Namespace:          pipeline.GetNamespace(),
						SourceName:         k,
						PipelineName:       pipeline.GetName(),
						ServiceName:        getServiceName(pipeline.GetAnnotations()[common.AnnotationServiceName], params.AggregatorName, pipeline.GetName()),
					})
					if err != nil {
						return nil, err
					}
					kubernetesEventsPort++
				}
			default:
				{
					if val, ok := v.Options["address"]; ok {
						address, _ := val.(string)
						if _, port, err := net.SplitHostPort(address); err == nil {
							portN, err := parsePort(port)
							if err != nil {
								return nil, fmt.Errorf("failed to parse port %s: %w", port, err)
							}
							protocol := extractProtocol(v.Options)
							err = cfg.internal.addServicePort(&ServicePort{
								Port:         portN,
								Protocol:     protocol,
								Namespace:    pipeline.GetNamespace(),
								SourceName:   k,
								PipelineName: pipeline.GetName(),
								ServiceName:  getServiceName(pipeline.GetAnnotations()[common.AnnotationServiceName], params.AggregatorName, pipeline.GetName()),
							})
							if err != nil {
								return nil, err
							}
						}
					}
				}
			}

			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k, pipeline.SkipPrefix())
			cfg.Sources[v.Name] = settings
		}
		for k, v := range p.Transforms {
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k, pipeline.SkipPrefix())
			for i, inputName := range v.Inputs {
				v.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName, pipeline.SkipPrefix())
			}
			cfg.Transforms[v.Name] = v
		}
		for k, v := range p.Sinks {
			v.Name = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), k, pipeline.SkipPrefix())
			for i, inputName := range v.Inputs {
				v.Inputs[i] = addPrefix(pipeline.GetNamespace(), pipeline.GetName(), inputName, pipeline.SkipPrefix())
			}
			cfg.Sinks[v.Name] = v
		}
	}

	// Add exporter pipeline
	if params.InternalMetrics && !isExporterSinkExists(cfg.Sinks) {
		cfg.Sources[DefaultInternalMetricsSourceName] = defaultInternalMetricsSource
		cfg.Sinks[DefaultInternalMetricsSinkName] = defaultInternalMetricsSink
	}
	if len(cfg.Sources) == 0 && len(cfg.Sinks) == 0 {
		cfg.PipelineConfig = defaultAggregatorPipelineConfig
		err := cfg.internal.addServicePort(&ServicePort{
			Port:         DefaultAggregatorSourcePort,
			Protocol:     corev1.ProtocolTCP,
			Namespace:    DefaultNamespace,
			SourceName:   DefaultInternalMetricsSourceName,
			PipelineName: DefaultPipelineName,
			ServiceName:  getServiceName("", params.AggregatorName, DefaultPipelineName),
		})
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func parsePort(port string) (int32, error) {
	p, err := strconv.ParseInt(port, 10, 32)
	if err != nil {
		return 0, err
	}
	if p < 0 || p > 65535 {
		return 0, errors.New("port out of range")
	}
	return int32(p), nil
}

func extractProtocol(opts map[string]any) corev1.Protocol {
	protocol := corev1.ProtocolTCP
	if val, ok := opts["mode"]; ok {
		if s, ok := val.(string); ok && strings.ToLower(s) == "udp" {
			return corev1.ProtocolUDP
		}
	}
	return protocol
}

func getServiceName(nameFromAnnotations, aggregatorName, pipelineName string) string {
	if nameFromAnnotations != "" {
		return nameFromAnnotations
	}
	return strcase.KebabCase(fmt.Sprintf("%s-aggregator-%s", aggregatorName, pipelineName))
}
