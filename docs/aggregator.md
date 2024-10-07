# Aggregator

The operator allows deploying Vector in the cluster as an aggregator for remote processing, [more details about this](https://vector.dev/docs/setup/going-to-prod/arch/aggregator/). 
Two types of resources are available for deploying aggregators in the cluster:
- VectorAggregator
- ClusterVectorAggregator

## VectorAggregator

“The configuration for the aggregator is formed from valid vector pipelines in the same namespace.
The Service name is generated as follows: <aggregator_name>-aggregator-<vp_name>.”

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorAggregator
metadata:
  name: vectorAggregator1
  namespace: vector
spec:
  image: timberio/vector:0.40.0-debian
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
      type: "socket"
      address: "0.0.0.0:9000"
      mode: tcp
  sinks:
    sink-test:
      type: "console"
      encoding:
        codec: "json"
      inputs:
        - source-test
```

## ClusterVectorAggregator

ClusterVectorAggregator works similarly to VectorAggregator, but ClusterVectorPipeline is used for configuration formation.
To deploy resources (Deployment, Service), you need to specify resourceNamespace in the specification.

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: ClusterVectorAggregator
metadata:
  name: clusterVectorAggregator1
spec:
  image: timberio/vector:0.40.0-debian
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
      type: "socket"
      mode: "tcp"
      address: "0.0.0.0:9000"
  sinks:
    sink-test:
      inputs:
        - source-test
      type: "elasticsearch"
      api_version: auto
      endpoints:
        - https://test-elastic-http.default:9200
      mode: bulk
      tls:
        verify_certificate: false
      bulk:
        action: create
        index: "test-%Y-%m-%d"
      auth:
        user: elastic
        password: test-password
        strategy: basic
```