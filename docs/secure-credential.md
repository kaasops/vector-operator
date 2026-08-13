# Secure credential

On Vector images below 0.57, you can use sensitive credentials (such as host, username, or password for Elasticsearch) through either of the following approaches:
- envFrom with secretRef (recommended of the two - see why below)
- environment variables.

On Vector 0.57+, use [Pipeline secrets](secrets.md) (`spec.secret`) instead - it is the operator's primary way to handle credentials now and the only one of the three that works on 0.57+ without an opt-in flag; see the warning below for why.

> **Warning: Vector 0.57+ breaks this mechanism by default.** Starting with Vector 0.57.0, `${VAR}` interpolation in configuration files is disabled by default (see the [0.57 upgrade guide](https://vector.dev/highlights/2026-07-14-0-57-0-upgrade-guide/)). The operator does not pass `--dangerously-allow-env-var-interpolation` to the vector containers, so on Vector images >= 0.57 the `${VAR}` references below reach the sinks as literal strings: Vector starts without a single warning, and `vector validate` (the operator's configcheck) still reports the config as valid, so the breakage is fully silent. If you rely on this mechanism, migrate to [Pipeline secrets](secrets.md), which uses Vector's secrets mechanism and works on 0.57+ without any opt-in flag. To keep the old behavior while you migrate, the flag has an environment-variable equivalent the operator does pass through: set `VECTOR_DANGEROUSLY_ALLOW_ENV_VAR_INTERPOLATION` to `true` in the workload's `env` (`spec.agent.env` for a Vector, `spec.env` for an aggregator). It re-enables interpolation for the whole workload - every `${...}` in every pipeline sharing it - so it is a bridge for an image upgrade, not a destination. Pinning the image below 0.57 works too, for as long as that is an option.

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

## See also

If each pipeline needs its own credentials, scoped to its own namespace, without the Vector CR owner in the loop, see [Pipeline secrets](secrets.md) instead.
