package config

const (
	// types
	AMQPType                  = "amqp"
	ApacheMetricsType         = "apache_metrics"
	AWSKinesisFirehoseType    = "aws_kinesis_firehose"
	AWSS3Type                 = "aws_s3"
	AWSSQSType                = "aws_sqs"
	DatadogAgentType          = "datadog_agent"
	DemoLogsType              = "demo_logs"
	DNSTapType                = "dnstap"
	DockerLogsType            = "docker_logs"
	EventStoreDBMetricsType   = "eventstoredb_metrics"
	FileType                  = "file"
	FluentType                = "fluent"
	GCPPubSubType             = "gcp_pubsub"
	HerokuLogsType            = "heroku_logs"
	HostMetricsType           = "host_metrics"
	HTTPClientType            = "http_client"
	HTTPServerType            = "http_server"
	InternalLogsType          = "internal_logs"
	InternalMetricsType       = "internal_metrics"
	JournaldType              = "journald"
	KafkaType                 = "kafka"
	KubernetesLogsType        = "kubernetes_logs"
	LogstashType              = "logstash"
	MongoDBMetricsType        = "mongodb_metrics"
	NATSType                  = "nats"
	NginxMetricsType          = "nginx_metrics"
	OpenTelemetryType         = "opentelemetry"
	PostgreSQLMetricsType     = "postgresql_metrics"
	PrometheusPushgatewayType = "prometheus_pushgateway"
	PrometheusRemoteWriteType = "prometheus_remote_write"
	PrometheusScrapeType      = "prometheus_scrape"
	PulsarType                = "pulsar"
	RedisType                 = "redis"
	SocketType                = "socket"
	SplunkHECType             = "splunk_hec"
	StatsDType                = "statsd"
	SyslogType                = "syslog"
	VectorType                = "vector"
	kubernetesEventsType      = "kubernetes_events"
)

var aggregatorTypes = map[string]struct{}{
	AMQPType:               {},
	AWSS3Type:              {},
	AWSSQSType:             {},
	AWSKinesisFirehoseType: {},
	DatadogAgentType:       {},
	FluentType:             {},
	GCPPubSubType:          {},
	HTTPClientType:         {},
	HTTPServerType:         {},
	HerokuLogsType:         {},
	InternalLogsType:       {},
	InternalMetricsType:    {},
	KafkaType:              {},
	LogstashType:           {},
	NATSType:               {},
	OpenTelemetryType:      {},
	PulsarType:             {},
	RedisType:              {},
	SocketType:             {},
	SplunkHECType:          {},
	StatsDType:             {},
	SyslogType:             {},
	VectorType:             {},
	kubernetesEventsType:   {},
}

var agentTypes = map[string]struct{}{
	ApacheMetricsType:         {},
	DNSTapType:                {},
	DemoLogsType:              {},
	DockerLogsType:            {},
	EventStoreDBMetricsType:   {},
	FileType:                  {},
	HTTPClientType:            {},
	HostMetricsType:           {},
	InternalLogsType:          {},
	InternalMetricsType:       {},
	JournaldType:              {},
	KubernetesLogsType:        {},
	MongoDBMetricsType:        {},
	NginxMetricsType:          {},
	OpenTelemetryType:         {},
	PostgreSQLMetricsType:     {},
	PrometheusPushgatewayType: {},
	PrometheusRemoteWriteType: {},
	PrometheusScrapeType:      {},
}

func isAggregator(name string) bool {
	_, ok := aggregatorTypes[name]
	return ok
}

func isAgent(name string) bool {
	_, ok := agentTypes[name]
	return ok
}
