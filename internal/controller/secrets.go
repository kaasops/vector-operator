/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

// pipelineSecretGetter builds a config.VectorConfigParams.PipelineSecretGetter that
// reads through an uncached client.Reader (the manager's API reader) for freshness and
// independence from cache scoping (namespace/label filters) - not to keep secret
// payloads out of the controller-runtime cache, which they already enter via the
// workload reconcilers' Owns(&corev1.Secret{}) in default mode.
//
// It closes over the reconcile's own ctx rather than relying on the ctx parameter the
// returned func receives: Build*Config always invokes the getter with
// context.Background() internally, so the caller's ctx (with its real
// deadline/cancellation) has to be captured here instead.
//
// A nil reader (unit tests that construct a reconciler without APIReader) yields a nil
// getter, matching the existing "secrets unsupported in this context" behavior.
//
// Called once per reconcile in each of the three workload controllers, so the in-memory
// cache below is scoped to exactly one reconcile: long enough to matter, since
// DetectSecretCollisions, DetectSecretSizeOverflow, BridgeAssets and resolvePendingSecrets
// can each ask for the same Secret in one round and would otherwise each pay for an
// uncached read, and short enough that staleness within the round is not a concern.
func pipelineSecretGetter(reader client.Reader, ctx context.Context) func(context.Context, string, string) (*corev1.Secret, error) {
	if reader == nil {
		return nil
	}
	type cacheEntry struct {
		secret *corev1.Secret
		err    error
	}
	cache := make(map[string]cacheEntry)
	return func(_ context.Context, namespace, name string) (*corev1.Secret, error) {
		key := namespace + "/" + name
		if entry, ok := cache[key]; ok {
			return entry.secret, entry.err
		}
		secret := &corev1.Secret{}
		err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret)
		if err != nil {
			cache[key] = cacheEntry{err: err}
			return nil, err
		}
		cache[key] = cacheEntry{secret: secret}
		return secret, nil
	}
}

// invalidSecretShapeError marks a secret-backend namespace shape violation (namespace
// set on a VectorPipeline backend, or missing on a ClusterVectorPipeline backend). This
// is a permanent spec error - the only fix is editing the pipeline's spec, which the
// pipeline controller's own generation-change watch already retriggers - as opposed to
// a transient resolve failure such as "secret not found yet", which is worth retrying
// on a timer. The pipeline controller checks for it via errors.As to skip the
// RequeueAfter it otherwise applies to resolve failures.
type invalidSecretShapeError struct {
	msg string
}

func (e *invalidSecretShapeError) Error() string { return e.msg }

