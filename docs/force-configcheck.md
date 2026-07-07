# Force ConfigCheck via Annotation

## Problem

The pipeline controller uses hash-based change detection that includes `Spec`, `Labels`, and `ServiceName` annotation.
When external dependencies change (Secrets, ConfigMaps, Aggregator endpoints), the hash remains the same and configcheck does not run.

## Solution

Annotate your VectorPipeline or ClusterVectorPipeline with `vector-operator.kaasops.io/force-configcheck` to trigger configcheck.

The annotation value is included in the pipeline hash. When it changes, the hash changes, and configcheck runs.

## Usage

```bash
# Trigger configcheck
kubectl annotate vp my-pipeline \
    vector-operator.kaasops.io/force-configcheck="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Re-trigger (change the value)
kubectl annotate vp my-pipeline \
    vector-operator.kaasops.io/force-configcheck="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --overwrite

# ClusterVectorPipeline
kubectl annotate cvp my-cluster-pipeline \
    vector-operator.kaasops.io/force-configcheck="v2" --overwrite

# Batch: all pipelines with a specific label
kubectl get vp -l app=myapp -o name | \
    xargs -P10 -I{} kubectl annotate {} \
    vector-operator.kaasops.io/force-configcheck="$(date +%s)" --overwrite

# Batch: all pipelines in a namespace
kubectl get vp -n production -o name | \
    xargs -P10 -I{} kubectl annotate -n production {} \
    vector-operator.kaasops.io/force-configcheck="$(date +%s)" --overwrite

# Batch: only invalid pipelines
kubectl get vp -A -o json | \
    jq -r '.items[] | select(.status.configCheckResult == false) | "-n \(.metadata.namespace) vp \(.metadata.name)"' | \
    xargs -P10 -l kubectl annotate \
    vector-operator.kaasops.io/force-configcheck="$(date +%s)" --overwrite
```

## How it works

1. User sets annotation with any value (timestamp, version, UUID)
2. The annotation value is included in the pipeline hash calculation
3. Changed value = changed hash = configcheck runs
4. After configcheck, the new hash is saved in `status.LastAppliedPipelineHash`
5. Same annotation value on next reconcile = same hash = no configcheck

## Notes

- The annotation value can be any non-empty string
- Setting the annotation to the same value has no effect (hash unchanged)
- Removing the annotation also changes the hash and triggers configcheck
- Pipelines without the annotation are unaffected (backward compatible)
- The controller never modifies the annotation — GitOps compatible
