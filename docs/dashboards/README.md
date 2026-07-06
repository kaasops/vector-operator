# Dashboards

The Grafana dashboard for the operator and its vector workloads ships inside the helm
chart: [`helm/charts/vector-operator/dashboards/vector-operator.json`](../../helm/charts/vector-operator/dashboards/vector-operator.json).

Ways to install it:

- **Helm (recommended):** set `grafanaDashboard.enabled=true` — the chart ships the
  dashboard as a ConfigMap labelled for a Grafana dashboard sidecar
  (e.g. kube-prometheus-stack). See [docs/monitoring.md](../monitoring.md).
- **Manual:** import the JSON file in the Grafana UI (Dashboards, Import) or via the HTTP API.

The dashboard expects the metrics described in [docs/monitoring.md](../monitoring.md):
the operator metrics endpoint (`metrics.enabled=true`) and vector internal metrics
(`spec.agent.internalMetrics: true` / `spec.internalMetrics: true` on the CRs).
