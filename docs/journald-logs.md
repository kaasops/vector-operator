# Secure credential

If you want collect service journald logs from node you can use example.

> Type `journald` in source block work only in ClusterVectorPipeline. In VectorPipeline can use only `kubernetes_logs` type

> If you want collect journald logs, needs to use vector-agent container with journalctl. `timberio/vector:0.48.0-debian` - for example


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
