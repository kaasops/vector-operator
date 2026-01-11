# Secure credential

If you want to collect service journald logs from node you can use example.

> Type `journald` is an agent source type that requires node-level access. In VectorPipeline with an agent role, only `kubernetes_logs` is allowed. Use ClusterVectorPipeline for `journald` sources.

> If you want to collect journald logs, needs to use vector-agent container with journalctl. `timberio/vector:0.48.0-debian` - for example


```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: ClusterVectorPipeline
metadata:
  name: journald
  namespace: vector
spec:
  sources:
    containerd:
      include_units:
      - containerd
      type: journald
    kubelet:
      include_units:
      - kubelet
      type: journald
  transforms:
    containerd-transform:
      inputs:
      - containerd
      source: |
        . = parse_regex!(.message, r'^time="(?P<time>.*)" level=(?P<loglevel>[\w]+.) msg="(?P<message>.*)"$')
        .@timestamp = .time
      type: remap
    kubelet-transform:
      inputs:
      - kubelet
      source: |
        . = parse_json!(.message)
        .@timestamp = .ts
      type: remap
  sinks:
    jornald-sink:
      auth:
        password: ${ELASTIC_PASSWORD}
        strategy: basic
        user: ${ELASTIC_USER}
      bulk:
        index: journald-%Y-%m-%d
      endpoint: ${ELASTIC_HOST}
      inputs:
      - containerd-transform
      - kubelet-transform
      tls:
        verify_certificate: false
      type: elasticsearch
```
