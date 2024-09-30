package k8sevents

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

type watcher struct {
	protocol  string
	addr      string
	createdAt time.Time
	stopCh    chan struct{}
	logger    Logger
}

func newWatcher(protocol, addr string, logger Logger) *watcher {
	r := watcher{
		addr:      addr,
		protocol:  protocol,
		createdAt: time.Now(),
		logger:    logger,
	}
	return &r
}

func (w *watcher) watchEvents(client rest.Interface, namespace string) {
	if w.stopCh != nil {
		return
	}

	w.stopCh = make(chan struct{})
	eventsCh := make(chan *corev1.Event)

	watchList := cache.NewListWatchFromClient(client, "events", namespace, fields.Everything())
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
			case <-w.stopCh:
				if conn != nil {
					_ = conn.Close()
				}
				return

			default:
				if !sending {
					select {
					case event = <-eventsCh:
						if eventTimestamp(event).Before(w.createdAt) || event == nil {
							continue
						}
						sending = true
					case <-w.stopCh:
						if conn != nil {
							_ = conn.Close()
						}
						return
					}
				}

				if conn == nil {
					for {
						conn, err = grpc.NewClient(w.addr,
							grpc.WithTransportCredentials(insecure.NewCredentials()),
						)
						if err != nil {
							w.logger.Error(err, "connect to address", "address", w.addr)
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
					w.logger.Error(err, "send event", "address", w.addr)
					_ = conn.Close()
					conn = nil
					time.Sleep(5 * time.Second)
					continue
				}

				sending = false
			}
		}

	}()
	go ctrl.Run(w.stopCh)
}

func (w *watcher) close() {
	close(w.stopCh)
}
