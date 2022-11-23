# Secure credential

If you want collect logs from file (like k8s-audit logs) you can use example.

> Type `file` in source block work only in ClusterVectorPipeline. In VectorPipeline can use only `kubernetes_logs` type


```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: ClusterVectorPipeline
metadata:
  labels:
    app.kubernetes.io/instance: ams-infra-vector
  name: k9s-audit
  namespace: vector
spec:
  sources:
    k8s-audit:
      include:
      - /var/log/kubernetes/audit/kube-apiserver-audit.log
      type: file
  transforms:
    k8s-audit-transform:
      inputs:
      - k8s-audit
      source: |
        . = parse_json!(.message)

        .@timestamp = .stageTimestamp

        .cluster = "ams-infra"
      type: remap
  sinks:
    k8s-audit-sink:
      auth:
        password: ${ELASTIC_PASSWORD}
        strategy: basic
        user: ${ELASTIC_USER}
      bulk:
        index: k8s-audit-%Y-%m-%d
      endpoint: ${ELASTIC_HOST}
      inputs:
      - k8s-audit-transform
      tls:
        verify_certificate: false
      type: elasticsearch
```
