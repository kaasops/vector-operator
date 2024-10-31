package evcollector

import (
	"context"
	"github.com/kaasops/vector-operator/internal/vector/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"time"
)

type Logger interface {
	Info(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

type Collector struct {
	Addr      string
	Namespace string
	createdAt time.Time
	stopCh    chan struct{}
	logger    Logger
	client    rest.Interface
}

func New(addr, namespace string, logger Logger, client rest.Interface) *Collector {
	c := Collector{
		Addr:      addr,
		createdAt: time.Now(),
		logger:    logger,
		Namespace: namespace,
		client:    client,
	}
	return &c
}

func (c *Collector) Start() {
	if c.stopCh != nil {
		return
	}

	c.stopCh = make(chan struct{})
	eventsCh := make(chan *corev1.Event)

	watchList := cache.NewListWatchFromClient(c.client, "events", c.Namespace, fields.Everything())
	_, ctrl := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: watchList,
		ObjectType:    &corev1.Event{},
		ResyncPeriod:  0,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				event := obj.(*corev1.Event)
				eventsCh <- event
			},
			UpdateFunc: func(_, obj interface{}) {
				event := obj.(*corev1.Event)
				eventsCh <- event
			},
		},
	})
	go func() {
		var conn *grpc.ClientConn
		var vectorClient gen.VectorClient
		var err error
		var event *corev1.Event
		var sending bool

		for {
			select {
			case <-c.stopCh:
				if conn != nil {
					_ = conn.Close()
				}
				return

			default:
				if !sending {
					select {
					case event = <-eventsCh:
						if eventTimestamp(event).Before(c.createdAt) || event == nil {
							continue
						}
						sending = true
					case <-c.stopCh:
						if conn != nil {
							_ = conn.Close()
						}
						return
					}
				}

				if conn == nil {
					for {
						conn, err = grpc.NewClient(c.Addr,
							grpc.WithTransportCredentials(insecure.NewCredentials()),
						)
						if err != nil {
							c.logger.Error("connect to address", "address", c.Addr, "error", err)
							time.Sleep(5 * time.Second)
							continue
						}
						vectorClient = gen.NewVectorClient(conn)
						break
					}
				}

				_, err = vectorClient.PushEvents(context.Background(), &gen.PushEventsRequest{
					Events: []*gen.EventWrapper{{
						Event: &gen.EventWrapper_Log{
							Log: k8sEventToVectorLog(event),
						},
					}},
				})
				if err != nil {
					c.logger.Error("send event", "address", c.Addr, "error", err)
					_ = conn.Close()
					conn = nil
					time.Sleep(5 * time.Second)
					continue
				}

				sending = false
			}
		}

	}()
	go ctrl.Run(c.stopCh)
}

func (c *Collector) Stop() {
	close(c.stopCh)
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
