# Config optimization

With many namespaced VectorPipelines the generated agent config contains one `kubernetes_logs` source per pipeline source. Inside vector every `kubernetes_logs` source runs its own kube-apiserver clients: three watch streams (Pods, Namespaces, Nodes) and its own pod metadata cache, plus a separate log file scanner. With N pipelines every agent pod keeps 3×N watch connections and N copies of the node's pod metadata. On large clusters this dominates kube-apiserver traffic and agent memory.

Sources optimization collapses such sources into one. It is a controller-level
feature flag (off by default):

```yaml
# helm values
args:
  - "-enable-config-optimization"
```

A particular Vector CR can be opted out (e.g. for a staged rollout or an agent
image too old to compile the generated routing):

```yaml
metadata:
  annotations:
    vector-operator.kaasops.io/config-optimization: disabled
```

The flag is expected to become the default behavior in a future release and
the gate to be removed eventually.

## What it does

Sources which differ **only in the watched namespace** (same type, label/field selectors and other options — the form generated for namespaced VectorPipelines) are grouped, and every group is replaced with:

- a single `kubernetes_logs` source watching the union of the namespaces (`kubernetes.io/metadata.name in (ns1,ns2,...)`);
- route transforms which split the stream back per namespace, so every pipeline receives exactly the events it received before. For large groups the routing is two-level (a remap computes an md5-based bucket of the namespace, a first-level route selects the bucket, a second-level route selects the namespace) to keep the number of conditions evaluated per event near `2*sqrt(N)` instead of `N`.

Inputs of pipeline transforms and sinks are rewired automatically. Sources with unique settings (for example a custom `extra_label_selector`) are left untouched. Groups larger than 1000 namespaces are split to keep the generated namespace selector short.

An event matching several pipelines is still delivered to all of them. The log file of such a pod is now read from disk once instead of once per pipeline.

## Effect

Measured on a 1000-pipeline single-node cluster (vector 0.48): watch requests to the kube-apiserver drop by three orders of magnitude (the periodic reconnect waves of thousands of watch streams disappear), agent memory drops ~20×, agent CPU and delivery throughput stay at parity under nominal load.

## Requirements and notes

- Namespace selection relies on the `kubernetes.io/metadata.name` label, which is set automatically on every namespace since Kubernetes 1.21.
- The optimized source names are derived from a hash of the group settings and do not depend on the namespace list: pipelines can be added and removed without renaming the source, so vector file checkpoints survive such changes.
- **Enabling (or disabling) the optimization on a running cluster renames the sources, so vector re-reads the log files currently retained on the nodes: expect a one-time redelivery of recent logs** — unless checkpoint migration is enabled, see below.
- Vector logs a warning about unconsumed `<router>._unmatched` outputs of the generated route transforms; it is harmless — events not matching any pipeline namespace are dropped there.

## Checkpoint migration

Vector keys file checkpoints by a fingerprint of the file content, not by the source name, so the positions saved under the old source names stay valid after the rename and can be carried over. `--enable-checkpoint-migration` does that:

```yaml
# helm values
args:
  - "-enable-config-optimization"
  - "-enable-checkpoint-migration"
```

- The agent config Secret name is bound to the optimization mode (`<name>-agent` / `<name>-agent-opt`) and both Secrets are kept up to date. Switching the mode (the flag or the per-CR annotation) changes the pod template and **rolls the DaemonSet** instead of a live config reload: pods not yet rolled keep their previous config, so every node migrates exactly at its own restart.
- A `checkpoint-merger` init container (image `kaasops/checkpoint-merger`, override with `--checkpoint-merger-image`) consolidates the checkpoints into the directories of the new source names before vector starts. The operation is idempotent, fail-open (a problem is logged and the agent starts anyway — worst case is the one-time redelivery that would have happened without migration) and only understands the stable `version: "1"` checkpoint format (unchanged in vector since v0.20).
- Rolling back to the legacy config restores the saved per-source positions; only files that appeared while the optimization was active are re-read.
- A mode switch is a rolling restart of the agents: on large clusters expect it to take a while, and the re-created watch connections to arrive gradually (which is what you want).