// resolveRelatedSecrets resolves every secret backend declared on a pipeline's
// spec.secret through an uncached read (the same rationale as pipelineSecretGetter:
// freshness and independence from cache scoping, not keeping payloads out of the
// cache), returning both the secret identities (for SecretIndex, which only needs to
// know which pipeline wants which secret) and the change token for
// RelatedSecretsHash.
//
// Resolution is scoped to the backends a SECRET[] reference actually uses (the used
// set comes from config.UsedSecretBackends): a declared-but-unused backend is never
// read, hashed, or watched, so it cannot fail the pipeline, widen API load, or leak
// existence information through the status.
//
// Shape validation still covers every declared backend (used or not) and runs before any
// Get: otherwise a CVP backend missing its required namespace falls into a namespaced
// Get-by-name with an empty namespace, which client-go rejects with its own confusing
// error instead of the designed "namespace is required" one (processPipelineSecrets
// enforces the identical rule at config-build time). Sorted aliases keep which violation
// is reported deterministic.
//
// refs is fully populated before any Get is attempted, so the index stays accurate even
// when a later Get fails - the watch can then find this pipeline again once the missing
// secret shows up. A shape violation (invalidSecretShapeError) is exempt: it is a
// permanent spec error no watch can resolve, so refs/token are left nil.
func resolveRelatedSecrets(ctx context.Context, reader client.Reader, p pipeline.Pipeline, used map[string]struct{}) ([]types.NamespacedName, *int64, error) {
	declared := p.GetSpec().Secret
	if len(declared) == 0 {
		return nil, nil, nil
	}
	if reader == nil {
		return nil, nil, fmt.Errorf("pipeline %s: secrets are not supported in this context", p.GetName())
	}

	_, isVP := p.(*v1alpha1.VectorPipeline)

	declaredAliases := make([]string, 0, len(declared))
	for alias := range declared {
		declaredAliases = append(declaredAliases, alias)
	}
	sort.Strings(declaredAliases)

	for _, alias := range declaredAliases {
		backend := declared[alias]
		if isVP && backend.Namespace != "" {
			return nil, nil, &invalidSecretShapeError{msg: fmt.Sprintf("secret backend %q: namespace is not allowed in VectorPipeline", alias)}
		}
		if !isVP && backend.Namespace == "" {
			return nil, nil, &invalidSecretShapeError{msg: fmt.Sprintf("secret backend %q: namespace is required", alias)}
		}
	}

	aliases := make([]string, 0, len(used))
	for _, alias := range declaredAliases {
		if _, ok := used[alias]; ok {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) == 0 {
		return nil, nil, nil
	}

	// Two aliases may point at the same Secret; refs and the token deduplicate by
	// identity so the index and the change token depend only on which Secrets are
	// referenced, not on how many aliases reference them.
	seen := make(map[types.NamespacedName]string, len(aliases))
	refs := make([]types.NamespacedName, 0, len(aliases))
	for _, alias := range aliases {
		backend := declared[alias]
		ns := p.GetNamespace()
		if !isVP {
			ns = backend.Namespace
		}
		ref := types.NamespacedName{Namespace: ns, Name: backend.Name}
		if _, dup := seen[ref]; !dup {
			seen[ref] = alias
			refs = append(refs, ref)
		}
	}

	identities := make([]corev1.ObjectReference, 0, len(refs))
	for _, ref := range refs {
		secret := &corev1.Secret{}
		if err := reader.Get(ctx, client.ObjectKey(ref), secret); err != nil {
			return refs, nil, fmt.Errorf("secret backend %q: failed to get secret %s: %w", seen[ref], ref, err)
		}
		identities = append(identities, corev1.ObjectReference{
			Namespace:       secret.Namespace,
			Name:            secret.Name,
			UID:             secret.UID,
			ResourceVersion: secret.ResourceVersion,
		})
	}
	return refs, relatedSecretsToken(identities), nil
}

// relatedSecretsToken computes the RelatedSecretsHash status value from the
// identities (namespace, name, UID, resourceVersion) of the Secrets a pipeline's
// SECRET[] references actually use - never from secret data. Two properties follow:
//
//   - No fingerprint oracle: the published status value carries no function of the
//     secret bytes, so a user who can read pipeline statuses but not Secrets cannot
//     brute-force low-entropy values offline or probe for a Secret's existence.
//   - No framing collisions: every field is length-prefixed before hashing, so no
//     combination of crafted names/values can serialize two different states to the
//     same input (the old data-based form concatenated "key=value\n" unescaped, and a
//     value containing "\nk=v" framed identically to two separate entries, silently
//     swallowing a real rotation).
//
// Any data change bumps the Secret's resourceVersion, so rotation detection can only
// over-trigger (e.g. on annotation-only updates), never under-trigger. The identities
// slice must already be deterministically ordered (resolveRelatedSecrets builds it
// from sorted refs). The sha256 sum is folded to the status's *int64 idiom (see
// VectorPipelineStatus.LastAppliedPipelineHash); returns nil for no identities so a
// pipeline without used secret references gets an absent (omitempty) field.
func relatedSecretsToken(identities []corev1.ObjectReference) *int64 {
	if len(identities) == 0 {
		return nil
	}

	buf := make([]byte, 0, 128)
	appendField := func(s string) {
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(len(s)))
		buf = append(buf, l[:]...)
		buf = append(buf, s...)
	}
	for _, id := range identities {
		appendField(id.Namespace)
		appendField(id.Name)
		appendField(string(id.UID))
		appendField(id.ResourceVersion)
	}

	sum := sha256.Sum256(buf)
	h := int64(binary.BigEndian.Uint64(sum[:8]))
	return &h
}

// relatedSecretsHashEqual compares two possibly-nil hash pointers by value.
func relatedSecretsHashEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// secretCollisionReasonPrefix marks a pipeline status Reason as coming from
// resolveWorkloadPipelines' collision attribution, distinguishing it from any other
// reason a pipeline can be invalid for. resolveWorkloadPipelines looks for this prefix
// to find pipelines worth reconsidering once whatever they collided with is gone or
// renamed - a plain "was this pipeline invalid last time" check cannot tell a
// collision failure (which resolves itself once the OTHER pipeline changes, with
// nothing touching this pipeline's own spec) apart from any other, permanent failure.
//
// Its exact wording is a frozen, persistent contract - see
// secretAssetsWaitingReasonPrefix's doc comment below for why.
const secretCollisionReasonPrefix = "secret flat key collision: "

