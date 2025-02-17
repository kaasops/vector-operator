# Secure credential

If you need to use sensitive credentials (such as host, username, or password for Elasticsearch), you can consider the following approaches:
- envFrom with secretRef (recommended)
- environment variables.

## envFrom

Create a secret:

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: mysecret
  namespace: vector
data:
  ELASTIC_HOST: "base64_host"
  ELASTIC_USER: "base64_user"
  ELASTIC_PASSWORD: "base64_password"
```

Deploy CR Vector to Kubernetes by specifying a reference to the secret with Elastic parameters:
```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: Vector
metadata:
  name: example
  namespace: vector
spec:
  agent:
    envFrom:
      - secretRef:
        name: mysecret
...
```

## Environment variables

Deploy CR Vector to Kubernetes where set credentials for Elastic in ENVs:
```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: Vector
metadata:
  name: example
  namespace: vector
spec:
  agent:
    env:
    - name: ELASTIC_HOST
      value: {{HOST}}
    - name: ELASTIC_USER
      value: {{USER}}
    - name: ELASTIC_PASSWORD
      value: {{PASSWORD}}
```

Now you can use this ENVs in CR VectorPipeline, like:
```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorPipeline
metadata:
  name: example
  namespace: vector
spec:
  sources:
    example:
      extra_label_selector: app=example
      type: kubernetes_logs
  transforms:
    example-transform:
      inputs:
      - example
      source: |
        . = parse_json!(.message)

        .@timestamp = .time

        .cluster = "example"
      type: remap
  sinks:
    elastic:
      auth:
        password: ${ELASTIC_PASSWORD}
        strategy: basic
        user: ${ELASTIC_USER}
      bulk:
        index: example-%Y-%m-%d
      endpoint: ${ELASTIC_HOST}
      inputs:
      - example-transform
      tls:
        verify_certificate: false
      type: elasticsearch
```

With this scheme, if developers have access only to CR `VectorPipeline`, they can use credential from ENVs, but don't see them.
