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
  image: timberio/vector:0.48.0-debian
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
  image: timberio/vector:0.48.0-debian
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

## Spreading replicas across zones

Both aggregator types accept `spec.topologySpreadConstraints`, passed straight to the pod
template. Use it to keep replicas out of a single zone or node, so losing one of them does not
take the aggregator down with it.

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorAggregator
metadata:
  name: vectorAggregator1
  namespace: vector
spec:
  image: timberio/vector:0.48.0-debian
  replicas: 6
  topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: ScheduleAnyway
    labelSelector:
      matchLabels:
        app.kubernetes.io/instance: vectorAggregator1
        app.kubernetes.io/component: Aggregator
  - maxSkew: 1
    topologyKey: kubernetes.io/hostname
    whenUnsatisfiable: ScheduleAnyway
    labelSelector:
      matchLabels:
        app.kubernetes.io/instance: vectorAggregator1
        app.kubernetes.io/component: Aggregator
```

The example carries a second constraint because setting any constraint replaces the cluster
level defaults instead of adding to them. The scheduler's built in defaults spread with
`maxSkew: 3` on `kubernetes.io/hostname` and `maxSkew: 5` on zone, and they apply only to a pod
that defines none of its own. A zone only constraint therefore trades node level spreading
away: six replicas the scheduler used to place one per node can end up two per node, which
undoes the isolation the field is here for.

The label selector decides which pods are counted per domain, so it has to match the aggregator
pods. The operator labels them with `app.kubernetes.io/instance: <aggregator name>` and
`app.kubernetes.io/component: Aggregator`. Select on both, so one aggregator is spread without
catching another in the same namespace, and without counting the event collector pod, which
shares the instance label but is labelled `component: EventCollector`.

The constraints reach the event collector pod too, the same way `spec.affinity` and
`spec.tolerations` already do. It runs a single replica, so there is nothing there to spread.

`spec.affinity` cannot express this. Pod anti-affinity has two forms and neither fits:
`preferredDuringSchedulingIgnoredDuringExecution` is only a scheduling score, so under a
provisioner like Karpenter, which adds nodes to fit pending pods rather than to satisfy
preferences, replicas still end up packed onto one node. `requiredDuringSchedulingIgnoredDuringExecution`
has no count, so it allows a single pod per zone and any replica above the zone count stays
Pending, which rules it out for [autoscaling](aggregator-autoscaling.md).

Keep `whenUnsatisfiable: ScheduleAnyway`. With `DoNotSchedule` a zone outage or a scale up
past what the zones can hold leaves pods Pending rather than merely unevenly placed.

An even spread is a precondition for zone local routing, not a way to get it. The operator
creates plain ClusterIP services, so kube-proxy picks an endpoint anywhere in the cluster and
events still cross zones until topology aware routing is turned on for the service itself.
`spec.annotations` is applied to the aggregator services, so
`service.kubernetes.io/topology-mode: Auto` can be set there.

The field works the same way with [persistence](aggregator-persistence.md) enabled, since the
Deployment and the StatefulSet share one pod template. Keep in mind that a volume is bound to
the zone it was provisioned in, so a rescheduled replica goes back to its original zone no
matter what the constraint says.