// secretSizeExclusionReasonPrefix is secretCollisionReasonPrefix's counterpart for
// resolveWorkloadPipelines' size-budget attribution (config.DetectSecretSizeOverflow):
// same purpose, same reconsideration mechanism, different limit.
const secretSizeExclusionReasonPrefix = "secret assets size limit: "

// secretObjectSizeExclusionReasonPrefix is the third attribution class, alongside
// collisions and the values-size limit: this pipeline's values fit under
// corev1.MaxSecretSize, but adding its entries would push the assets Secret past
// config.SecretAssetsObjectBudget - the modelled size of the whole object, key names
// included. A separate prefix rather than a variant of the size wording, because the
// two are different failures with different remedies (see config.SecretAssetsObjectBudget)
// and a user must not be told to shrink values that were never the problem.
//
// Frozen on the same terms as the other two - see secretAssetsWaitingReasonPrefix.
const secretObjectSizeExclusionReasonPrefix = "secret assets object size limit: "

// secretAssetsWaitingReasonPrefix marks a pipeline temporarily held back by
// planSecretAssetsBridge: it IS part of the final target set (unlike
// secretSizeExclusionReasonPrefix, which marks a loser), but its values cannot be staged
// this round because what the assets Secret currently holds - unprunable while a
// not-yet-rolled pod may still need it - leaves no room. A scheduling delay, not a
// verdict: a later reconcile's deferred prune frees the room on its own.
//
// The exact wording of this prefix (and secretCollisionReasonPrefix/
// secretSizeExclusionReasonPrefix above) is a persistent contract, not just a log
// message: isSecretAttributionReason parses it back out of a pipeline's stored
// .status.reason on every future reconcile to decide whether that pipeline is worth
// reconsidering. Changing the text in a future version without a migration would
// strand every pipeline already marked with the OLD text outside the retry pool
// forever - freeze it, or add explicit backward-compatible matching alongside any
// wording change.
const secretAssetsWaitingReasonPrefix = "secret assets waiting for room: "

// isSecretAttributionReason reports whether reason was written by
// resolveWorkloadPipelines/planSecretAssetsBridge itself (collision attribution,
// size-budget attribution, or a bridge-round wait) - as opposed to any other,
// permanent reason a pipeline can be invalid for. All three are self-correcting once
// something else (another pipeline, or the assets Secret's own contents) changes,
// with nothing touching this pipeline's own spec, so all three are worth
// reconsidering on a later reconcile; see resolveWorkloadPipelines' doc comment.
func isSecretAttributionReason(reason string) bool {
	return strings.HasPrefix(reason, secretCollisionReasonPrefix) ||
		strings.HasPrefix(reason, secretSizeExclusionReasonPrefix) ||
		strings.HasPrefix(reason, secretObjectSizeExclusionReasonPrefix) ||
		strings.HasPrefix(reason, secretAssetsWaitingReasonPrefix)
}

// secretCollisionReason renders a config.SecretCollision into the pipeline status
// Reason: the flat key, the surviving pipeline it collided with, and the workload
// whose merged build discovered it - naming the workload because the same pipeline
// can be selected by several workloads and this failure is only true for this one
// (see resolveWorkloadPipelines' doc comment on the multi-workload nuance).
func secretCollisionReason(c config.SecretCollision, workloadKind, workloadNamespace, workloadName string) string {
	workloadRef := workloadKind + " " + workloadName
	if workloadNamespace != "" {
		workloadRef = fmt.Sprintf("%s %s/%s", workloadKind, workloadNamespace, workloadName)
	}
	return fmt.Sprintf(
		"%sflat key %q collides with pipeline %s on %s; rename one of them (or its secret backend alias/key) to change its flat key",
		secretCollisionReasonPrefix, c.FlatKey, c.Survivor, workloadRef,
	)
}

