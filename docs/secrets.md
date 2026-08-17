# Pipeline secrets

`spec.secret` lets a VectorPipeline or ClusterVectorPipeline reference Kubernetes Secret values from its sources/transforms/sinks without the values ever appearing in the generated Vector config, and without the pipeline's namespace needing access to any other namespace's Secrets.

## Why

The operator is often run as "log collection as a service": a central Vector CR (agent or aggregator) is shared across many teams, while each team owns its own VectorPipeline in its own namespace and its own sink credentials. Before this feature, putting a credential into a sink meant either hardcoding it into the pipeline spec or wiring it through `envFrom`/`env` on the shared Vector CR (see [Secure credential](secure-credential.md)), which means every credential is set by whoever owns the Vector CR, and every pipeline author can see the environment of every other pipeline sharing that CR. `spec.secret` lets each pipeline author keep credentials in a Secret in their own namespace and reference them directly, with no central coordination and no cross-tenant visibility.

## Quickstart

Create a Secret in the pipeline's own namespace:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: creds
  namespace: team-a
type: Opaque
stringData:
  username: es-user
  password: es-pass
```

Declare a named backend for it in `spec.secret` and reference its keys as `SECRET[alias.key]` inside any string option:

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorPipeline
metadata:
  name: es-pipeline
  namespace: team-a
spec:
  secret:
    es:
      type: kubernetes_secret
      name: creds
  sources:
    logs:
      type: kubernetes_logs
  sinks:
    out:
      type: elasticsearch
      inputs:
        - logs
      endpoints:
        - "http://elasticsearch.example.com:9200"
      auth:
        strategy: basic
        user: "SECRET[es.username]"
        password: "SECRET[es.password]"
```

The backend alias (`es` above) must match `^[A-Za-z0-9_]+$`, enforced by a CEL rule on the CRD. `SECRET[...]` references only work inside string option values.

At config-build time the operator resolves every `SECRET[alias.key]` reference, rewrites it to Vector's native secrets mechanism (a `directory` backend), and copies only the referenced keys into an operator-managed aggregated Secret named `<workload>-secret-assets` in the Vector/aggregator workload's own namespace, mounted read-only at `/etc/vector/secrets`. The generated Vector config that ends up in the config Secret never contains the resolved values, only the rewritten `SECRET[...]` reference.

For VectorPipeline the Secret is always read from the pipeline's own namespace; `namespace` is forbidden in `spec.secret` for VectorPipeline, which is what keeps one namespace from reading another's Secrets.

## ClusterVectorPipeline

ClusterVectorPipeline is cluster-scoped and has no "own namespace" to default to, so each backend in `spec.secret` must set `namespace` explicitly:

```yaml
spec:
  secret:
    es:
      type: kubernetes_secret
      name: creds
      namespace: observability
```

Everything else (alias syntax, `SECRET[alias.key]` references, aggregated Secret, mount path) works the same as for VectorPipeline.

## Rotation

Updating the source Secret's data is enough; no pipeline edit is required. The operator syncs the aggregated `<workload>-secret-assets` Secret, kubelet refreshes the mounted volume on the workload's pods (typically within about a minute), and Vector reloads automatically via `--watch-config` once the new file content lands. The generated Vector config itself does not change on rotation, only the mounted secret file does.

If a rotation removes a key that a running pipeline still references, the reload fails and Vector keeps running the previously loaded topology rather than dropping the affected sink.

## Validation and troubleshooting

The operator validates `spec.secret` references at config-build time, before `vector validate` runs — `vector validate` itself does not resolve secrets. A `SECRET[alias.key]` reference to an alias not declared in `spec.secret`, or to a Secret/key that does not exist, marks that one pipeline invalid; other pipelines sharing the same Vector/aggregator are unaffected. Check:

- `.status.configCheckResult` and `.status.reason` on the pipeline for the validation error.
- `.status.relatedSecretsHash` on the pipeline, which tracks the identity and version of every Secret its `SECRET[...]` references actually use and changes whenever a referenced Secret rotates (even though the pipeline spec itself did not change). The value is derived only from Secret metadata, never from secret data, so it reveals nothing about the contents.

A backend declared in `spec.secret` but never referenced by any `SECRET[...]` placeholder is ignored: it is not read, not watched for rotation, and its absence does not affect the pipeline.

Transient failures (the referenced Secret temporarily missing, or an API error while reading it) are retried automatically about every 10 seconds; when the operator's Secret watch covers the Secret (the default — see the `--watch-namespace`/`--watch-name` limitations below), creating or updating it also triggers a reconcile immediately. Either way, once the Secret is fixed the pipeline recovers on its own. Spec-level errors (a reference to an alias not declared in `spec.secret`, `namespace` set on a VectorPipeline backend, or `namespace` missing on a ClusterVectorPipeline backend) are not retried: fix the pipeline spec, and the edit itself triggers the reconcile.

## Requirements

Vector `>= v0.43.0` is required for the `directory` secrets backend that this feature relies on. The operator's default images already satisfy this.

The feature is also compatible with Vector 0.57+, which disables `${VAR}` environment-variable interpolation by default: `SECRET[...]` is Vector's separate secrets mechanism and keeps working without any opt-in flag, and the operator only ever writes references into string values, in line with 0.57's deprecation of placeholders in non-string positions.

## Limitation: `--watch-namespace`

When the operator runs with `--watch-namespace`, automatic rotation detection (the mechanism behind the "typically within about a minute" reload above) only covers Secrets in the watched namespace(s). A Secret referenced from outside the watched namespaces (possible for a ClusterVectorPipeline backend) is still read correctly at reconcile time, but rotating it does not by itself trigger a reconcile: the watch cannot report that change, so it is the periodic re-check described below that picks it up.

When the operator runs with `--watch-name`, the Secret cache is additionally label-filtered to `app.kubernetes.io/managed-by=vector-operator` AND `app.kubernetes.io/name=<the flag's value>`, so automatic rotation detection does not fire for an ordinary user pipeline Secret, which does not normally carry the operator's own `managed-by` label. Values are still read correctly on every reconcile, so a reconcile triggered by any other cause (or an operator restart) picks up rotated values — and when nothing else triggers one, the periodic re-check described below does.

To keep rotation working in both cases, an operator started with either flag re-checks each pipeline that actually references a Secret every 4-5 minutes, so a rotation the watch cannot report is picked up on the next re-check instead of waiting for an unrelated reconcile. The exact interval is derived from the pipeline's own namespace and name, which spreads the re-checks across the window and keeps each pipeline's schedule stable across operator restarts — a fixed interval would have a large cluster re-checking every pipeline in the same second. A re-check that finds nothing changed ends without writing anything and without running a config check; only an actual change reaches the workload. This costs one read per referenced Secret per interval and is not enabled in default mode, where the watch already reports rotations on its own.

A rotation can also be pulled in immediately, without waiting for the next re-check: set or change an annotation of your own on the pipeline, for example `kubectl annotate --overwrite vectorpipeline/app rotated-at="$(date -Is)"`. The annotation's name and value carry no meaning to the operator — an annotation change is simply one of the two edits that wake a pipeline reconcile, and that reconcile re-reads every referenced Secret straight from the API server, independently of what the operator's cache holds or is scoped to. If the values did rotate, the workload is rebuilt with the new ones; if nothing actually changed, the reconcile stops early and touches nothing, so the annotation is safe to set as often as you like.

## Limitation: what a value may contain

A `SECRET[alias.key]` reference is replaced by the key's **content**, not by a path to a file holding it, and Vector substitutes that content into the config text before parsing it. The operator publishes that config as JSON, so a value has to be representable inside a JSON string. Values that are not are rejected up front, before ever reaching a config:

