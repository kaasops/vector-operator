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
	"errors"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/config/configcheck"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/kaasops/vector-operator/internal/utils/hash"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"github.com/kaasops/vector-operator/internal/vector/vectoragent"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	rbacv1 "k8s.io/api/rbac/v1"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/common"
)

// VectorReconciler reconciles a Vector object
type VectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset                 *kubernetes.Clientset
	ConfigCheckTimeout        time.Duration
	DiscoveryClient           *discovery.DiscoveryClient
	EventChan                 chan event.GenericEvent
	EnableConfigOptimization  bool
	EnableCheckpointMigration bool
	CheckpointMergerImage     string

	// APIReader is an uncached read-only client (mgr.GetAPIReader()), used to resolve
	// pipeline secrets: reads go through it for freshness and independence from cache
	// scoping (namespace/label filters), not to keep secret payloads out of the
	// controller-runtime cache - Owns(&corev1.Secret{}) below already puts them there in
	// default mode.
	APIReader client.Reader
}

// optimizeSources reports whether the agent config of the given Vector should be
// built with the sources optimization: the controller-level feature flag is on and
// the Vector CR is not opted out with the config-optimization=disabled annotation.
func optimizeSources(enabled bool, v *v1alpha1.Vector) bool {
	return enabled && v.Annotations[common.AnnotationConfigOptimization] != common.AnnotationValueDisabled
}

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get;list
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;patch;delete

func (r *VectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Vector", req.NamespacedName)
	log.Info("Start Reconcile Vector")
	if req.Namespace == "" {
		vectors, err := listVectorAgents(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to list vector instances")
			return ctrl.Result{}, err
		}
		return r.reconcileVectors(ctx, r.Client, r.Clientset, vectors...)
	}

	vectorCR, err := r.findVectorCustomResourceInstance(ctx, req)
	if err != nil {
		log.Error(err, "Failed to get Vector")
		return ctrl.Result{}, err
	}
	if vectorCR == nil {
		log.Info("Vector CR not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}
	return r.createOrUpdateVector(ctx, r.Client, r.Clientset, vectorCR)
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	monitoringCRD, err := k8s.ResourceExists(r.DiscoveryClient, monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Vector{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))).
		WatchesRawSource(source.Channel(r.EventChan, &handler.EnqueueRequestForObject{})).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{})

	if monitoringCRD {
		builder.Owns(&monitorv1.PodMonitor{})
	}

	if err = builder.Complete(r); err != nil {
		return err
	}
	return nil
}

func listVectorAgents(ctx context.Context, client client.Client) (vectors []*v1alpha1.Vector, err error) {
	vectorList := v1alpha1.VectorList{}
	err = client.List(ctx, &vectorList)
	if err != nil {
		return nil, err
	}
	for _, vector := range vectorList.Items {
		if vector.DeletionTimestamp != nil {
			continue
		}
		vectors = append(vectors, &vector)
	}
	return vectors, nil
}

func (r *VectorReconciler) findVectorCustomResourceInstance(ctx context.Context, req ctrl.Request) (*v1alpha1.Vector, error) {
	// fetch the master instance
	vectorCR := &v1alpha1.Vector{}
	err := r.Get(ctx, req.NamespacedName, vectorCR)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	setAgentTypeMetaIfNeeded(vectorCR)
	return vectorCR, nil
}