// secretSizeExclusionReason renders a config.SecretSizeExclusion into the pipeline
// status Reason: the limit, how many bytes were already committed to older pipelines
// sharing the same aggregated Secret, how many this pipeline's own values would have
// added, and the workload whose merged build discovered the overflow - the same
// naming rationale as secretCollisionReason.
func secretSizeExclusionReason(e config.SecretSizeExclusion, workloadKind, workloadNamespace, workloadName string) string {
	workloadRef := workloadKind + " " + workloadName
	if workloadNamespace != "" {
		workloadRef = fmt.Sprintf("%s %s/%s", workloadKind, workloadNamespace, workloadName)
	}
	if e.ObjectBudget {
		// Deliberately says nothing about the 1 MiB values limit (and never quotes
		// its number - see TestSecretSizeExclusionReasonNamesTheObjectBudget):
		// this pipeline's values fit it. What did not fit is the whole object - key
		// names, values, and serialization overhead together (one flat key per
		// namespace/pipeline/alias/key, each with its own value) - so the wording
		// names key names AND values, not values alone.
		return fmt.Sprintf(
			"%sadding this pipeline's %d bytes of secret entries (key names and values) to the %d bytes already committed by older pipelines would exceed the "+
				"%d byte budget the operator keeps for the whole secret-assets Secret object; the Secret is shared by every pipeline %s selects, so older "+
				"pipelines (by creation time) keep it and this one is excluded until the total fits - this limit accounts for the combined size of key names "+
				"and values together, not values alone",
			secretObjectSizeExclusionReasonPrefix, e.PipelineObjectBytes, e.AcceptedObjectBytes, config.SecretAssetsObjectBudget, workloadRef,
		)
	}
	return fmt.Sprintf(
		"%sadding this pipeline's secret values (%d bytes) to the %d bytes already committed by older pipelines would exceed the %d byte Kubernetes Secret limit; "+
			"the secret-assets Secret is shared by every pipeline %s selects, so older pipelines (by creation time) keep it and this one is excluded until the total fits",
		secretSizeExclusionReasonPrefix, e.PipelineBytes, e.AcceptedTotal, corev1.MaxSecretSize, workloadRef,
	)
}

// secretAssetsWaitingReason renders the workload whose bridge round could not stage a
// pipeline this round into the pipeline status Reason - see
// secretAssetsWaitingReasonPrefix's doc comment for what this state means and why it
// is not a failure.
func secretAssetsWaitingReason(workloadKind, workloadNamespace, workloadName string) string {
	workloadRef := workloadKind + " " + workloadName
	if workloadNamespace != "" {
		workloadRef = fmt.Sprintf("%s %s/%s", workloadKind, workloadNamespace, workloadName)
	}
	return fmt.Sprintf(
		"%sthis pipeline is part of the correct target set for %s, but its secret values cannot be safely staged into the shared assets Secret "+
			"until data still possibly in use by an older, not-yet-rolled config is freed on a later reconcile; no action is needed, it will be published automatically",
		secretAssetsWaitingReasonPrefix, workloadRef,
	)
}

// SecretAssetsPruneGracePeriod bounds how long the operator waits, after actually
// publishing a config that stopped referencing some secret-assets keys, before it
// will prune those keys from the assets Secret - see secretAssetsPruneDecision and
// ensureVectorAgentSecretAssets/ensureVectorAggregatorSecretAssets' doc comments for
// why pruning any earlier can crash-loop a pod that restarts before kubelet has
// projected the new config and assets Secrets.
//
// A heuristic, not a guarantee: it has to outlast this cluster's kubelet Secret sync,
// which the operator does not control. On a `kind` test cluster the window closed within
// 46-60 seconds; this value has margin above that single measurement, not a proven upper
// bound elsewhere. Deliberately not a CRD field - it is a property of the cluster, not of
// a workload, so a controller flag would be the place for it.
const SecretAssetsPruneGracePeriod = 90 * time.Second

// secretAssetsPruneDecision decides, for this reconcile, whether it is safe to prune
// the assets Secret down to the exact target the (confirmed unchanged) published
// config references, and if not, how much longer the caller should wait before
// checking again.
//
// publishedAt is when the config currently live on the cluster was last actually written
// (SetSuccessStatus's configPublished parameter); nil means no such record yet - a first
// publish, or an upgrade from a version that did not track it. Absent counts as "not safe
// yet", never as "nothing to protect": the caller seeds the mark this round and waits out
// a full grace period before pruning against it.
//
// requeueAfter is computed from publishedAt rather than from "now". Re-deriving a fresh
// grace period on every call would let a workload reconciled more often than the interval
// push its own deadline forward forever; anchoring to the fixed mark makes it converge.
func secretAssetsPruneDecision(configUnchanged bool, publishedAt *metav1.Time) (prune bool, requeueAfter time.Duration) {
	if !configUnchanged {
		return false, 0
	}
	if publishedAt == nil {
		return false, SecretAssetsPruneGracePeriod
	}
	if elapsed := time.Since(publishedAt.Time); elapsed < SecretAssetsPruneGracePeriod {
		return false, SecretAssetsPruneGracePeriod - elapsed
	}
	return true, 0
}

