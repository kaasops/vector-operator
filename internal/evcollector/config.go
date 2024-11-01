package evcollector

type ReceiverParams struct {
	ServiceName      string
	ServiceNamespace string
	Port             string
	WatchedNamespace string
}

type Config struct {
	MaxBatchSize int32
	Receivers    []*ReceiverParams
}
