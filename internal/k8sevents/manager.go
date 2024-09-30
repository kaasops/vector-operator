package k8sevents

import (
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

type EventsManager struct {
	client rest.Interface
	logger Logger
	mx     sync.Mutex
	mp     map[string]*watcher
}

func NewEventsManager(clientset *kubernetes.Clientset, logger Logger) *EventsManager {
	return &EventsManager{
		mp:     make(map[string]*watcher),
		client: clientset.CoreV1().RESTClient(),
		logger: logger,
	}
}

func (m *EventsManager) RegisterSubscriber(id, host, port, protocol, namespace string) {
	addr := net.JoinHostPort(host, port)
	c := newWatcher(protocol, addr, m.logger)

	m.mx.Lock()
	if oldC, ok := m.mp[id]; ok {
		oldC.close()
	}
	m.mp[id] = c
	m.mx.Unlock()

	c.watchEvents(m.client, namespace)
}

func (m *EventsManager) UnregisterSubscriber(id string) {
	m.mx.Lock()
	defer m.mx.Unlock()
	if v, ok := m.mp[id]; ok {
		v.close()
		delete(m.mp, id)
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