// assetsWouldDropAKey reports whether replacing existing outright with target would
// remove a key existing currently has - the one thing the grace period exists to
// prevent (see SecretAssetsPruneGracePeriod). A key changing VALUE (rotation) is not
// a drop and never needs to wait: only a key's disappearance can make a live,
// not-yet-rolled pod's config reference something the mounted assets Secret no
// longer has.
//
// This is what lets a caller skip secretAssetsPruneDecision's grace/requeue dance
// entirely when target is a superset of existing (adding a pipeline, or a workload
// that has never used spec.secret at all, where both maps are empty): publishing
// target immediately in that case can never crash-loop anything, so it must not cost
// every such reconcile a scheduled wakeup SecretAssetsPruneGracePeriod later - an
// additive feature has to stay a no-op for workloads that never trigger an actual
// removal, and "we might remove something eventually" is not the same condition as
// "we are removing something now".
func assetsWouldDropAKey(existing, target map[string][]byte) bool {
	for k := range existing {
		if _, ok := target[k]; !ok {
			return true
		}
	}
	return false
}

// planSecretAssetsBridge splits finalPipelines (resolveWorkloadPipelines' output -
// the correct, deterministic target set, computed independently of any assets
// Secret's current contents) into bridgePipelines (safe to actually publish into the
// config and every assets Secret variant THIS round) and waitingPipelines (still part
// of the final target, but held back this round - see secretAssetsWaitingReasonPrefix).
//
// existingVariants holds the CURRENT Data of every assets Secret variant a live pod might
// still have mounted - one entry for a plain workload, two for the agent under checkpoint
// migration. Each variant is an INDEPENDENT object with its OWN corev1.MaxSecretSize
// budget, so the bridge is computed per variant against that variant's own content,
// never against a merge of them: merging two variants each legitimately holding 600 KiB
// would fabricate a 1.2 MiB baseline no real object has to fit into, and BridgeAssets
// would then hand back data no single write could accept. A pipeline reaches
// bridgePipelines only if it fits in EVERY variant, because checkpoint migration's
// dual-sync invariant requires both variants' configs to reference the same pipeline set.
//
// bridgeDataPerVariant is the per-variant publish target, index-parallel to
// existingVariants: each entry is that variant's existing content plus every accepted
// pipeline's values, so nothing already staged there is dropped. The caller needs it only
// on a round that changes the config; on a confirmed-unchanged round the exact target
// (cfg.SecretAssets()) is identical for every variant - secret resolution does not depend
// on the optimization mode - and is published as-is instead.
//
// config.BridgeAssets fails two ways, told apart exactly as resolveWorkloadPipelines does
// for DetectSecretSizeOverflow. A structural error (malformed spec) is returned as-is: the
// scan never got far enough to say anything about ANY pipeline, so the caller must not
// proceed as if nobody needs to wait. A per-pipeline *config.SecretSizeDataError (a
// declared Secret or key is missing) says nothing about the rest of the pool, so it must
// not block the bridge round: this returns as if no bridge were needed and leaves the
// broken pipeline's own Build*Config call, which hits the identical problem moments later,
// to report it against the pipeline it belongs to. That error comes from resolving the
// pipeline's own declared secret, so the first variant checked already surfaces it.
func planSecretAssetsBridge(
	ctx context.Context,
	getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error),
	assetsPrototype *corev1.Secret,
	finalPipelines []pipeline.Pipeline,
	existingVariants ...map[string][]byte,
) (bridgeDataPerVariant []map[string][]byte, bridgePipelines []pipeline.Pipeline, waitingPipelines []pipeline.Pipeline, err error) {
	if len(existingVariants) == 0 {
		return nil, finalPipelines, nil, nil
	}

	// Pass 1: determine, independently per variant, which pipelines it has room
	// for. A pipeline waits if ANY variant does not have room for it.
	waitingKeys := make(map[types.NamespacedName]struct{})
	for _, existing := range existingVariants {
		_, waiting, err := config.BridgeAssets(ctx, getter, assetsPrototype, existing, finalPipelines)
		if err != nil {
			var dataErr *config.SecretSizeDataError
			if errors.As(err, &dataErr) {
				return existingVariants, finalPipelines, nil, nil
			}
			return nil, nil, nil, err
		}
		for _, p := range waiting {
			waitingKeys[client.ObjectKeyFromObject(p)] = struct{}{}
		}
	}

	bridgePipelines = make([]pipeline.Pipeline, 0, len(finalPipelines))
	for _, p := range finalPipelines {
		if _, waits := waitingKeys[client.ObjectKeyFromObject(p)]; !waits {
			bridgePipelines = append(bridgePipelines, p)
		} else {
			waitingPipelines = append(waitingPipelines, p)
		}
	}

	// Pass 2: compute each variant's actual publish target using the (possibly
	// narrowed) bridgePipelines - guaranteed to fit in every variant, since it is a
	// subset of what pass 1 already independently proved fits in each of them. Always
	// runs, even when nobody waited (bridgePipelines == finalPipelines then) - the
	// caller still needs the real per-variant union data, not just a "no one is
	// waiting" signal, whenever the config itself is changing this round (see
	// createOrUpdateVector's configUnchanged branch).
	bridgeDataPerVariant = make([]map[string][]byte, len(existingVariants))
	for i, existing := range existingVariants {
		data, _, err := config.BridgeAssets(ctx, getter, assetsPrototype, existing, bridgePipelines)
		if err != nil {
			return nil, nil, nil, err
		}
		bridgeDataPerVariant[i] = data
	}
	return bridgeDataPerVariant, bridgePipelines, waitingPipelines, nil
}

