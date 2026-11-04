# Aggregator

The operator allows deploying Vector in the cluster as an aggregator for remote processing, [more details about this](https://vector.dev/docs/setup/going-to-prod/arch/aggregator/).
Two types of resources are available for deploying aggregators in the cluster:
- VectorAggregator
- ClusterVectorAggregator

## Pipeline Services

The operator creates a Service for each aggregator pipeline that has a `kubernetes_events` source or a source with an `address` in the form `host:port`, such as `0.0.0.0:9000`. The Service has a port for each such source. The default Service name is `<aggregator_name>-aggregator-<pipeline_name>` in kebab case. The operator owns this Service in the aggregator's namespace (`spec.resourceNamespace` for a ClusterVectorAggregator). It overwrites a user Service with the same name: it sets the type to ClusterIP, replaces the selector, labels and ports, and sets the aggregator as its owner.

Do not give other Services in that namespace the full set of labels the operator uses to select its Services: `app.kubernetes.io/managed-by: vector-operator`, `app.kubernetes.io/name: vector`, `app.kubernetes.io/component: Aggregator`, and `app.kubernetes.io/instance: <aggregator_name>`. The operator deletes matching Services whose names it no longer needs.

If the API server rejects a Service of the aggregator as invalid (a pipeline Service, the API Service or the headless Service of the StatefulSet), the operator skips that Service and keeps reconciling the rest of the aggregator. For example, the API server rejects a Service name longer than 63 characters. A Service with such a name cannot exist, so clients cannot reach the pipeline through it. Use a shorter pipeline or aggregator name to fix it. A ValidatingAdmissionPolicy denial is handled the same way, unless the policy sets `reason` to something other than `Invalid`. The aggregator's `.status.reason` names the skipped Service and the API server error. `.status.configCheckResult` stays `true`, because the problem is in the Service, not in the config. While a Service is skipped, the operator reconciles the aggregator every 5 minutes, so it also picks up a fix made outside the aggregator and its pipelines, for example a changed admission policy. The operator clears the reason on the next successful reconcile in which no Service is skipped.

With persistence enabled, the headless Service of the StatefulSet is named `<aggregator_name>-aggregator-headless`. A pipeline Service with the same name, for example the Service of a pipeline named `headless`, is skipped and reported the same way, and the headless Service is kept. When persistence is turned on or off while such a pipeline exists, the operator deletes its own Service with that name and creates the headless Service or the pipeline Service instead, because a Service cannot switch between headless and a cluster IP in place. Use another pipeline name to fix it.

## VectorAggregator

The configuration for the aggregator is formed from valid vector pipelines in the same namespace.

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

## Draining pods on shutdown

Both aggregator types accept `spec.terminationGracePeriodSeconds` and `spec.lifecycle`. The
pod template receives the grace period, and the `vector` container receives the lifecycle.
Use them to keep a terminating pod serving while load balancers stop routing to it, so a
rolling restart or a scale down does not fail in-flight requests when Vector closes its
listeners.

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorAggregator
metadata:
  name: vectorAggregator1
  namespace: vector
spec:
  image: timberio/vector:0.48.0-debian
  replicas: 3
  terminationGracePeriodSeconds: 60
  lifecycle:
    preStop:
      sleep:
        seconds: 30
```

The grace period starts before the `preStop` hook runs and covers the hook and Vector's own
shutdown together. Allow enough time for both, or Vector may be killed before it finishes
draining. Vector's own shutdown timeout defaults to 60 seconds. Kubernetes uses 30 seconds
when the grace period is not set, so a 30-second sleep uses all of it.

Constraints the API server enforces on the CR:

- `sleep.seconds` must not exceed `terminationGracePeriodSeconds`, or 30 when it is not set.
  Kubernetes also rejects such a pod. The rule reports the error when you run `kubectl apply`.
- `sleep.seconds` must be greater than 0. Kubernetes before 1.33 rejects a zero sleep by
  default.
- `lifecycle.stopSignal` is rejected, because it needs pod `spec.os.name` and the operator
  does not set it.

The native `sleep` action needs no shell in the image and works with the distroless Vector
images. It is available in Kubernetes 1.30 and later.

The lifecycle applies only to the Vector container. The grace period is a pod setting, so it
also applies to the config reloader sidecar. These fields are not passed to the event
collector. It runs one replica and does not accept ingest traffic.
