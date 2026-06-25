
The `Autoscaling` feature enables the Vector Operator to automatically deploy, configure, and manage a native Kubernetes Horizontal Pod Autoscaler (HPA) resource for your Vector aggregator. 

> ⚠️ **Availability Note:** This section is exclusively available for the `VectorAggregator` and `ClusterVectorAggregator` Custom Resource Definitions (CRDs). It is not applicable to agent roles.

## Hierarchy

```yaml
spec:
  autoscaling:
    Enabled: <boolean>
    MinReplicas: <integer>
    MaxReplicas: <integer>
    Metrics: <array>
    Behaviors: <object>
```

---

## Configuration Example

```yaml
apiVersion: observability.timber.io/v1alpha1
kind: VectorAggregator
metadata:
  name: vector-aggregator
  namespace: logging
spec:
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    metrics:
      - type: Resource
        resource:
          name: cpu
          target:
            type: Utilization
            averageUtilization: 75
      - type: Resource
        resource:
          name: memory
          target:
            type: Utilization
            averageUtilization: 80
    behaviors:
      scaleUp:
        stabilizationWindowSeconds: 0
        policies:
          - type: Percent
            value: 100
            periodSeconds: 15
      scaleDown:
        stabilizationWindowSeconds: 300
        policies:
          - type: Percent
            value: 10
            periodSeconds: 60
```

---

For a deeper understanding of how Kubernetes handles horizontal autoscaling, metric evaluation, and stabilization windows under the hood, please refer to the official Kubernetes documentation:

* [Kubernetes Horizontal Pod Autoscaler (HPA) Overview](https://kubernetes.io)
* [Configuring HPA Scaling Behaviors](https://kubernetes.io#configurable-scaling-behavior)