// markWaitingPipelines writes secretAssetsWaitingReason to every pipeline in
// waitingPipelines whose status is not ALREADY that exact reason - avoiding a
// pointless status write (and the API traffic/resourceVersion churn it costs) on
// every single reconcile for a pipeline that is still waiting for the same reason it
// was last round. It clears RelatedSecretsHash for any pipeline whose status
// actually changes this call, the same idiom the collision/size-exclusion paths use,
// so a later reconcile of this pipeline cannot match a stale hash and skip itself.
func markWaitingPipelines(ctx context.Context, c client.Client, waitingPipelines []pipeline.Pipeline, workloadKind, workloadNamespace, workloadName string) error {
	if len(waitingPipelines) == 0 {
		return nil
	}
	reason := secretAssetsWaitingReason(workloadKind, workloadNamespace, workloadName)
	for _, p := range waitingPipelines {
		if err := writeAttributionReasonIfChanged(ctx, c, p, reason); err != nil {
			return err
		}
	}
	return nil
}

// writeAttributionReasonIfChanged writes reason (a collision, size-exclusion, or
// bridge-waiting reason) to p's status, but only if it actually differs from what is
// already stored - both to avoid a pointless write (and the API traffic/
// resourceVersion churn it costs) when nothing has changed, and to make sure a
// pipeline whose failure CLASS changes between rounds (e.g. it clears a collision but
// is now the one the size budget excludes, or a waiting pipeline becomes an outright
// size exclusion once a younger pipeline outbids it) gets the new, accurate reason
// instead of being left with stale text from the previous round forever: earlier
// versions of this function only rewrote the reason for a pipeline discovered as a
// victim for the FIRST time (gated on !wasRetryCandidate), which meant a retry
// candidate whose reason for failing changed shape kept displaying the OLD reason
// indefinitely, since nothing about its own spec ever changes to trigger a fresh
// per-pipeline reconcile that would overwrite it.
func writeAttributionReasonIfChanged(ctx context.Context, c client.Client, p pipeline.Pipeline, reason string) error {
	if r := p.GetReason(); r != nil && *r == reason {
		return nil
	}
	// The patch base is this pipeline as it stands before the mutations below, so the
	// write carries exactly what this call changes.
	base := p.DeepCopyObject().(pipeline.Pipeline)
	p.SetRelatedSecretsHash(nil)
	return pipeline.SetFailedStatus(ctx, c, p, reason, base)
}

// intersectPipelinesByKey returns the subset of pipelines whose ObjectKey also
// appears in include - used to narrow resolveWorkloadPipelines' reinstateCandidates
// down to only the ones planSecretAssetsBridge actually admitted into this round's
// bridgePipelines. A reinstate candidate that planSecretAssetsBridge instead put in
// waitingPipelines must not be reinstated: its references are not actually part of
// the config being published this round, and reinstatePipelines' own doc comment is
// exactly about not writing a success status before that is true.
func intersectPipelinesByKey(pipelines []pipeline.Pipeline, include []pipeline.Pipeline) []pipeline.Pipeline {
	if len(pipelines) == 0 || len(include) == 0 {
		return nil
	}
	keys := make(map[types.NamespacedName]struct{}, len(include))
	for _, p := range include {
		keys[client.ObjectKeyFromObject(p)] = struct{}{}
	}
	result := make([]pipeline.Pipeline, 0, len(pipelines))
	for _, p := range pipelines {
		if _, ok := keys[client.ObjectKeyFromObject(p)]; ok {
			result = append(result, p)
		}
	}
	return result
}

