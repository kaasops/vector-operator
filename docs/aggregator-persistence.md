The `persistence` feature gives the Vector aggregator durable storage for its `data_dir`, backed by a persistent volume per replica, so that disk buffered events survive a pod restart or reschedule. When it is enabled the operator renders the aggregator as a StatefulSet instead of the default Deployment.

Persistence provides the durable storage only. It does not turn on disk buffering by itself, that has to be configured per sink. See [Enabling disk buffering](#enabling-disk-buffering) below.

> ⚠️ **Availability Note:** This section is exclusively available for the `VectorAggregator` and `ClusterVectorAggregator` Custom Resource Definitions (CRDs). It is not applicable to agent roles.

## Enabling disk buffering

Persistence gives the aggregator a durable `data_dir`, but it does not enable disk buffering. The operator renders sinks exactly as you write them and never sets a `buffer`, so what gets buffered, and whether it survives a restart, is entirely down to your sink configuration.

For data to survive a restart, the sink must use a disk buffer. Set `buffer.type = disk` on the sinks whose data you need to keep:

```yaml
sinks:
  my_sink:
    type: ...
    buffer:
      type: disk
      max_size: 1073741824 # bytes, roughly 1Gi. Vector requires at least 256Mi.
```

Without a disk buffer a sink buffers in memory, so enabling `persistence` leaves the volume unused and any in flight events are still lost when the pod restarts. Only events held in a disk buffer are written to the `data_dir` and recovered when the pod comes back.

## Why a StatefulSet

Vector's disk buffer lives under the global `data_dir`, and its whole purpose is surviving restarts. That requires the data directory to be persistent and stable per replica, so a restarted replica reclaims its own buffer rather than a foreign or empty one. A Deployment backed by a node local hostPath cannot provide that, so the operator uses a StatefulSet with `volumeClaimTemplates`, which gives each replica a stable volume that reattaches to the same ordinal across restarts. This mirrors the Aggregator role in the official Vector Helm chart.

When `persistence` is not enabled the aggregator stays a Deployment and nothing changes, so existing aggregators are unaffected.

## Hierarchy

```yaml
spec:
  persistence:
    enabled: <boolean>
    size: <quantity>
    storageClassName: <string>
    accessModes: <array>
    retentionPolicy: <object>
    volumeClaimTemplates: <array>
```

## Configuration Example

```yaml
apiVersion: observability.kaasops.io/v1alpha1
kind: VectorAggregator
metadata:
  name: vector-aggregator
  namespace: logging
spec:
  persistence:
    enabled: true
    size: 10Gi
    storageClassName: gp3
```

## Fields

| Field | Default | Description |
| --- | --- | --- |
| `enabled` | `false` | Switches the workload to a StatefulSet with a persistent volume per replica. Does not enable disk buffering on its own, see Enabling disk buffering above. |
| `size` | `10Gi` | Requested size of the volume for each replica. |
| `storageClassName` | cluster default | StorageClass for the volume. |
| `accessModes` | `["ReadWriteOnce"]` | Access modes for the volume. ReadWriteOnce is required, since a Vector disk buffer must have exactly one writer. |
| `retentionPolicy` | `Retain` / `Retain` | Whether the volumes are kept or deleted when replicas are scaled down or the StatefulSet is deleted. Defaults to retaining them so buffered data can be replayed. |
| `volumeClaimTemplates` | none | Escape hatch for full control over the persistent volume claims. When set it takes precedence over the convenience fields above. |

When persistence is enabled the operator also creates a headless governing service named `<name>-aggregator-headless`, which the StatefulSet uses for stable per replica DNS.

## Deployment versus StatefulSet

The operator selects the workload from the spec:

- `persistence.enabled: true`, or a non empty `persistence.volumeClaimTemplates`, renders a StatefulSet.
- Anything else renders the existing Deployment.

Turning persistence on for an existing aggregator is a recreate operation, because Kubernetes cannot convert a Deployment to a StatefulSet in place. Plan for a short interruption when you enable it on an aggregator that already exists.

## Sizing

Vector force exits when it cannot write to a disk buffer, for example when the volume is full, and recovers the on disk buffer on restart. Size the volume for the sum of the maximum sizes of all disk buffers configured on the aggregator's sinks, with headroom, and monitor free space on the volume.

## Resizing volumes

The operator does not resize persistent volumes. The `size` and `storageClassName` of a StatefulSet's `volumeClaimTemplates` are immutable, so once the StatefulSet exists Kubernetes rejects changes to them. If you edit `persistence.size` on an existing aggregator the operator keeps the live value and leaves the StatefulSet unchanged, since applying the change would require recreating the aggregator. It records a log entry noting that the change was not applied, so watch the operator logs when you change these fields.

If you need a larger volume you have two options, both manual:

- **Grow in place.** If the StorageClass has `allowVolumeExpansion: true`, patch each existing PVC's `spec.resources.requests.storage` directly. The volume claim template only governs volumes created for new replicas, so growing the existing PVCs does not conflict with the immutable template. With a CSI driver that supports online expansion the filesystem grows without a restart, otherwise the pod must restart to pick up the larger filesystem.
- **Recreate.** Delete and recreate the aggregator with the new size. Because the default retention policy is `Retain`, the old PVCs are not removed automatically, so reclaim or migrate them deliberately.

Shrinking a volume and changing the StorageClass are not supported by Kubernetes for an existing StatefulSet.

Automating expansion in the operator is deliberately out of scope for now. It adds reconcile complexity and partial failure states for a feature whose whole point is durability, so resizing is kept manual and explicit.
