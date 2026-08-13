/*
Copyright 2022.

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

package vectoragent

import (
	"bytes"
	"context"
	"fmt"

	"time"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/compression"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

// EnsureVectorAgent reconciles the agent DaemonSet and everything it depends on.
//
// ctrl.SecretAssets is written before the config that may reference it - see
// ensureVectorAgentSecretAssets for why that order is the one that cannot crash-loop the
// shared DaemonSet. What ctrl.SecretAssets holds this round is the CALLER's decision (the
// full target, or the staged union from planSecretAssetsBridge); this function publishes
// it without second-guessing the content.
//
// Where the DaemonSet's pod template write falls relative to assets/config depends on
// whether this round CROSSES the secret-assets mount's presence/absence boundary (see
// hasSecretAssetsMount). A pod created by any trigger - an unrelated node drain, an
// eviction, a new node - gets whatever template and config Secret are persisted at that
// instant, so a config must never reference what its own template does not mount.
// Ordering the three writes so the mount is only ever AHEAD of what the config
// references, never behind it, holds for every interleaving, not just the happy path:
//
//   - Gaining the mount for the first time: assets (so the mount's target already
//     exists) -> DaemonSet (mount added) -> config (now free to reference it). A pod
//     created between the DaemonSet and config writes gets the new template but the
//     still-old, reference-free config - harmless, nothing to resolve yet.
//   - Losing the mount entirely (the round after the last reference was already
//     dropped and pruned - see ensureVectorAgentSecretAssets): config (already
//     reference-free, from a PRIOR round) -> DaemonSet (mount removed) -> assets
//     (now safe to delete). Reversing this - dropping the mount before the assets
//     Secret it pointed to is gone - would just swap the crash-loop for a
//     FailedMount on any pod created in that window, which is exactly as broken.
//   - Neither crossing (the mount was, and remains, present or absent throughout):
//     the DaemonSet template's mount decision does not change this round, so the
//     original assets -> config -> DaemonSet order stands - reordering here would
//     only add churn for no safety gain.
//
// configPublishing is true exactly when the caller's allConfigsUnchanged was false
// this round - i.e. this call is actually about to (re-)write a config Secret, not
// just republish identical bytes. When true, StampConfigPublishing runs immediately
// before this round's FIRST config write - whichever branch that write falls in -
// closing the gap described in its own doc comment. In the two branches that write
// config after assets, that is right after assets succeeds; in the removal branch,
// where config goes first, it is the first thing the branch does.
func (ctrl *Controller) EnsureVectorAgent(ctx context.Context, configPublishing bool) error {
	log := log.FromContext(ctx).WithValues("vector-agent", ctrl.Vector.Name)
	log.Info("start Reconcile Vector Agent")

	monitoringCRD, err := k8s.ResourceExists(ctrl.ClientSet.Discovery(), monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}

	hadMount, err := ctrl.hasSecretAssetsMount(ctx)
	if err != nil {
		return err
	}
	willHaveMount := len(ctrl.SecretAssets) > 0

	stampBeforeConfig := func() error {
		if !configPublishing {
			return nil
		}
		return ctrl.StampConfigPublishing(ctx)
	}

	switch {
	case !hadMount && willHaveMount:
		if err := ctrl.ensureVectorAgentSecretAssets(ctx); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAgentDaemonSet(ctx); err != nil {
			return err
		}
		if err := stampBeforeConfig(); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAgentConfig(ctx); err != nil {
			return err
		}
	case hadMount && !willHaveMount:
		// The stamp is taken here too, even though this branch is normally reached
		// only after a prune (which needs allConfigsUnchanged, i.e. configPublishing
		// false). "Normally" is not "always": the prune decision is skipped outright
		// when publishing the exact target would drop no key (see assetsWouldDropAKey),
		// and that fast path is taken regardless of whether the config is changing -
		// so a round can land here with configPublishing true, and it is the first
		// config write of that round that then needs the mark. Stamping
		// unconditionally is what makes the ordering hold without depending on an
		// invariant this switch cannot enforce.
		if err := stampBeforeConfig(); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAgentConfig(ctx); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAgentDaemonSet(ctx); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAgentSecretAssets(ctx); err != nil {
			return err
		}
	default:
		if err := ctrl.ensureVectorAgentSecretAssets(ctx); err != nil {
			return err
		}
		if err := stampBeforeConfig(); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAgentConfig(ctx); err != nil {
			return err
		}
		// Kept right after config, before RBAC/Service/PodMonitor below - a
		// transient failure in one of those three, unrelated to secrets entirely,
		// must not withhold the template update (mirroring why the aggregator's
		// PodDisruptionBudget write is kept last: a non-essential step's failure
		// must not withhold something the workload's own correctness depends on).
		if err := ctrl.ensureVectorAgentDaemonSet(ctx); err != nil {
			return err
		}
	}

	if err := ctrl.ensureVectorAgentRBAC(ctx); err != nil {
		return err
	}

	if ctrl.Vector.Spec.Agent.Api.Enabled {
		if err := ctrl.ensureVectorAgentService(ctx); err != nil {
			return err
		}
	}

	if ctrl.Vector.Spec.Agent.InternalMetrics && monitoringCRD {
		if err := ctrl.ensureVectorAgentPodMonitor(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (ctrl *Controller) ensureVectorAgentRBAC(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent RBAC")

	if err := ctrl.ensureVectorAgentServiceAccount(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureVectorAgentClusterRole(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureVectorAgentClusterRoleBinding(ctx); err != nil {
		return err
	}

	return nil
}

func (ctrl *Controller) ensureVectorAgentServiceAccount(ctx context.Context) error {
	vectorAgentServiceAccount := ctrl.createVectorAgentServiceAccount()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentServiceAccount, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentClusterRole(ctx context.Context) error {
	vectorAgentClusterRole := ctrl.createVectorAgentClusterRole()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentClusterRole, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentClusterRoleBinding(ctx context.Context) error {
	vectorAgentClusterRoleBinding := ctrl.createVectorAgentClusterRoleBinding()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentClusterRoleBinding, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentService(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-service", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent Service")

	vectorAgentService := ctrl.createVectorAgentService()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentService, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentConfig(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-secret", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent Secret")

	vectorAgentSecret, err := ctrl.createVectorAgentConfig(ctx, ctrl.getConfigSecretName(), ctrl.ByteConfig)
	if err != nil {
		return err
	}
	if err := k8s.CreateOrUpdateResource(ctx, vectorAgentSecret, ctrl.Client); err != nil {
		return err
	}

	if ctrl.CheckpointMigration && ctrl.AltByteConfig != nil {
		altSecret, err := ctrl.createVectorAgentConfig(ctx, ctrl.getAltConfigSecretName(), ctrl.AltByteConfig)
		if err != nil {
			return err
		}
		return k8s.CreateOrUpdateResource(ctx, altSecret, ctrl.Client)
	}

	// Migration is off: the agent uses the single legacy-named secret. Remove the
	// standby (-opt) secret a previous migration-enabled run may have left, so the
	// feature gate is removable without leaking a stale, unmanaged config.
	if !ctrl.CheckpointMigration {
		if err := ctrl.deleteAgentConfigSecret(ctx, ctrl.getNameVectorAgent()+"-opt"); err != nil {
			log.Error(err, "failed to clean up standby agent config secret")
		}
	}
	return nil
}

// ExistingSecretAssets reads the CURRENT Data of every assets Secret variant that
// might still be mounted by a live pod - just the primary when checkpoint migration
// is off, primary AND alt when it is on. The caller (createOrUpdateVector) uses this
// to compute a safe bridge round via config.BridgeAssets/planSecretAssetsBridge
// before ever building the config this reconcile will publish.
//
// The two variants are returned SEPARATELY, never merged into one map: each is an
// independent Kubernetes object with its own separate corev1.MaxSecretSize budget,
// and planSecretAssetsBridge computes the bridge per variant against its own actual
// content for exactly that reason - see its doc comment for why a merged "existing"
// view would let two individually-valid variants combine into a fabricated,
// over-budget baseline that no real object on the cluster ever has to satisfy.
//
// alt is nil (not an empty map) when checkpoint migration is off, so callers can tell
// "there is no alt variant to consider" apart from "the alt variant exists but is
// currently empty". Absent Secrets are treated as empty, not an error - the common
// case (no pipeline references a secret yet, or this is the workload's first
// reconcile) has nothing to read.
// secretAssetsSafetyReader returns the uncached reader every secret-assets safeguard has
// to read through. Three decisions depend on it - the write-order gate, the bridge plan,
// and the prune gate - and all three are safeguards, so none of them may be decided by
// how far behind the informer cache happens to be: a stale snapshot that understates
// what the assets Secret currently holds makes a prune look free and drops a key a live
// config still references.
//
// Deliberately not defaulted to ctrl.Client: a caller that forgets to set APIReader
// fails loudly here instead of silently going back to reading the cache.
func (ctrl *Controller) secretAssetsSafetyReader(decision string) (client.Reader, error) {
	if ctrl.APIReader == nil {
		return nil, fmt.Errorf("APIReader is not set: %s must not be decided from the cache", decision)
	}
	return ctrl.APIReader, nil
}

func (ctrl *Controller) ExistingSecretAssets(ctx context.Context) (primary map[string][]byte, alt map[string][]byte, err error) {
	primary, err = ctrl.existingSecretAssetsFor(ctx, ctrl.getSecretAssetsName())
	if err != nil {
		return nil, nil, err
	}
	if !ctrl.CheckpointMigration {
		return primary, nil, nil
	}
	alt, err = ctrl.existingSecretAssetsFor(ctx, ctrl.getAltConfigSecretName()+"-secret-assets")
	if err != nil {
		return nil, nil, err
	}
	return primary, alt, nil
}

func (ctrl *Controller) existingSecretAssetsFor(ctx context.Context, name string) (map[string][]byte, error) {
	reader, err := ctrl.secretAssetsSafetyReader("the secret-assets bridge and prune plan")
	if err != nil {
		return nil, err
	}
	secret := &corev1.Secret{}
	err = reader.Get(ctx, client.ObjectKey{Namespace: ctrl.Vector.Namespace, Name: name}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string][]byte{}, nil
		}
		return nil, err
	}
	existing := make(map[string][]byte, len(secret.Data))
	for k, v := range secret.Data {
		existing[k] = v
	}
	return existing, nil
}

// PublishedConfigMatches reports whether the ACTIVE config Secret currently on the
// cluster already holds exactly byteConfig (compressed the same way this round
// would, if CompressConfigFile is enabled) - a direct byte comparison against the
// actually deployed artifact, not a hash. A 32-bit CRC32 (internal/utils/hash,
// originally only a configcheck-skip optimization where an occasional false
// "changed" just cost one extra validation pod) is not safe enough now that the
// identical signal also gates whether it is safe to prune the assets Secret: two
// distinct generated configs have been observed sharing a CRC32 (10648681) on a
// real snapshot, and a false "unchanged" there means pruning a key a
// live, different config still references. Byte comparison cannot collide - two
// different configs can never compare equal. Absent (not yet created) counts as "not
// published", i.e. changed - there is nothing live yet to safely prune against.
func (ctrl *Controller) PublishedConfigMatches(ctx context.Context, byteConfig []byte) (bool, error) {
	return ctrl.publishedConfigMatches(ctx, ctrl.getConfigSecretName(), byteConfig)
}

// AltPublishedConfigMatches is PublishedConfigMatches for the standby (alt) config
// Secret used under checkpoint migration - see PublishedConfigMatches' doc comment
// for the full byte-vs-hash rationale, which applies here identically.
//
// This exists because pruning must never be decided from the active variant alone:
// each variant is an independent object that can fail to write independently of the
// other (see ensureVectorAgentSecretAssets' doc comment on why the two variants'
// assets are never computed against each other's content), so a caller that only
// checked the active config could see it catch up on a retry after the alt write
// silently failed, and mistake that for "nothing has changed" - pruning shared logic
// gated on that alone would then be free to drop a key the still-stale alt config
// secret was never updated to stop referencing. Both variants must be confirmed
// unchanged before either is treated as safe to prune from.
func (ctrl *Controller) AltPublishedConfigMatches(ctx context.Context, altByteConfig []byte) (bool, error) {
	return ctrl.publishedConfigMatches(ctx, ctrl.getAltConfigSecretName(), altByteConfig)
}

func (ctrl *Controller) publishedConfigMatches(ctx context.Context, secretName string, byteConfig []byte) (bool, error) {
	reader, err := ctrl.secretAssetsSafetyReader("the secret-assets prune gate")
	if err != nil {
		return false, err
	}
	secret := &corev1.Secret{}
	err = reader.Get(ctx, client.ObjectKey{Namespace: ctrl.Vector.Namespace, Name: secretName}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	want := byteConfig
	if ctrl.Vector.Spec.Agent.CompressConfigFile {
		want = compression.Compress(byteConfig, log.FromContext(ctx))
	}
	return bytes.Equal(secret.Data["agent.json"], want), nil
}

// ensureVectorAgentSecretAssets keeps the secret-assets Secret in sync with
// ctrl.SecretAssets: replaces its Data outright with whatever the caller put there.
// Empty (the common case: no pipeline references a secret) means no Secret is
// created, and any leftover from a previous non-empty state is deleted so removing
// the last secret reference cleans up after itself.
//
// The assets name follows the active config Secret name, which flips with the
// checkpoint-migration mode. While migration is on, both config Secret variants are
// kept up to date so pods not yet rolled after a mode switch stay functional (see
// createOrUpdateVector); the assets Secret mirrors that lifecycle: both name
// variants carry the same data, and the standby variant is dropped together with
// the standby config Secret once migration is off.
//
// Called before ensureVectorAgentConfig: a config that references a flat key the
// mounted assets Secret does not have yet makes Vector's "directory" secret backend
// fail to resolve it - and unlike a failed live reload (which leaves the running
// process on its last-good config), that failure is baked into the Secret on disk:
// the very next pod restart for ANY reason (node drain, eviction, a new node) hits
// it and CrashLoopBackOffs, taking down every tenant sharing this DaemonSet, not
// just the one whose secret changed. So a key must never be referenced before it
// exists on disk.
//
// Whether that content is the full union or the exact, narrower target is decided
// upstream in createOrUpdateVector (ExistingSecretAssets/planSecretAssetsBridge), before
// ctrl.SecretAssets is set:
//
//   - On a round that is about to change the config it is the bridge union - existing
//     keys plus every newly-admitted pipeline's - so this call can never drop a key a
//     not-yet-rolled pod's old, still-live config depends on.
//   - On a round where the config is confirmed unchanged it is the exact target for the
//     pipelines in that config, safe to prune down to since that config no longer
//     references what gets dropped. The operator has no rollout-completion signal, so
//     the prune is deferred by at least one full reconcile rather than happening in the
//     same round the config changed. That narrows the window rather than closing it: a
//     pod restarting between the prune write and kubelet projecting it still crash-loops
//     on the missing key - the difference is duration, not kind - and it self-heals on
//     the next start, which picks up the config that already dropped the reference. On a
//     `kind` test cluster the window closed within 46-60 seconds; no bound is promised
//     elsewhere, since it depends on that cluster's kubelet sync, not on the operator.
func (ctrl *Controller) ensureVectorAgentSecretAssets(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-secret-assets", ctrl.Vector.Name)

	altName := ctrl.getAltConfigSecretName() + "-secret-assets"

	// Primary, handled entirely on its own: empty deletes it, non-empty writes it.
	if len(ctrl.SecretAssets) == 0 {
		if err := ctrl.deleteAgentConfigSecret(ctx, ctrl.getSecretAssetsName()); err != nil {
			return err
		}
	} else {
		log.Info("start Reconcile Vector Agent Secret Assets")
		if err := k8s.CreateOrUpdateResource(ctx, ctrl.createSecretAssetsSecret(ctrl.getSecretAssetsName()), ctrl.Client); err != nil {
			return err
		}
	}

	if !ctrl.CheckpointMigration {
		return ctrl.deleteAgentConfigSecret(ctx, altName)
	}

	// The alt variant's assets must only be touched (written, or deleted) when its
	// CONFIG was actually (re-)written this round too - ctrl.AltByteConfig != nil is
	// the exact condition ensureVectorAgentConfig itself uses to decide whether to
	// write the alt config Secret at all. If the alt config build failed this round
	// (config.BuildAgentConfig's error is only logged, AltByteConfig stays nil - see
	// createOrUpdateVector), the alt config Secret on the cluster is left exactly as
	// it was from the last round that DID succeed - so its assets must be left
	// exactly as they were too. Syncing (or worse, pruning) the alt assets to this
	// round's target regardless would starve a config Secret variant that still
	// references the OLD keys of the very data it depends on, permanently: nothing
	// about that stale config Secret ever triggers a reconcile of its own to notice
	// and fix the mismatch.
	if ctrl.AltByteConfig == nil {
		return nil
	}

	// AltSecretAssets is only set apart from SecretAssets during a bridge round
	// where the two variants' existing content genuinely diverged - see its doc
	// comment on the Controller struct; a bridge round can even leave primary empty
	// while alt still needs its own (different, non-empty) target, or vice versa -
	// so alt's own emptiness is checked independently of primary's, never inferred
	// from it. Absent an explicit AltSecretAssets, the alt variant gets the same
	// data as the primary (every non-bridge round, and any bridge round where the
	// two variants did not actually diverge).
	altData := ctrl.SecretAssets
	if ctrl.AltSecretAssets != nil {
		altData = ctrl.AltSecretAssets
	}
	if len(altData) == 0 {
		return ctrl.deleteAgentConfigSecret(ctx, altName)
	}
	altSecret := ctrl.createSecretAssetsSecret(altName)
	altSecret.Data = altData
	return k8s.CreateOrUpdateResource(ctx, altSecret, ctrl.Client)
}

// hasSecretAssetsMount reports whether the DaemonSet CURRENTLY PERSISTED on the
// cluster - not the one this reconcile is about to write - already has the
// OPERATOR'S OWN secret-assets mount in its pod template: a volume named
// "secret-assets" whose source is exactly this workload's assets Secret, mounted by
// the container at config.SecretsMountPath - see k8s.HasOperatorSecretAssetsMount's
// doc comment for why a bare name match is not enough (a user's own, unrelated
// volume that merely happens to be named "secret-assets" passes through untouched
// before the workload's first-ever secret reference, since SetAuthoritativeVolume is
// never called while ctrl.SecretAssets is still empty - mistaking that for the real
// mount is a real hazard, not hypothetical: it skips the gaining-the-mount write
// order below on the exact round that order exists for). Absent (not yet created,
// the workload's first-ever reconcile) counts as no mount, same as a template that
// exists but predates the feature ever being used.
//
// EnsureVectorAgent compares this against whether ctrl.SecretAssets is non-empty this
// round to detect an actual transition. The answer has to come from a real read rather
// than be inferred from ctrl.SecretAssets or the assets Secret: those can be out of sync
// with the DaemonSet's template after a partial failure in an earlier round (assets
// written, DaemonSet not), and inferring from the wrong object defeats the question.
//
// The read goes through ctrl.APIReader - uncached, at decision time. Not an atomicity
// claim: the DaemonSet can change the instant after this returns, which the write
// ordering covers. It is about not letting an informer cache that has not observed a
// preceding round's DaemonSet write pick the branch, since a safeguard decided off stale
// data is no safeguard. A read failure aborts the round before any write THIS gate
// orders - though not before any write at all, since the reconcile may already have
// created a configcheck pod and updated pipeline statuses.
// There is deliberately no fallback to the cached client: an unanswerable gate means
// the correct order is unknown, and guessing it is exactly the risk being avoided.
func (ctrl *Controller) hasSecretAssetsMount(ctx context.Context) (bool, error) {
	if ctrl.APIReader == nil {
		return false, fmt.Errorf("APIReader is not set: the secret-assets write-order gate must not be decided from the cache")
	}
	daemonSet := &appsv1.DaemonSet{}
	err := ctrl.APIReader.Get(ctx, client.ObjectKey{Namespace: ctrl.Vector.Namespace, Name: ctrl.getNameVectorAgent()}, daemonSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return k8s.HasOperatorSecretAssetsMount(daemonSet.Spec.Template.Spec, ctrl.getSecretAssetsName(), config.SecretsMountPath), nil
}

func (ctrl *Controller) ensureVectorAgentDaemonSet(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-daemon-set", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent DaemonSet")

	vectorAgentDaemonSet := ctrl.createVectorAgentDaemonSet()
	if ctrl.globalConfigChanged() {
		if vectorAgentDaemonSet.Spec.Template.Annotations == nil {
			vectorAgentDaemonSet.Spec.Template.Annotations = make(map[string]string)
		}
		vectorAgentDaemonSet.Spec.Template.Annotations[common.AnnotationRestartedAt] = time.Now().Format(time.RFC3339)
	}

	return k8s.CreateOrUpdateResource(ctx, vectorAgentDaemonSet, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentPodMonitor(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-podmonitor", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent PodMonitor")

	vectorAgentPodMonitor := ctrl.createVectorAgentPodMonitor()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentPodMonitor, ctrl.Client)
}

func (ctrl *Controller) matchLabelsForVectorAgent() map[string]string {
	return map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector",
		k8s.ComponentLabelKey: "Agent",
		k8s.InstanceLabelKey:  ctrl.Vector.Name,
	}
}

func (ctrl *Controller) labelsForVectorAgent() map[string]string {
	basicLabels := ctrl.matchLabelsForVectorAgent()

	labels := k8s.MergeLabels(basicLabels, ctrl.Vector.Spec.Agent.Labels)

	return labels
}

func (ctrl *Controller) annotationsForVectorAgent() map[string]string {
	return ctrl.Vector.Spec.Agent.Annotations
}

func (ctrl *Controller) objectMetaVectorAgent(labels map[string]string, annotations map[string]string, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ctrl.Vector.Name + "-agent",
		Namespace:       namespace,
		Labels:          labels,
		Annotations:     annotations,
		OwnerReferences: ctrl.getControllerReference(),
	}
}

func (ctrl *Controller) getNameVectorAgent() string {
	name := ctrl.Vector.Name + "-agent"
	return name
}

// ConfigSecretName is the exported accessor for the active config Secret name
// (for logging by the reconciler). Requires CheckpointMigration/OptimizeSources
// to be set on the Controller.
func (ctrl *Controller) ConfigSecretName() string {
	return ctrl.getConfigSecretName()
}

// getConfigSecretName returns the name of the config Secret the DaemonSet
// mounts. With checkpoint migration enabled the name is bound to the
// optimization mode: switching the mode changes the pod template (a rolling
// restart) instead of a live config reload, so the checkpoint-merger init
// container runs on every node exactly when it picks up the renamed sources.
func (ctrl *Controller) getConfigSecretName() string {
	if ctrl.CheckpointMigration && ctrl.OptimizeSources {
		return ctrl.getNameVectorAgent() + "-opt"
	}
	return ctrl.getNameVectorAgent()
}

func (ctrl *Controller) getAltConfigSecretName() string {
	if ctrl.CheckpointMigration && ctrl.OptimizeSources {
		return ctrl.getNameVectorAgent()
	}
	return ctrl.getNameVectorAgent() + "-opt"
}

// getSecretAssetsName returns the name of the Secret that materializes the
// pipeline secret data mounted at config.SecretsMountPath. It is derived from the
// active config Secret name, so it follows the same checkpoint-migration mode split.
func (ctrl *Controller) getSecretAssetsName() string {
	return ctrl.getConfigSecretName() + "-secret-assets"
}

// SecretAssetsPrototype returns the assets Secret exactly as this controller would
// build it, minus the data - the object the reconciler models against
// config.SecretAssetsObjectBudget. Built through the real builder so the model
// charges the labels, annotations and ownerReference the write will actually carry,
// and cannot drift from the builder later.
func (ctrl *Controller) SecretAssetsPrototype() *corev1.Secret {
	prototype := ctrl.createSecretAssetsSecret(ctrl.getSecretAssetsName())
	prototype.Data = nil
	return prototype
}

func (ctrl *Controller) getControllerReference() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         ctrl.Vector.APIVersion,
			Kind:               ctrl.Vector.Kind,
			Name:               ctrl.Vector.GetName(),
			UID:                ctrl.Vector.GetUID(),
			BlockOwnerDeletion: ptr.To(true),
			Controller:         ptr.To(true),
		},
	}
}

func (ctrl *Controller) globalConfigChanged() bool {
	globalCfgHash := ctrl.Config.GetGlobalConfigHash()
	if ctrl.Vector.Status.LastAppliedGlobalConfigHash == nil {
		return false
	}
	return *ctrl.Vector.Status.LastAppliedGlobalConfigHash != *globalCfgHash
}
