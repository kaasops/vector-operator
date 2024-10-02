package k8sevents

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net"
	"sync"
	"time"
)

type Logger interface {
	Info(msg string, keysAndValues ...any)
	Error(err error, msg string, keysAndValues ...any)
}

type EventsCollector struct {
	client rest.Interface
	logger Logger
	mx     sync.Mutex
	mp     map[string]*watcher
}

func NewEventsCollector(clientset *kubernetes.Clientset, logger Logger) *EventsCollector {
	return &EventsCollector{
		mp:     make(map[string]*watcher),
		client: clientset.CoreV1().RESTClient(),
		logger: logger,
	}
}

func (m *EventsCollector) RegisterSubscriber(svcName, svcNamespace, port, namespace string) {
	host := fmt.Sprintf("%s.%s", svcName, svcNamespace)
	addr := net.JoinHostPort(host, port)
	c := newWatcher(addr, namespace, m.logger)

	m.mx.Lock()
	if oldC, ok := m.mp[host]; ok {
		if oldC.addr == addr && oldC.namespace == namespace {
			m.mx.Unlock()
			return
		}
		oldC.close()
	}
	m.mp[host] = c
	m.mx.Unlock()

	c.watchEvents(m.client)
}

func (m *EventsCollector) UnregisterSubscriber(svcName, svcNamespace string) {
	host := fmt.Sprintf("%s.%s", svcName, svcNamespace)
	m.mx.Lock()
	defer m.mx.Unlock()
	if v, ok := m.mp[host]; ok {
		v.close()
		delete(m.mp, host)
	}
}

func eventTimestamp(ev *corev1.Event) time.Time {
	var ts time.Time
	switch {
	case ev.EventTime.Time != time.Time{}:
		ts = ev.EventTime.Time
	case ev.LastTimestamp.Time != time.Time{}:
		ts = ev.LastTimestamp.Time
	case ev.FirstTimestamp.Time != time.Time{}:
		ts = ev.FirstTimestamp.Time
	}
	return ts
}
