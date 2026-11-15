# Design
This document describes the design and interaction between the custom resource definitions (CRD) that the Vector Operator introduces.

Operator introduces the following custom resources:
- [Vector](#vector)
- [VectorPipeline](#vectorpipeline)
- [ClusterVectorPipeline](#clustervectorpipeline)
- [VectorAggregator](#vectoraggregator)
- [ClusterVectorAggregator](#clustervectoraggregator)

# Vector
The `Vector` CRD declaratively defines a Vector agent installation to run in a Kubernetes cluster.
For each Vector resource, the Operator deploys a properly configured DaemonSet in the same namespace.
For each Vector resource, the Operator adds:
- DaemonSet with Vector
- Secret with Vector Configuration file
- Service. (For connect to Vector API, or scrape Vector metrics, if enabled)
- ServiceAccount/ClusterRole/ClusterRoleBinding for get access to Kubernetes API

## Restrictions
- Currently tested only ONE installation Vector on Kubernetes cluster

## Planned
- Add features for compress Vector configuration file. (Delete duplicates sources/Transforms/Sinks. Compress to gzip)

## Specification
Specification access to [this](https://github.com/kaasops/vector-operator/blob/main/docs/specification.md#vector-spec) page


# VectorPipeline
The `VectorPipeline` is a namespace-scoped CRD.
The `VectorPipeline` CRD defines Sources, Transforms and Sinks rules for Vector.
All `VectorPipelines`, with validated configuration file, added to Vector configuration file.
The pipeline role is determined automatically based on source types, or pinned with `spec.role`:

**Agent role** (routed to Vector DaemonSet):
- Only [kubernetes_logs](https://vector.dev/docs/reference/configuration/sources/kubernetes_logs/) source type is allowed
- Collects logs only from the namespace where VectorPipeline is defined
- `extra_namespace_label_selector` is auto-configured to the pipeline's namespace; setting a different namespace will result in an error

**Aggregator role** (routed to VectorAggregator in the same namespace):
- Supports aggregator source types: `kafka`, `http_server`, `amqp`, `socket`, `redis`, `nats`, etc.
- No namespace restrictions on data sources

## Restrictions
- Without `spec.role`, a pipeline gets the agent role only if every source can run on the agent. If any source runs only on the aggregator, the whole pipeline gets the aggregator role and goes into the aggregator config, agent sources such as `kubernetes_logs` included. Keep agent and aggregator sources in separate pipelines.
- Some sources run in both roles, such as `opentelemetry`, `prometheus_remote_write` and `http_client`. Without `spec.role`, a pipeline with only such sources gets the agent role. When a `Vector` selects it, the pipeline is invalid, because the agent accepts only `kubernetes_logs` from a `VectorPipeline`. Set `spec.role: aggregator` to run it on the aggregator.
- With `spec.role`, every source must still be a known type that runs in the pinned role. A pinned
  aggregator cannot use a source that reads the node (`kubernetes_logs`, `file`, `journald`,
  `docker_logs`, `host_metrics`).
- When a source type does not run in the pinned role, `VALID` in `kubectl get vectorpipeline` is `false` and `.status.reason` explains why. `ROLE` keeps its previous value, or stays empty if the pipeline never had a role.
- Every aggregator replica runs the same config. A source that pulls data, such as `http_client` or `prometheus_scrape`, polls on each replica independently; the operator does not split the targets between replicas.

## Specification
Specification access to [this](https://github.com/kaasops/vector-operator/blob/main/docs/specification.md#vectorpipelinespec-clustervectorpipelinespec) page

# ClusterVectorPipeline
The `ClusterVectorPipeline` is a cluster-scoped CRD.
The `ClusterVectorPipeline` CRD defines Sources, Transforms and Sinks rules for Vector.
All `ClusterVectorPipelines`, with validated configuration file, added to Vector configuration file.
The pipeline role is determined automatically based on source types, or pinned with `spec.role`:

**Agent role** (routed to Vector DaemonSet):
- Supports all agent source types: `kubernetes_logs`, `file`, `journald`, `host_metrics`, `docker_logs`, etc.
- Can collect logs from any namespace
- No restrictions on `extra_namespace_label_selector`

**Aggregator role** (routed to ClusterVectorAggregator):
- Supports aggregator source types: `kafka`, `http_server`, `amqp`, `socket`, `redis`, `nats`, etc.

## Restrictions
- Without `spec.role`, a pipeline gets the agent role only if every source can run on the agent. If any source runs only on the aggregator, the whole pipeline gets the aggregator role and goes into the aggregator config, agent sources such as `kubernetes_logs` included. Keep agent and aggregator sources in separate pipelines.
- Some sources run in both roles, such as `opentelemetry`, `prometheus_remote_write` and `http_client`. Without `spec.role`, a pipeline with only such sources gets the agent role and runs in every pod of each `Vector` that selects it. For a listener such as `opentelemetry`, each of those pods listens on its ports, and no Service is created for them. Set `spec.role: aggregator` to run it on the ClusterVectorAggregator.
- With `spec.role`, every source must still be a known type that runs in the pinned role. A pinned
  aggregator cannot use a source that reads the node (`kubernetes_logs`, `file`, `journald`,
  `docker_logs`, `host_metrics`).
- When a source type does not run in the pinned role, `VALID` in `kubectl get clustervectorpipeline` is `false` and `.status.reason` explains why. `ROLE` keeps its previous value, or stays empty if the pipeline never had a role.
- Every aggregator replica runs the same config. A source that pulls data, such as `http_client` or `prometheus_scrape`, polls on each replica independently; the operator does not split the targets between replicas.

## Specification
Specification access to [this](https://github.com/kaasops/vector-operator/blob/main/docs/specification.md#vectorpipelinespec-clustervectorpipelinespec) page


# VectorAggregator
The `VectorAggregator` CRD declaratively defines a Vector aggregator installation to run in a Kubernetes cluster.
For each VectorAggregator resource, the Operator deploys a properly configured Deployment in the same namespace.
For each VectorAggregator resource, the Operator adds:
- Deployment with Vector
- Secret with Vector Configuration file
- Service. (For connect to Vector API, or scrape Vector metrics, if enabled)
- ServiceAccount/ClusterRole/ClusterRoleBinding for get access to Kubernetes API

## Restrictions
- It only forms a configuration from VectorPipelines in the same namespace as the aggregator

## Specification
Specification access to [this](https://github.com/kaasops/vector-operator/blob/main/docs/specification.md#vectoraggregator-spec) page


# ClusterVectorAggregator
The `ClusterVectorAggregator` is a cluster-scoped CRD that works like VectorAggregator but with ClusterVectorPipelines.

## Restrictions
- It works only with ClusterVectorPipelines

