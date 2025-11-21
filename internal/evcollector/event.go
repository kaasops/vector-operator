package evcollector

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/kaasops/vector-operator/internal/vector/gen"
)

func k8sEventToVectorLog(ev *corev1.Event) *gen.Log {
	return &gen.Log{
		Value: &gen.Value{
			Kind: &gen.Value_Map{
				Map: &gen.ValueMap{Fields: map[string]*gen.Value{
					"event": {Kind: &gen.Value_Map{
						Map: &gen.ValueMap{Fields: map[string]*gen.Value{
							"uid":               valueFromString(string(ev.UID)),
							"message":           valueFromString(ev.Message),
							"reason":            valueFromString(ev.Reason),
							"action":            valueFromString(ev.Action),
							"type":              valueFromString(ev.Type),
							"creationTimestamp": valueFromString(ev.CreationTimestamp.Format(time.RFC3339)),
							"firstTimestamp":    valueFromString(ev.FirstTimestamp.Format(time.RFC3339)),
							"lastTimestamp":     valueFromString(ev.LastTimestamp.Format(time.RFC3339)),
							"name":              valueFromString(ev.Name),
							"namespace":         valueFromString(ev.Namespace),
							"source": {Kind: &gen.Value_Map{
								Map: &gen.ValueMap{Fields: map[string]*gen.Value{
									"host":      valueFromString(ev.Source.Host),
									"component": valueFromString(ev.Source.Component),
								}},
							}},
							"involvedObject": {Kind: &gen.Value_Map{
								Map: &gen.ValueMap{Fields: map[string]*gen.Value{
									"uid":             valueFromString(string(ev.InvolvedObject.UID)),
									"kind":            valueFromString(ev.InvolvedObject.Kind),
									"name":            valueFromString(ev.InvolvedObject.Name),
									"namespace":       valueFromString(ev.InvolvedObject.Namespace),
									"apiVersion":      valueFromString(ev.InvolvedObject.APIVersion),
									"resourceVersion": valueFromString(ev.InvolvedObject.ResourceVersion),
									"fieldPath":       valueFromString(ev.InvolvedObject.FieldPath),
								}},
							}},
						}},
					}},
				}},
			},
		},
	}
}

func k8sEventsToVectorEvents(list []*corev1.Event) *gen.PushEventsRequest {
	events := make([]*gen.EventWrapper, 0, len(list))
	for _, event := range list {
		events = append(events, &gen.EventWrapper{
			Event: &gen.EventWrapper_Log{
				Log: k8sEventToVectorLog(event),
			},
		})
	}
	return &gen.PushEventsRequest{
		Events: events,
	}
}

func valueFromString(s string) *gen.Value {
	return &gen.Value{
		Kind: &gen.Value_RawBytes{RawBytes: []byte(s)},
	}
}
