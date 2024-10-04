package k8sevents

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net"
	"sync"
)

type Logger interface {
	Info(msg string, keysAndValues ...any)
	Error(err error, msg string, keysAndValues ...any)
}

type EventsCollector struct {
	client rest.Interface
	logger Logger
	mx     sync.Mutex
	mp     map[string]map[string]*watcher
}

func NewEventsCollector(clientset *kubernetes.Clientset, logger Logger) *EventsCollector {
	return &EventsCollector{
		mp:     make(map[string]map[string]*watcher),
		client: clientset.CoreV1().RESTClient(),
		logger: logger,
	}
}

func (m *EventsCollector) RegisterSubscriber(aggregatorID, svcName, svcNamespace, port, namespace string) {
	host := fmt.Sprintf("%s.%s", svcName, svcNamespace)
	addr := net.JoinHostPort(host, port)
	c := newWatcher(addr, namespace, m.logger)

	m.mx.Lock()

	if group, ok := m.mp[aggregatorID]; ok {

		if oldC, ok := group[host]; ok {
			if oldC.addr == addr && oldC.namespace == namespace {
				m.mx.Unlock()
				return
			}
			oldC.close()
		}

		group[host] = c

	} else {
		group = make(map[string]*watcher)
		group[host] = c
		m.mp[aggregatorID] = group
	}

	m.mx.Unlock()

	c.watchEvents(m.client)
}

func (m *EventsCollector) UnregisterSubscriber(aggregatorID, svcName, svcNamespace string) {
	host := fmt.Sprintf("%s.%s", svcName, svcNamespace)
	m.mx.Lock()
	defer m.mx.Unlock()

	if group, ok := m.mp[aggregatorID]; ok {
		if v, ok := group[host]; ok {
			v.close()
			delete(m.mp, host)
		}
	}
}

func (m *EventsCollector) UnregisterByAggregatorID(aggregatorID string) {
	m.mx.Lock()
	defer m.mx.Unlock()
	if group, ok := m.mp[aggregatorID]; ok {
		for host, w := range group {
			w.close()
			delete(m.mp, host)
		}
		delete(m.mp, aggregatorID)
	}
}
