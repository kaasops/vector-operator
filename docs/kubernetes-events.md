# [EXPERIMENTAL] Kubernetes events

The operator allows organizing the collection of events from the Kubernetes cluster in which it is deployed.
To do this, you need to deploy an aggregator and a pipeline.
The operator allows collecting events from the entire cluster or from a specific namespace.

## Namespace event collection

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorAggregator
metadata:
  name: vectorAggregator1
  namespace: vector
spec:
  image: timberio/vector:0.47.0-debian
  api:
    enabled: true
  replicas: 1
  tolerations:
  - effect: NoSchedule
    key: node-role.kubernetes.io/master
    operator: Exists
  - effect: NoSchedule
    key: node-role.kubernetes.io/control-plane
    operator: Exists
```

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorPipeline
metadata:
  name: vectorPipeline1
  namespace: vector
spec:
  sources:
    source-test:
      type: "kubernetes_events"
  sinks:
    sink-test:
      type: "console"
      encoding:
        codec: "json"
      inputs:
        - source-test
```

## Collection of all events in the cluster

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: ClusterVectorAggregator
metadata:
  name: clusterVectorAggregator1
spec:
  image: timberio/vector:0.47.0-debian
  resourceNamespace: default
  api:
    enabled: true
  replicas: 1
  tolerations:
  - effect: NoSchedule
    key: node-role.kubernetes.io/master
    operator: Exists
  - effect: NoSchedule
    key: node-role.kubernetes.io/control-plane
    operator: Exists
```

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: ClusterVectorPipeline
metadata:
  name: clusterVectorPipeline1
spec:
  sources:
    source-test:
      type: "kubernetes_events"
  sinks:
    sink-test:
      type: "console"
      encoding:
        codec: "json"
      inputs:
        - source-test
```
