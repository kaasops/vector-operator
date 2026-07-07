# Monitoring

How to monitor the operator and the vector workloads it manages, and how to install the
bundled Grafana dashboard.

## What is exposed

| Target | Metrics | Port | How to enable |
|---|---|---|---|
| Operator | controller-runtime: `controller_runtime_reconcile_*`, `workqueue_*`, `rest_client_requests_total`, `process_*`, `go_*` | `:8080` (or `:8443` secure) | `metrics.enabled=true` helm value |
| Vector agents / aggregators | vector internal metrics: `vector_component_*`, `vector_utilization`, `vector_buffer_*`, `vector_source_lag_time_seconds`, `vector_open_files` | `:9598` | `spec.agent.internalMetrics: true` on the Vector CR / `spec.internalMetrics: true` on (Cluster)VectorAggregator |
| Event collector | `event_collector_{handled,skipped,processed}_events_total` | `:8080` | deployed with the aggregator when a selected pipeline has a `kubernetes_events` source |

## Operator metrics via helm

```yaml
metrics:
  enabled: true
  podMonitor:
    enabled: true              # requires the Prometheus Operator CRDs
    additionalLabels:
      release: kube-prometheus-stack   # so your Prometheus selects it
```

This adds a named `metrics` container port, passes `--metrics-bind-address` (plain HTTP,
`--metrics-secure=false`) to the operator and creates a PodMonitor. With all `metrics.*`
values left at defaults the rendered manifests are unchanged.

### Secure mode

```yaml
metrics:
  enabled: true
  secure: true                 # HTTPS :8443 with authn/authz (TokenReview)
  podMonitor:
    enabled: true
    createScrapeIdentity: true
```

In secure mode every scrape is authenticated and authorized, so the chart additionally
creates: a ClusterRole allowing the operator to issue TokenReviews/SubjectAccessReviews,
and (with `createScrapeIdentity`) a dedicated ServiceAccount with a long-lived token
Secret plus a `get /metrics` ClusterRole; the PodMonitor sends that token as the
Authorization header (a PodMonitor cannot read the Prometheus pod's own token file).
Requests without a token get 401. Instead of `createScrapeIdentity` you can point
`metrics.podMonitor.bearerTokenSecret` at an existing Secret; one of the two is required.
Two trade-offs to know: the created token does not expire (rotate it by deleting the
Secret — the token controller repopulates it), and the scrape uses
`insecureSkipVerify` because the operator serves a self-signed certificate.

## Vector metrics

Set `internalMetrics: true` on the CR (`spec.agent.internalMetrics` for Vector,
`spec.internalMetrics` for aggregators). The operator then injects an
`internal_metrics` source with a `prometheus_exporter` sink (port 9598) into the generated config
and creates a PodMonitor for the workload **in the workload's namespace**. By default that
PodMonitor carries no extra labels, so a Prometheus with a label-based
`podMonitorSelector` (kube-prometheus-stack's default) will not pick it up. The CR's
`labels` field propagates to the PodMonitor — the usual fix is one line on the CR:

```yaml
spec:
  agent:                      # spec.labels for aggregators
    labels:
      release: kube-prometheus-stack
```

(alternatively, configure the Prometheus selector or create your own monitor; note the
`labels` also land on the workload's pods and service).

The event collector exposes `/metrics` on its `metrics` service port (8080); no monitor
is created for it — a minimal ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: vector-event-collector
  labels:
    release: kube-prometheus-stack
spec:
  namespaceSelector:
    matchNames: [<aggregator namespace>]
  selector:
    matchLabels:
      app.kubernetes.io/component: EventCollector
  endpoints:
    - port: metrics
```

## Grafana dashboard

The dashboard ([`helm/charts/vector-operator/dashboards/vector-operator.json`](../helm/charts/vector-operator/dashboards/vector-operator.json))
opens with an overview stat strip and has three rows: operator (reconciliation,
workqueues, API-server load, process), agents (events flow, errors, buffers, source lag)
and aggregators + event collector.

Install via helm:

```yaml
grafanaDashboard:
  enabled: true
  namespace: monitoring   # the kube-prometheus-stack sidecar only watches its own namespace by default
```

or import the JSON manually in the Grafana UI (Dashboards, Import).

Dashboard variables map to scrape jobs: `operator_job`, `agent_job`, `aggregator_job`
and `evc_job` — agents and aggregators expose identical vector metrics, so they are told
apart by which job scraped them; pick the matching job in each variable. The `group_by`
variable switches the per-component panels between `component_type` (default, stays
readable and cheap at any scale) and `component_id` (drill-down to a single pipeline
component).

## Cardinality at scale

Every agent pod carries the merged config of all selected pipelines, and vector exports
per-component series: at thousands of pipelines this reaches hundreds of thousands of
lines per pod (the `vector_source_lag_time_seconds` histogram alone is ~20 series per
`kubernetes_logs` source). Before scraping large installations:

- drop what you don't chart, e.g. keep the lag histogram sum/count but drop its buckets:

  ```yaml
  podMetricsEndpoints:
    - port: prom-exporter
      metricRelabelings:
        - sourceLabels: [__name__]
          regex: vector_source_lag_time_seconds_bucket
          action: drop
  ```

- set `expireMetricsSecs` on the CR so vector stops exporting series for components that
  no longer emit (stale pipelines otherwise accumulate);
- keep the dashboard's `group_by` variable on `component_type`; error and buffer-discard
  metrics are sparse (they only exist for failing components) and stay cheap either way.