func (r *VectorReconciler) reconcileVectors(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, vectors ...*v1alpha1.Vector) (ctrl.Result, error) {
	if len(vectors) == 0 {
		return ctrl.Result{}, nil
	}

	for _, vector := range vectors {
		if vector.DeletionTimestamp != nil {
			continue
		}
		setAgentTypeMetaIfNeeded(vector)
		if _, err := r.createOrUpdateVector(ctx, client, clientset, vector); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *VectorReconciler) createOrUpdateVector(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, v *v1alpha1.Vector) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Vector", v.Name)

	// A terminating namespace rejects new content, so reconciling into it only produces
	// errors and requeues that starve the (serial) worker until the namespace is gone.
	if terminating, err := k8s.NamespaceIsTerminating(ctx, client, v.Namespace); err != nil {
		return ctrl.Result{}, err
	} else if terminating {
		log.Info("Skip reconcile: namespace is terminating or gone", "namespace", v.Namespace)
		return ctrl.Result{}, nil
	}

	// Init Controller for Vector Agent
	vaCtrl := vectoragent.NewController(v, client, clientset)
	// The write-order gate reads through this, never through the cached client -
	// see the Controller field's own doc comment.
	vaCtrl.APIReader = r.APIReader

	// Set BEFORE anything below reads or writes secret-assets: getSecretAssetsName()
	// (and therefore ExistingSecretAssets, called just below) is bound to
	// CheckpointMigration/OptimizeSources, so setting these late (as a previous
	// version of this function did, only right before EnsureVectorAgent) made
	// ExistingSecretAssets read the assets Secret under the WRONG name whenever
	// optimized mode was active - missing the variant actually mounted in pods and
	// risking a prune computed against a target the operator had not actually
	// checked for room.
	optimize := optimizeSources(r.EnableConfigOptimization, vaCtrl.Vector)
	if r.EnableCheckpointMigration {
		vaCtrl.CheckpointMigration = true
		vaCtrl.CheckpointMergerImage = r.CheckpointMergerImage
		vaCtrl.OptimizeSources = optimize
	}

	secretGetter := pipelineSecretGetter(r.APIReader, ctx)

	// Get Vector Config file. resolveWorkloadPipelines also attributes any secret
	// flat-key collision among the selected pipelines to the younger one instead of
	// letting BuildAgentConfig fail the whole build below. reinstateCandidates'
	// statuses are finalized further down, only once the build (and configcheck, if
	// enabled) actually succeed - see reinstatePipelines' doc comment.
	pipelines, reinstateCandidates, err := resolveWorkloadPipelines(ctx, vaCtrl.Client, secretGetter, pipeline.FilterPipelines{
		Scope:    pipeline.AllPipelines,
		Selector: vaCtrl.Vector.Spec.Selector,
		Role:     v1alpha1.VectorPipelineRoleAgent,
	}, "Vector", v.Namespace, v.Name, vaCtrl.SecretAssetsPrototype())
	if err != nil {
		return ctrl.Result{}, err
	}

	// pipelines is the correct, deterministic target set - computed independently of
	// the assets Secret's current contents (see resolveWorkloadPipelines' doc
	// comment). Whether all of it can actually be PUBLISHED this round is a separate
	// question: planSecretAssetsBridge may hold back a subset (bridgePipelines is the
	// safe-to-publish-now part, waitingPipelines the rest) if staging their values
	// alongside whatever the assets Secret currently, unprunably holds would itself
	// overflow corev1.MaxSecretSize - see its doc comment for the full rationale and
	// docs/secrets.md's "transitions that would themselves overflow" section.
	existingPrimary, existingAlt, err := vaCtrl.ExistingSecretAssets(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	existingVariants := []map[string][]byte{existingPrimary}
	if vaCtrl.CheckpointMigration {
		existingVariants = append(existingVariants, existingAlt)
	}
	bridgeDataPerVariant, bridgePipelines, waitingPipelines, err := planSecretAssetsBridge(ctx, secretGetter, vaCtrl.SecretAssetsPrototype(), pipelines, existingVariants...)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := markWaitingPipelines(ctx, vaCtrl.Client, waitingPipelines, "Vector", v.Namespace, v.Name); err != nil {
		return ctrl.Result{}, err
	}
	reinstateCandidates = intersectPipelinesByKey(reinstateCandidates, bridgePipelines)

	// Get Config in Json ([]byte)
	params := config.VectorConfigParams{
		ApiEnabled:           vaCtrl.Vector.Spec.Agent.Api.Enabled,
		PlaygroundEnabled:    vaCtrl.Vector.Spec.Agent.Api.Playground,
		UseApiServerCache:    vaCtrl.Vector.Spec.UseApiServerCache,
		InternalMetrics:      vaCtrl.Vector.Spec.Agent.InternalMetrics,
		ExpireMetricsSecs:    vaCtrl.Vector.Spec.Agent.ExpireMetricsSecs,
		OptimizeSources:      optimize,
		PipelineSecretGetter: secretGetter,
	}
	cfg, byteConfig, err := config.BuildAgentConfig(params, bridgePipelines...)
	if err != nil {
		if err := vaCtrl.SetFailedStatus(ctx, err.Error()); err != nil {
			return ctrl.Result{}, err
		}
		log.Error(err, "Build config failed")
		return ctrl.Result{}, nil
	}
	if collapsed, groups := cfg.OptimizationSummary(); collapsed > 0 {
		log.Info("Sources optimization collapsed kubernetes_logs sources", "sources", collapsed, "optimizedSources", groups)
	}

	cfgHash := int64(hash.Get(byteConfig))

	// configUnchanged tells us whether the config about to be (re-)written is
	// byte-identical to the one ALREADY ACTUALLY PUBLISHED on the cluster - a direct
	// read-and-compare (PublishedConfigMatches), not the status's LastAppliedConfigHash
	// (a 32-bit CRC32, kept below only as a user-visible status field and as the
	// configcheck-skip fast path's own historical signal). See PublishedConfigMatches'
	// doc comment for why a hash collision here is a real, demonstrated risk once the
	// signal also gates whether it is safe to prune the assets Secret - a false
	// "unchanged" would prune a key the live, actually-different config still needs.
	configUnchanged, err := vaCtrl.PublishedConfigMatches(ctx, byteConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !vaCtrl.Vector.Spec.Agent.ConfigCheck.Disabled {
		if vaCtrl.Vector.Status.LastAppliedConfigHash == nil || *vaCtrl.Vector.Status.LastAppliedConfigHash != cfgHash {
			configCheck := configcheck.New(
				byteConfig,
				vaCtrl.Client,
				vaCtrl.ClientSet,
				&vaCtrl.Vector.Spec.Agent.VectorCommon,
				vaCtrl.Vector.Name,
				vaCtrl.Vector.Namespace,
				r.ConfigCheckTimeout,
				configcheck.ConfigCheckInitiatorVector,
				cfg.SecretAssets(),
			)
			reason, err := configCheck.Run(ctx)
			if err != nil {
				if errors.Is(err, configcheck.ErrValidation) {
					if err := vaCtrl.SetFailedStatus(ctx, reason); err != nil {
						return ctrl.Result{}, err
					}
					log.Error(err, "Invalid config")
					return ctrl.Result{}, nil
				}
				if errors.Is(err, configcheck.ErrConfigcheckSkipped) {
					// namespace is terminating; the Vector CR is on its way out, nothing to do
					log.Info("ConfigCheck skipped, namespace is terminating")
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}
		}
	}

	vaCtrl.ByteConfig = byteConfig
	vaCtrl.Config = cfg

	// Built here, BEFORE the prune decision below - moved up from after it so that
	// allConfigsUnchanged (next) can already tell whether the alt variant has itself
	// caught up, not just the active one. See allConfigsUnchanged's own comment for
	// why checking only the active variant is not safe enough to gate a decision that
	// affects both.
	if r.EnableCheckpointMigration {
		mode := "legacy"
		if params.OptimizeSources {
			mode = "optimized"
		}
		log.Info("Checkpoint migration enabled; agent config secret bound to optimization mode",
			"mode", mode, "activeSecret", vaCtrl.ConfigSecretName())
		// the config of the opposite optimization mode: kept in the second
		// Secret so pods not yet rolled after a mode switch stay up to date. Built
		// from bridgePipelines, the same set the active config uses this round -
		// see planSecretAssetsBridge's doc comment on why both variants must agree
		// on which pipelines are actually published.
		//
		// A build failure here is deliberately not fatal, and its effect on the grace
		// period is accepted rather than worked around: AltByteConfig stays nil, which
		// forces allConfigsUnchanged false, so every round claims to be publishing and
		// keeps stamping a fresh LastConfigPublishedAt - the grace anchor moves forward
		// and stale keys are never pruned while the alt build keeps failing. That is the
		// conservative direction, and it does not spin: the prune decision requeues zero
		// on this path, the status stamp is filtered by GenerationChangedPredicate, and
		// republishing identical bytes is a no-op write, so only external events
		// re-trigger the controller.
		altParams := params
		altParams.OptimizeSources = !params.OptimizeSources
		if _, altBytes, err := config.BuildAgentConfig(altParams, bridgePipelines...); err != nil {
			log.Error(err, "Build alternate config failed, checkpoint migration secret not updated")
		} else {
			vaCtrl.AltByteConfig = altBytes
		}
	}

	// allConfigsUnchanged is configUnchanged broadened to every config Secret variant
	// that might be mounted, not just the active one. Checking only the active
	// variant is not safe enough to gate pruning or the publish mark: the two
	// variants are independent objects that can fail to write independently of each
	// other (see ensureVectorAgentSecretAssets' doc comment on why their assets are
	// never computed against each other's content either), so a round where the alt
	// write previously failed and the active write is simply retried would see
	// configUnchanged flip true on the active side alone - even though the alt config
	// Secret on the cluster is still the stale one, still referencing whatever it
	// used to. Pruning (or even just marking the publish clock) on that signal alone
	// would be free to drop a key the alt config secret was never actually updated to
	// stop referencing.
	//
	// A round where the alt build itself failed (AltByteConfig is nil) cannot claim
	// to know what alt should look like at all, so it is treated the same as "alt has
	// not caught up" - never as "alt is fine, nothing to check".
	allConfigsUnchanged := configUnchanged
	if vaCtrl.CheckpointMigration {
		if vaCtrl.AltByteConfig == nil {
			allConfigsUnchanged = false
		} else {
			altUnchanged, err := vaCtrl.AltPublishedConfigMatches(ctx, vaCtrl.AltByteConfig)
			if err != nil {
				return ctrl.Result{}, err
			}
			allConfigsUnchanged = allConfigsUnchanged && altUnchanged
		}
	}

	// On a round that changes nothing AND has waited out SecretAssetsPruneGracePeriod
	// since the live config was actually published, it is safe to prune ctrl.SecretAssets
	// down to what that unchanged config references - the same exact target for every
	// variant, since secret resolution does not depend on the optimization mode. On any
	// other round publish each variant's own bridgeDataPerVariant entry instead, so
	// nothing already staged in THAT variant is dropped before kubelet can catch up, and
	// no variant's target is computed against another's contents.
	//
	// The grace/requeue dance itself is only entered when publishing the exact target
	// would actually drop a key either variant currently has - see
	// assetsWouldDropAKey's doc comment for why a workload that never triggers a real
	// removal (no pipeline uses spec.secret at all, or this round is purely additive)
	// must not pay for a deferred reconcile it has no use for.
	targetAssets := cfg.SecretAssets()
	wouldDrop := assetsWouldDropAKey(existingPrimary, targetAssets) ||
		(vaCtrl.CheckpointMigration && assetsWouldDropAKey(existingAlt, targetAssets))

	var prune bool
	var requeueAfter time.Duration
	if wouldDrop {
		prune, requeueAfter = secretAssetsPruneDecision(allConfigsUnchanged, vaCtrl.Vector.Status.LastConfigPublishedAt)
	} else {
		prune = true
	}
	if prune {
		vaCtrl.SecretAssets = targetAssets
	} else {
		vaCtrl.SecretAssets = bridgeDataPerVariant[0]
		if vaCtrl.CheckpointMigration && len(bridgeDataPerVariant) > 1 {
			vaCtrl.AltSecretAssets = bridgeDataPerVariant[1]
		}
	}

	// Start Reconcile Vector Agent. configPublishing (!allConfigsUnchanged) tells it
	// whether to stamp the publish mark right before its first config write - see
	// StampConfigPublishing's doc comment for why that stamp has to live inside
	// EnsureVectorAgent, immediately before that write, rather than out here.
	if err := vaCtrl.EnsureVectorAgent(ctx, !allConfigsUnchanged); err != nil {
		return ctrl.Result{}, err
	}

	// Only now - after EnsureVectorAgent has actually published the config and
	// assets these candidates' references depend on - is it safe to mark them valid
	// again. Running this any earlier (e.g. right after Build*Config/configcheck, as
	// this used to) risks writing a green pipeline status the instant before the
	// actual Secret writes fail (an RBAC/quota rejection, a write conflict, ...),
	// leaving a lying "valid" pipeline next to a workload that never got its update -
	// exactly the stale "valid" status this whole feature exists to prevent.
	// See reinstatePipelines' doc comment. reinstateCandidates was already narrowed
	// to bridgePipelines above, so a candidate the bridge held back this round is
	// never reinstated on the strength of a build it is not actually part of.
	if err := reinstatePipelines(ctx, vaCtrl.Client, reinstateCandidates); err != nil {
		return ctrl.Result{}, err
	}

	if err := vaCtrl.SetSuccessStatus(ctx, &cfgHash, cfg.GetGlobalConfigHash(), !allConfigsUnchanged); err != nil {
		return ctrl.Result{}, err
	}

	// requeueAfter is non-zero exactly when this round held off pruning solely to
	// wait out SecretAssetsPruneGracePeriod - nothing else changed that would trigger
	// a fresh reconcile on its own (no Secret write, no pipeline status change), so
	// the operator has to explicitly ask to be woken up once the grace period is
	// actually over.
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func setAgentTypeMetaIfNeeded(cr *v1alpha1.Vector) {
	// https://github.com/kubernetes/kubernetes/issues/80609
	if cr.Kind == "" || cr.APIVersion == "" {
		cr.Kind = "Vector"
		cr.APIVersion = "observability.kaasops.io/v1alpha1"
	}
}