- it must be valid UTF-8 — Vector's `directory` secrets backend reads the mounted file expecting UTF-8;
- no double quote (`"`) — it closes the surrounding JSON string early, corrupting every field after it;
- no backslash (`\`) — depending on the character next to it, it either breaks the parse or forms a *valid* escape sequence that decodes to different bytes than the Secret holds, which both Vector and `vector validate` accept silently;
- no raw control byte (0x00–0x1F).

An empty value is left alone, and anything else valid UTF-8 can express — ordinary text, `/`, non-ASCII Unicode — passes through untouched. A value that fails the check marks the pipeline invalid like any other validation error: `.status.configCheckResult` becomes `false` and `.status.reason` names the Secret and key, never the value or any part of it. Recovery is automatic once the Secret is rotated to a value that satisfies the contract.

This makes the feature a fit for single-line credentials — passwords, tokens, usernames, connection strings — and not for certificate or private-key material, which is multi-line by nature. The obstacle is the transport rather than the sink options themselves: Vector's `ca_file`/`crt_file`/`key_file` do accept inline PEM in place of a path, and for `kafka` it passes such material to librdkafka through its `ssl.ca.pem`/`ssl.certificate.pem`/`ssl.key.pem` properties instead of the `.location` ones, which take a path only. So for a `kafka` sink using TLS, keep the certificate and key as ordinary files — mounted through `volumes`/`volumeMounts` on the Vector CR — and use `spec.secret` for the single-line credentials next to them:

```yaml
sinks:
  out:
    type: kafka
    inputs: [logs]
    bootstrap_servers: "kafka.example.com:9093"
    auth:
      tls:
        enabled: true
        # mounted on the Vector CR, not carried through spec.secret
        ca_file: /etc/vector/tls/ca.crt
      sasl:
        enabled: true
        mechanism: SCRAM-SHA-512
        username: "SECRET[kafka.username]"
        password: "SECRET[kafka.password]"
```

## Limitation: flat-key collisions across pipelines

Each pipeline's secret references are sanitized into a flat key shared by every pipeline on the same Vector/aggregator, and that sanitization turns both `-` and the namespace/name separator into `_`, so two different pipelines can produce the identical flat key: namespace `team` with pipeline `a-x` collides with namespace `team-a` with pipeline `x`. The operator attributes the collision to the younger of the two (the later `metadata.creationTimestamp`) and fails only that one — its `configCheckResult` becomes `false` with a `.status.reason` naming the colliding flat key and the surviving pipeline, while the older pipeline and the workload keep running and no secret value can leak between them. Attribution is computed oldest first across the whole workload, so a pipeline that is itself excluded never goes on to exclude anyone else. Recovery is automatic once either pipeline is renamed, deleted, or has its alias or key changed so the flat keys no longer collide.

Two effects to expect while a collision persists. The pipeline's status is global, so a pipeline selected by several workloads shows as invalid even though the collision exists on only one of them — the `.status.reason` names which workload discovered it. And an unrelated reconcile of the younger pipeline (an operator restart, a rotation, an annotation change) can briefly flip its status back to valid before the next workload reconcile re-marks it: a bounded flicker, not a stuck state.

## Limitation: aggregated Secret size

Every pipeline on the same Vector/aggregator shares one aggregated `<workload>-secret-assets` Secret, and Kubernetes limits a Secret to 1 MiB (1,048,576 bytes), counting the resolved values only. A pipeline whose own values fit comfortably can still be excluded if the other pipelines on that workload already fill most of the budget.

Before building the merged config the operator sums every pipeline's referenced values and walks the pipelines oldest first, keeping each one that still fits and skipping any that would push the total over — the same oldest-first policy as for collisions above. Skipping one oversized pipeline does not disqualify the rest: a smaller, younger pipeline is still evaluated afterwards and kept if it fits on its own. A value referenced more than once within the same pipeline is counted once; two different pipelines referencing the same underlying Secret key each pay for their own copy.

An excluded pipeline's `configCheckResult` becomes `false`, with a `.status.reason` naming the byte limit, the bytes already committed by older pipelines, and its own would-be contribution; the workload and the other pipelines are unaffected — this is what keeps one tenant's secret data from freezing a shared workload. Once room frees up, the excluded pipeline returns on its own.

## Limitation: aggregated Secret object size

The 1 MiB limit counts values only, and it is not the only ceiling the aggregated Secret has to stay under: the write also has to fit through etcd's request limit, which counts the **whole object**, every generated key name included. Since the operator generates one flat key per (namespace, pipeline, alias, key), a workload with many pipelines can accumulate enough key-name mass to hit that limit while its values stay far below 1 MiB. The two rejections read very differently, which matters when diagnosing one: values over the limit give `data: Too long: may not be more than 1048576 bytes`, naming the field and the number, while an object too large for etcd gives `Error from server: etcdserver: request is too large`, naming neither.

To keep the second one from ever reaching you, the operator models the size of the whole assets Secret (key names, values, and serialization overhead) and holds it under a conservative internal ceiling. Attribution, status, and recovery are exactly as for the values limit above, with one difference: the `.status.reason` names the object-size budget explicitly, so it is clear that the constraint is the combined size of key names, values, and metadata rather than the 1 MiB values limit. A workload approaching this ceiling is usually better served by splitting its pipelines across more than one Vector/aggregator. Note that the ceiling is a safety budget, not a guarantee: an etcd running with a lowered `--max-request-bytes` can still reject a write the operator considered safe.

## Limitation: transitions that would themselves overflow

The config Secret and the assets Secret are two independent Kubernetes objects that cannot be updated atomically, and the operator has no signal confirming that a pod has actually rolled onto a new config. So it never lets the config Secret reference a key the assets Secret does not have yet: the assets Secret is staged first with the union of the keys the outgoing and the incoming config need, and keys that are no longer referenced are pruned only on a later reconcile — one that confirms the config has not changed since and that a grace period (currently 90 seconds) has passed since it was published. The wait exists because kubelet projects a changed Secret into running pods on its own schedule: 46-60 seconds on a `kind` test cluster, with no bound the operator can promise for other clusters.

That union can, in a narrow case, itself exceed the 1 MiB limit even though neither the outgoing nor the incoming state does — swapping one large pipeline for another on a workload already near the ceiling. The operator neither writes over the limit nor drops an existing key early; instead it publishes a narrower **bridge** config with whatever fits alongside what is already staged, and the rest wait with a `.status.reason` saying so (distinct from a size-limit exclusion: a waiting pipeline is still the intended member of the target set, just delayed). Waiting does mean excluded from the published config, so a pipeline that was already collecting logs stops for as long as it waits. Such a transition takes a few extra reconciles and at least the grace period to converge, but it always converges on its own, with no manual step and no window where a config references a key the assets Secret does not have — the operator schedules its own re-check for the waiting round, so the workload can simply look idle for up to that long.

## Using Vault or another external secret store

`spec.secret` only reads from a native Kubernetes Secret, so an external store has to be synced into one first. Point [External Secrets Operator](https://external-secrets.io/) or [Vault Secrets Operator](https://developer.hashicorp.com/vault/docs/platform/k8s/vso) at Vault (or another supported backend) to materialize a Kubernetes Secret in the pipeline's own namespace, then reference it via `spec.secret` exactly as in the quickstart above. See the respective project's documentation for setting up the sync.

## See also

[Secure credential](secure-credential.md) documents `envFrom`/`env` on the Vector CR, which injects credentials into every pipeline sharing that CR and is set by whoever owns the CR. Use `spec.secret` instead when each pipeline needs its own credentials, scoped to its own namespace, without the Vector CR owner in the loop.