// resolveWorkloadPipelines lists the pipelines a workload build should use for filter
// and attributes both classes of shared-resource conflict a merged build can hit
// among them to whichever pipeline is younger (oldest CreationTimestamp wins),
// instead of letting Build*Config fail the whole build the way it does when called
// directly: a secret flat-key collision (config.DetectSecretCollisions) and an
// aggregated secret-assets Secret over corev1.MaxSecretSize
// (config.DetectSecretSizeOverflow) - see either function's doc comment for its own
// attribution policy. Size is checked only among collision survivors: a collision
// victim never reaches Build*Config either, so it must not consume any of the size
// budget.
//
// It also reconsiders pipelines this SAME filter previously excluded: it lists through
// pipeline.GetAllPipelines rather than GetValidPipelines, because by IsValid()==false
// an excluded pipeline would otherwise vanish from every future build with nothing left
// to wake it up. Once whatever it lost to is gone, it rejoins the build list and is
// returned as a reinstate candidate - status is NOT finalized here. Neither detector
// fully validates a pipeline (DetectSecretCollisions reads no Secret data at all;
// DetectSecretSizeOverflow reads bytes, not whether every declared key still exists), so
// a candidate that clears attribution can still be individually broken; only the merged
// Build*Config and the workload's own configcheck can catch that, and marking it valid
// before they run would write a status that is a lie. See reinstatePipelines for the
// required sequencing. A pipeline invalid for any other reason stays excluded, exactly
// as GetValidPipelines does today.
//
// A candidate whose spec was edited while failed is left alone instead (excluded, status
// untouched): that edit queued its own generation-change reconcile, which must validate
// the new spec from scratch. Without this guard the pipeline would come back as "valid"
// here while RelatedSecretsHash and LastAppliedPipelineHash still date from the
// failure-marking write, so the "no changes" skip in pipeline_controller.go would then
// hide the broken edit from its own reconcile permanently.
//
// Two accepted imprecisions while a conflict persists. Anything that reconciles the
// pipeline itself (operator restart, Secret rotation, annotation change) can flip it
// back to valid for one cycle before the next workload reconcile re-marks it - a
// bounded flicker, not a stuck state - and a candidate whose failure changes class
// (clears a collision, now loses on size) can keep last round's reason text, since the
// status of a still-excluded candidate is not rewritten. The exclusion itself is always
// correct; only the displayed reason can lag a round. Marking a pipeline failed is also
// global across workloads - see docs/secrets.md.
func resolveWorkloadPipelines(
	ctx context.Context,
	c client.Client,
	getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error),
	filter pipeline.FilterPipelines,
	workloadKind, workloadNamespace, workloadName string,
	// assetsPrototype is the aggregated secret-assets Secret exactly as the workload's
	// builder produces it, minus the data - what its object size is modelled against
	// (config.SecretAssetsObjectBudget). Passed as the built object rather than as a
	// name so the model charges the metadata the write actually carries; see
	// config.secretObjectBaseSize.
	assetsPrototype *corev1.Secret,
) (pipelines []pipeline.Pipeline, reinstateCandidates []pipeline.Pipeline, err error) {
	all, err := pipeline.GetAllPipelines(ctx, c, filter)
	if err != nil {
		return nil, nil, err
	}

	pool := make([]pipeline.Pipeline, 0, len(all))
	retryCandidates := make(map[types.NamespacedName]struct{})
	for _, p := range all {
		if p.IsValid() {
			pool = append(pool, p)
			continue
		}
		if reason := p.GetReason(); reason != nil && isSecretAttributionReason(*reason) {
			pool = append(pool, p)
			retryCandidates[client.ObjectKeyFromObject(p)] = struct{}{}
		}
	}

	// individuallyValid is the fallback for a structural detection failure (a
	// pipeline-level spec/shape problem while scanning, before either detector could
	// say anything about the pool at all): the individually-valid subset of pool,
	// exactly what GetValidPipelines would have returned before attribution existed.
	// Nothing is marked a victim and no retry candidate is reinstated on evidence
	// this shaky; Build*Config hits (and reports) the same underlying problem on its
	// own right after this returns. DetectSecretSizeOverflow's other error class - a
	// per-pipeline data-read failure, see its doc comment - is handled separately
	// and does NOT use this fallback, since it says nothing about the rest of pool.
	individuallyValid := func() []pipeline.Pipeline {
		validOnly := make([]pipeline.Pipeline, 0, len(pool))
		for _, p := range pool {
			if p.IsValid() {
				validOnly = append(validOnly, p)
			}
		}
		return validOnly
	}

	collisions, err := config.DetectSecretCollisions(getter, pool...)
	if err != nil {
		return individuallyValid(), nil, nil
	}
	collisionVictims := make(map[types.NamespacedName]config.SecretCollision, len(collisions))
	for _, col := range collisions {
		collisionVictims[client.ObjectKeyFromObject(col.Victim)] = col
	}

	// Size is checked only among collision survivors - a collision victim never
	// reaches Build*Config either, so it must not consume any of the size budget (see
	// this function's doc comment).
	survivors := make([]pipeline.Pipeline, 0, len(pool))
	for _, p := range pool {
		if _, isVictim := collisionVictims[client.ObjectKeyFromObject(p)]; !isVictim {
			survivors = append(survivors, p)
		}
	}

	sizeExclusions, err := config.DetectSecretSizeOverflow(ctx, getter, assetsPrototype, survivors...)
	if err != nil {
		var dataErr *config.SecretSizeDataError
		if errors.As(err, &dataErr) {
			// A per-pipeline data problem (its Secret or declared key is gone, the
			// API call failed, ...), not a structural pool-wide scan failure - see
			// DetectSecretSizeOverflow's doc comment. It says nothing about whether
			// the rest of the pool fits, so unlike the collision fallback below this
			// must not strip every retry candidate from the pool: just skip size
			// attribution for this round (nobody is marked a size victim) and let
			// Build*Config's own resolvePendingSecrets rediscover and report the
			// exact same problem for the one pipeline it actually belongs to.
			sizeExclusions = nil
		} else {
			return individuallyValid(), nil, nil
		}
	}
	sizeVictims := make(map[types.NamespacedName]config.SecretSizeExclusion, len(sizeExclusions))
	for _, ex := range sizeExclusions {
		sizeVictims[client.ObjectKeyFromObject(ex.Victim)] = ex
	}

	result := make([]pipeline.Pipeline, 0, len(pool))
	var reinstate []pipeline.Pipeline
	for _, p := range pool {
		key := client.ObjectKeyFromObject(p)
		_, wasRetryCandidate := retryCandidates[key]

		if col, isVictim := collisionVictims[key]; isVictim {
			reason := secretCollisionReason(col, workloadKind, workloadNamespace, workloadName)
			if err := writeAttributionReasonIfChanged(ctx, c, p, reason); err != nil {
				return nil, nil, err
			}
			continue
		}

		if ex, isVictim := sizeVictims[key]; isVictim {
			reason := secretSizeExclusionReason(ex, workloadKind, workloadNamespace, workloadName)
			if err := writeAttributionReasonIfChanged(ctx, c, p, reason); err != nil {
				return nil, nil, err
			}
			continue
		}

		if wasRetryCandidate {
			// Only reinstate a retry candidate whose spec is still the one that was
			// valid when it got marked failed - see the guard's rationale in this
			// function's doc comment above.
			unchanged, err := pipeline.IsPipelineChanged(p)
			if err != nil {
				return nil, nil, err
			}
			if !unchanged {
				continue
			}
			// Status write deliberately deferred to reinstatePipelines: the caller
			// must not call it until Build*Config (and, if enabled, the workload's
			// configcheck) have confirmed this candidate's references actually work
			// in the merged config - see this function's doc comment.
			reinstate = append(reinstate, p)
		}
		result = append(result, p)
	}
	return result, reinstate, nil
}

