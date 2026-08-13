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
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

type Controller struct {
	client.Client
	Vector *vectorv1alpha1.Vector

	// APIReader is an uncached read-only client (mgr.GetAPIReader()), used by every
	// secret-assets safeguard: the write-order gate (hasSecretAssetsMount), the bridge
	// plan's view of what the assets Secret currently holds (ExistingSecretAssets), and
	// the prune gate (PublishedConfigMatches). A safeguard must not be decided by how far
	// behind the informer cache happens to be - see secretAssetsSafetyReader.
	APIReader client.Reader

	ByteConfig []byte
	Config     *config.VectorConfig
	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	ClientSet *kubernetes.Clientset

	// Checkpoint migration (--enable-checkpoint-migration): the config Secret
	// name is bound to the optimization mode, so switching the mode changes the
	// pod template and rolls the DaemonSet instead of a live reload, and a
	// checkpoint-merger init container consolidates file checkpoints saved
	// under the previous source names before vector starts. AltByteConfig
	// holds the config of the opposite mode: both Secrets are kept up to date,
	// so pods not yet rolled keep receiving pipeline updates of their mode.
	CheckpointMigration   bool
	CheckpointMergerImage string
	OptimizeSources       bool
	AltByteConfig         []byte

	// SecretAssets holds the resolved pipeline secret data (cfg.SecretAssets()) to
	// materialize into the secret-assets Secret and mount into the DaemonSet. Empty
	// when no pipeline references a secret (zero-churn: no Secret, no volume, no mount).
	SecretAssets map[string][]byte

	// AltSecretAssets is the alt (standby) assets Secret's own publish target, when
	// it must differ from SecretAssets - only possible during a bridge round (see
	// planSecretAssetsBridge), since the two variants can have diverged existing
	// content (e.g. after an earlier alt build failure left it behind) even though
	// they are converging on the same bridgePipelines. Nil means "same as
	// SecretAssets" - the common case, including every non-bridge round, since the
	// exact/pruned target is a pure function of bridgePipelines and does not depend
	// on which variant's config referenced it.
	AltSecretAssets map[string][]byte
}

func NewController(v *vectorv1alpha1.Vector, c client.Client, cs *kubernetes.Clientset) *Controller {
	ctrl := &Controller{
		Client:    c,
		Vector:    v,
		ClientSet: cs,
	}
	ctrl.SetDefault()
	return ctrl
}

// configPublished is true when the config Secret was actually (re-)written this
// round (i.e. the caller's configUnchanged was false) - the exact condition that
// starts (or restarts) the secret-assets prune grace period, see
// SecretAssetsPruneGracePeriod. LastConfigPublishedAt is also seeded when it is
// still nil regardless of configPublished, so a workload with no prior record (a
// fresh install, or an upgrade from before this field existed) is treated as "just
// published" rather than silently unlocking an immediate prune with no reference
// point to measure the grace period against.
func (ctrl *Controller) SetSuccessStatus(ctx context.Context, cfgHash, globCfgHash *int64, configPublished bool) error {
	base := ctrl.Vector.DeepCopy()
	// A merge patch only clears the keys it mentions, and base can predate the reason it
	// has to clear, so make the patch carry reason whatever base was read with.
	base.Status.Reason = ptr.To("")
	var status = true
	ctrl.Vector.Status.ConfigCheckResult = &status
	ctrl.Vector.Status.Reason = nil
	ctrl.Vector.Status.LastAppliedConfigHash = cfgHash
	ctrl.Vector.Status.LastAppliedGlobalConfigHash = globCfgHash
	if configPublished || ctrl.Vector.Status.LastConfigPublishedAt == nil {
		now := metav1.Now()
		ctrl.Vector.Status.LastConfigPublishedAt = &now
	}

	return k8s.PatchStatus(ctx, ctrl.Vector, base, ctrl.Client)
}

// StampConfigPublishing writes ONLY LastConfigPublishedAt, deliberately touching
// nothing else in status (not ConfigCheckResult, not Reason, not the applied
// hashes) - unlike SetSuccessStatus, this is not a "the round succeeded" signal, so
// it must never let a pipeline or the workload appear valid before the actual
// Secret writes that follow it are known to have succeeded (see reinstatePipelines'
// doc comment and createOrUpdateVector's identical ordering rationale for why that
// distinction matters).
//
// EnsureVectorAgent calls this, when configPublishing is true, immediately before the
// FIRST config Secret write of the round in every branch of its write-order switch. That
// closes the gap SetSuccessStatus alone leaves: it runs only at the very end of
// createOrUpdateVector, so a round that published a new config and then failed on the
// DaemonSet, RBAC, Service, PodMonitor or reinstatement never reached it, leaving
// LastConfigPublishedAt at some much earlier round's value. A retry would then find
// PublishedConfigMatches true and, gated on that stale mark, open the grace period
// immediately. Stamping right before the write keeps the mark at least as fresh as the
// config it protects: staged-but-unwritten costs nothing, while stamping later would let
// the assets-staging step eat into the grace period it is meant to cover.
//
// The patch base is a copy taken before the stamp, so the diff carries this one field and
// nothing else: this is not a "the whole status is now what I hold" write.
//
// This is the one status write that happens OUTSIDE SetSuccessStatus, which matters
// because SetSuccessStatus takes its own patch base at write time: by then the stamp is
// already persisted AND already on ctrl.Vector, so it is identical on both sides of that
// later diff and is correctly left out of it. Any future status field written outside a
// setter has to keep that property - a field mutated in memory before SetSuccessStatus
// runs, and never written by its own call, would sit in that base too and silently never
// reach the cluster.
//
// Writes through a DEEP COPY of ctrl.Vector - a real bug, not a hypothetical.
// Status().Patch() decodes the API server's full response, spec included, back into the
// object it was given, exactly as Update() did (controller-runtime typed_client.go,
// PatchSubResource ends in Do(ctx).Into(body)), even though only /status was touched on
// the wire. ctrl.Vector's
// Spec still carries this reconcile's in-memory-only SetDefault() values (Image, Volumes,
// Resources), which were never persisted, so updating through it directly would revert
// them, and the config/DaemonSet builders called right after would build from the
// un-defaulted spec. Only what later status writes in this reconcile need - the new
// ResourceVersion, so they do not lose a conflict, and the field this call sets - is
// copied back.
func (ctrl *Controller) StampConfigPublishing(ctx context.Context) error {
	now := metav1.Now()
	base := ctrl.Vector.DeepCopy()
	stamped := ctrl.Vector.DeepCopy()
	stamped.Status.LastConfigPublishedAt = &now
	if err := k8s.PatchStatus(ctx, stamped, base, ctrl.Client); err != nil {
		return err
	}
	ctrl.Vector.ResourceVersion = stamped.ResourceVersion
	ctrl.Vector.Status.LastConfigPublishedAt = &now
	return nil
}

func (ctrl *Controller) SetFailedStatus(ctx context.Context, reason string) error {
	base := ctrl.Vector.DeepCopy()
	var status = false
	ctrl.Vector.Status.ConfigCheckResult = &status
	ctrl.Vector.Status.Reason = &reason

	return k8s.PatchStatus(ctx, ctrl.Vector, base, ctrl.Client)
}
