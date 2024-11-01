package evcollector

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	eventsHandled = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "event_collector",
		Name:      "handled_events_total",
		Help:      "The total number of handled events",
	})
	eventsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "event_collector",
		Name:      "processed_events_total",
		Help:      "The total number of processed events",
	}, []string{"service", "namespace"})
)