// reinstatePipelines finalizes the status of every retry candidate resolveWorkloadPipelines
// returned alongside its build list, marking each one valid via pipeline.SetSuccessStatus.
//
// The caller MUST only invoke this after Build*Config, the workload's own configcheck
// (if enabled) AND the workload's actual publish step (EnsureVectorAgent /
// EnsureVectorAggregator) have all succeeded for the config that includes these
// candidates - the publish step included, for two independent reasons:
//
//   - neither detector reads enough to clear a pipeline (collisions read no secret data,
//     size reads bytes but not key existence), so a candidate can still fail on something
//     only Build*Config's resolvePendingSecrets sees - a deleted key, a renamed backend;
//   - the publish step can fail after both of those succeed, most notably with
//     ErrStagedSecretAssetsTooLarge: the candidate's own values fit, but staging them
//     next to what the assets Secret still holds does not, which is only discovered
//     while writing.
//
// Marking a candidate valid earlier would claim success for a pipeline whose references
// never reached a published config. On any earlier failure the caller must skip this call
// entirely, leaving the failed status untouched.
func reinstatePipelines(ctx context.Context, c client.Client, candidates []pipeline.Pipeline) error {
	for _, p := range candidates {
		// Every candidate patches against its OWN base - the pipeline as it stands
		// before this call mutates it. One base shared across the loop would compute
		// each pipeline's status diff against a different pipeline's status.
		base := p.DeepCopyObject().(pipeline.Pipeline)
		if err := pipeline.SetSuccessStatus(ctx, c, p, base); err != nil {
			return err
		}
	}
	return nil
}
