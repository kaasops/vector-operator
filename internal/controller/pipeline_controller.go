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
	"fmt"
	"hash/fnv"
	"reflect"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/config/configcheck"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"github.com/kaasops/vector-operator/internal/vector/aggregator"
	"github.com/kaasops/vector-operator/internal/vector/vectoragent"
)

type PipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset                                *kubernetes.Clientset
	ConfigCheckTimeout                       time.Duration
	VectorAgentEventCh                       chan event.GenericEvent
	VectorAggregatorsEventCh                 chan event.GenericEvent
	ClusterVectorAggregatorsEventCh          chan event.GenericEvent
	EnableReconciliationInvalidPipelines     bool
	ReconciliationInvalidPipelinesRetryDelay time.Duration
	EnableConfigOptimization                 bool

	// APIReader is an uncached read-only client (mgr.GetAPIReader()), used to resolve
	// pipeline secrets: reads go through it for freshness and independence from cache
	// scoping (namespace/label filters), not to keep secret payloads out of the
	// controller-runtime cache - the workload reconcilers' Owns(&corev1.Secret{}) already
	// puts them there in default mode.
	APIReader client.Reader

	// SecretIndex tracks which pipelines declare which Secrets (spec.secret), kept
	// current on every reconcile of a pipeline that has spec.secret set. The Secret
	// watch (added on top of this reconciler separately) uses it to resolve which
	// pipelines to requeue when a Secret changes.
	SecretIndex *pipeline.SecretIndex

	// PollSecretRotation turns on the periodic re-check described by
	// secretRotationPollInterval. Set exactly when the operator runs scoped
	// (--watch-namespace and/or --watch-name): that is the only configuration where
	// the Secret watch can miss a rotation outright. In default mode the watch already
	// covers every referenced Secret, and polling would be API load buying nothing.
	PollSecretRotation bool
}

var (
	ErrBuildConfigFailed = errors.New("failed to build config")
)

// relatedSecretsResolveRetryDelay is how long to wait before re-checking a pipeline
// whose declared secret backends could not be resolved (e.g. the Secret doesn't exist
// yet) - the same treatment as any other pipeline-invalid path, just on a fixed short
// delay since there is no user-facing spec change to wait for.
const relatedSecretsResolveRetryDelay = 10 * time.Second

// secretRotationPollBase and secretRotationPollSpread bound the interval at which a
// scoped operator re-checks a pipeline's referenced Secrets on its own initiative -
// see secretRotationPollInterval for why the interval is per-pipeline rather than
// fixed, and PollSecretRotation for when this runs at all.
const (
	secretRotationPollBase   = 4 * time.Minute
	secretRotationPollSpread = 1 * time.Minute
)

// secretRotationPollInterval returns this pipeline's own poll interval: somewhere in
// [4m, 5m), decided by a hash of its namespace/name and therefore STABLE across
// operator restarts and identical on every replica.
//
// The spread is the point: a fixed interval would set every pipeline's timer at the same
// instant an operator starts, so a cluster with thousands of pipelines would re-resolve
// all of them in the same second every four minutes - a self-inflicted thundering herd
// against the API server. Hashing the identity spreads them over the window and keeps
// each pipeline's phase stable across restarts, so the herd does not re-form after a
// rollout. Identity rather than rand also keeps the interval from drifting between
// reconciles: the requeue is re-armed every round, and client-go's delaying queue keeps
// the EARLIEST deadline for an already-queued key, so frequent reconciles never starve a
// pipeline of its poll.
func secretRotationPollInterval(key types.NamespacedName) time.Duration {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key.Namespace))
	// Separator: without it "ab"/"c" and "a"/"bc" hash identically, which would hand
	// two different pipelines the same phase for no reason.
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key.Name))
	// Reduced in MILLISECONDS, not nanoseconds. Taking the hash modulo the spread
	// expressed in nanoseconds reads correctly and is not: a uint32 tops out at
	// ~4.29e9, which is under 4.3 SECONDS, so a modulo against 60e9 never wraps and the
	// offset collapses to the raw hash - a 4.3-second window pretending to be a minute,
	// and with it most of the herd this jitter exists to break up. Milliseconds are far
	// finer than a poll interval needs and leave the reduction room to actually work.
	return secretRotationPollBase +
		time.Duration(h.Sum32()%uint32(secretRotationPollSpread/time.Millisecond))*time.Millisecond
}

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines;clustervectorpipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines/status;clustervectorpipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines/finalizers;clustervectorpipelines/finalizers,verbs=update

// Reconcile wraps the reconcile body with the scoped-mode rotation poll, deliberately
// as a wrapper rather than at each return: the body has ~20 exits (no-op, success,
// every flavour of "this pipeline is invalid"), and every one of them EXCEPT a
// returned error has to keep the next poll armed. The one that matters most is the
// least obvious: a pipeline turned red by a bad secret value stops being reconciled by
// anything else, so if the poll were dropped on that path, FIXING the value would
// never be noticed - the very failure this feature exists to prevent, just one step
// later. A returned error needs no arming: controller-runtime requeues it with
// backoff and ignores RequeueAfter entirely.
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile(ctx, req)
	if err != nil {
		return result, err
	}
	return r.armSecretRotationPoll(ctx, req, result), nil
}

// armSecretRotationPoll lowers result's requeue to this pipeline's poll interval when
// scoped-mode polling applies to it.
//
// It only ever moves the wakeup EARLIER (a zero RequeueAfter means "no wakeup", so it
// counts as infinitely far away). The short retries the body sets for its own reasons -
// relatedSecretsResolveRetryDelay, ReconciliationInvalidPipelinesRetryDelay - are
// therefore untouched: they are seconds, the poll is minutes, and polling is meant to
// be a floor under them, never a replacement that would slow a recovery down.
//
// A pipeline with no USED secret backend is left alone: nothing about it can change
// behind the operator's back, so there is nothing to poll for. Declared-but-unreferenced
// backends do not count, matching resolveRelatedSecrets' own used-refs-only rule.
func (r *PipelineReconciler) armSecretRotationPoll(ctx context.Context, req ctrl.Request, result ctrl.Result) ctrl.Result {
	if !r.PollSecretRotation {
		return result
	}

	p, err := r.getPipeline(ctx, req)
	if err != nil || p == nil {
		// Deleted, or unreadable this round. Nothing to poll for, and nothing worth
		// failing the round over - the body already returned its own verdict.
		return result
	}
	if len(p.GetSpec().Secret) == 0 {
		return result
	}
	used, err := config.UsedSecretBackends(p)
	if err != nil || len(used) == 0 {
		return result
	}

	poll := secretRotationPollInterval(types.NamespacedName{Namespace: p.GetNamespace(), Name: p.GetName()})
	if result.RequeueAfter == 0 || poll < result.RequeueAfter {
		result.RequeueAfter = poll
	}
	return result
}

func (r *PipelineReconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Pipeline", req.Name)

	log.Info("start Reconcile Pipeline")
	pipelineCR, err := r.getPipeline(ctx, req)
	if err != nil {
		log.Error(err, "Failed to get Pipeline")
		return ctrl.Result{}, err
	}
	// Secret index maintenance and resolution run before the workloads-not-found
	// early return below: an operator restarted (empty in-memory index) in a cluster
	// whose workload CRs only appear later would otherwise never index this
	// pipeline's secrets, and rotations would stop requeuing it until some unrelated
	// reconcile happened by.
	if pipelineCR == nil {
		if r.SecretIndex != nil {
			r.SecretIndex.Set(req.NamespacedName, nil)
		}
	}

	// basePipeline is the pipeline as it was read at the start of this reconcile: every
	// status write of the round patches against it, so the patch carries whatever the
	// round touched since (see pipeline.SetSuccessStatus). Taken here rather than after
	// the workload lookup below, because the secret resolution that follows can already
	// fail the pipeline and needs the same base.
	var basePipeline pipeline.Pipeline
	if pipelineCR != nil {
		basePipeline = pipelineCR.DeepCopyObject().(pipeline.Pipeline)
	}

	var newRelatedSecretsHash *int64
	if pipelineCR != nil {
		pipelineKey := types.NamespacedName{Namespace: pipelineCR.GetNamespace(), Name: pipelineCR.GetName()}
		if used, usedErr := config.UsedSecretBackends(pipelineCR); usedErr != nil {
			// Malformed spec JSON: clear the index entry and fall through - the
			// UnmarshalJson below fails the pipeline with its established message.
			if r.SecretIndex != nil {
				r.SecretIndex.Set(pipelineKey, nil)
			}
		} else if len(pipelineCR.GetSpec().Secret) > 0 {
			refs, token, err := resolveRelatedSecrets(ctx, r.APIReader, pipelineCR, used)
			if r.SecretIndex != nil {
				// refs is fully populated even when err != nil, so the index stays
				// accurate (and the watch can find this pipeline again) even while a
				// referenced secret is missing.
				r.SecretIndex.Set(pipelineKey, refs)
			}
			if err != nil {
				log.Error(err, "Failed to resolve pipeline secrets")
				// Clear the stored hash before writing the failure: SetFailedStatus
				// below writes LastAppliedPipelineHash for the *current* (unchanged)
				// spec, so without this a later recovery reconcile could see
				// notChanged==true with a stale RelatedSecretsHash still matching and
				// skip itself via the "Pipeline has no changes" branch below forever.
				pipelineCR.SetRelatedSecretsHash(nil)
				if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, err.Error(), basePipeline); err != nil {
					return ctrl.Result{}, err
				}
				var shapeErr *invalidSecretShapeError
				if errors.As(err, &shapeErr) {
					// Permanent spec error: only editing the pipeline fixes it, and that
					// edit's generation change already triggers a reconcile on its own.
					return ctrl.Result{}, nil
				}
				return ctrl.Result{RequeueAfter: relatedSecretsResolveRetryDelay}, nil
			}
			newRelatedSecretsHash = token
		} else if r.SecretIndex != nil {
			r.SecretIndex.Set(pipelineKey, nil)
		}
	}

	vectorAgents, err := listVectorAgents(ctx, r.Client)
	if err != nil {
		log.Error(err, "Failed to get vector agents")
		return ctrl.Result{}, nil
	}
	vectorAggregators, err := listVectorAggregators(ctx, r.Client)
	if err != nil {
		log.Error(err, "Failed to get vector aggregators")
		return ctrl.Result{}, nil
	}
	clusterVectorAggregators, err := listClusterVectorAggregators(ctx, r.Client)
	if err != nil {
		log.Error(err, "Failed to get cluster vector aggregators")
		return ctrl.Result{}, nil
	}

	if len(vectorAgents) == 0 && len(vectorAggregators) == 0 && len(clusterVectorAggregators) == 0 {
		log.Info("Vectors not found")
		return ctrl.Result{}, nil
	}

	if pipelineCR == nil {
		log.Info("Pipeline CR not found. Ignoring since object must be deleted")
		for _, vector := range vectorAgents {
			r.VectorAgentEventCh <- event.GenericEvent{Object: vector}
		}
		for _, vector := range vectorAggregators {
			r.VectorAggregatorsEventCh <- event.GenericEvent{Object: vector}
		}
		for _, vector := range clusterVectorAggregators {
			r.ClusterVectorAggregatorsEventCh <- event.GenericEvent{Object: vector}
		}
		return ctrl.Result{}, nil
	}

	if !r.EnableReconciliationInvalidPipelines || pipelineCR.IsValid() {
		notChanged, err := pipeline.IsPipelineChanged(pipelineCR)
		if err != nil {
			return ctrl.Result{}, err
		}
		if notChanged && relatedSecretsHashEqual(pipelineCR.GetRelatedSecretsHash(), newRelatedSecretsHash) {
			log.Info("Pipeline has no changes. Finish Reconcile Pipeline")
			return ctrl.Result{}, nil
		}
	}

	pipelineCR.SetRelatedSecretsHash(newRelatedSecretsHash)

	p := &config.PipelineConfig{}
	if err := config.UnmarshalJson(pipelineCR.GetSpec(), p); err != nil {
		if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, fmt.Sprintf("Failed to unmarshal vector pipeline %s", err.Error()), basePipeline); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set pipeline status %s: %w", pipelineCR.GetName(), err)
		}
		return ctrl.Result{}, nil
	}

	pipelineVectorRole, err := resolvePipelineRole(p, pipelineCR)
	if err != nil {
		if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, err.Error(), basePipeline); err != nil {
			log.Error(err, "Failed to set pipeline status")
			return ctrl.Result{}, err
		}
		log.Error(err, "Failed to determine pipeline role")
		return ctrl.Result{}, nil
	}
	pipelineCR.SetRole(pipelineVectorRole)
	var pipelineLabels map[string]string
	if pipelineCR.GetLabels() != nil {
		pipelineLabels = pipelineCR.GetLabels()
	}
	eg := errgroup.Group{}

	if *pipelineVectorRole == v1alpha1.VectorPipelineRoleAgent {

		for _, vector := range vectorAgents {
			var selectorLabels map[string]string
			if vector.Spec.Selector != nil {
				selectorLabels = vector.Spec.Selector.MatchLabels
			}
			if !k8s.MatchLabels(selectorLabels, pipelineLabels) {
				continue
			}
			eg.Go(func() error {
				vaCtrl := vectoragent.NewController(vector, r.Client, r.Clientset)
				cfg, byteConfig, err := config.BuildAgentConfig(config.VectorConfigParams{
					ApiEnabled:           vaCtrl.Vector.Spec.Agent.Api.Enabled,
					PlaygroundEnabled:    vaCtrl.Vector.Spec.Agent.Api.Playground,
					UseApiServerCache:    vaCtrl.Vector.Spec.UseApiServerCache,
					InternalMetrics:      vaCtrl.Vector.Spec.Agent.InternalMetrics,
					ExpireMetricsSecs:    vaCtrl.Vector.Spec.Agent.ExpireMetricsSecs,
					OptimizeSources:      optimizeSources(r.EnableConfigOptimization, vaCtrl.Vector),
					PipelineSecretGetter: pipelineSecretGetter(r.APIReader, ctx),
				}, pipelineCR)
				if err != nil {
					return fmt.Errorf("agent %s/%s build config failed: %w: %w", vector.Namespace, vector.Name, ErrBuildConfigFailed, err)
				}

				vaCtrl.Config = cfg
				vaCtrl.ByteConfig = byteConfig

				configCheck := configcheck.New(
					vaCtrl.ByteConfig,
					vaCtrl.Client,
					vaCtrl.ClientSet,
					&vaCtrl.Vector.Spec.Agent.VectorCommon,
					vaCtrl.Vector.Name,
					vaCtrl.Vector.Namespace,
					r.ConfigCheckTimeout,
					configcheck.ConfigCheckInitiatorPipieline,
					cfg.SecretAssets(),
				)

				reason, err := configCheck.Run(ctx)
				if errors.Is(err, configcheck.ErrConfigcheckSkipped) {
					return nil
				}
				if reason != "" {
					return fmt.Errorf("agent %s/%s config check failed: %s", vector.Namespace, vector.Name, reason)
				}
				return err
			})
		}

	} else {

		if pipelineCR.GetNamespace() != "" {
			for _, vector := range vectorAggregators {
				// VectorPipeline should only be validated against VectorAggregator in the same namespace
				if vector.Namespace != pipelineCR.GetNamespace() {
					continue
				}
				var selectorLabels map[string]string
				if vector.Spec.Selector != nil {
					selectorLabels = vector.Spec.Selector.MatchLabels
				}
				if !k8s.MatchLabels(selectorLabels, pipelineLabels) {
					continue
				}
				eg.Go(func() error {
					vaCtrl := aggregator.NewController(vector, r.Client, r.Clientset)
					cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{
						AggregatorName:       vaCtrl.Name,
						ApiEnabled:           vaCtrl.Spec.Api.Enabled,
						PlaygroundEnabled:    vaCtrl.Spec.Api.Playground,
						InternalMetrics:      vaCtrl.Spec.InternalMetrics,
						ExpireMetricsSecs:    vaCtrl.Spec.ExpireMetricsSecs,
						PipelineSecretGetter: pipelineSecretGetter(r.APIReader, ctx),
					}, pipelineCR)
					if err != nil {
						return fmt.Errorf("aggregator %s/%s build config failed: %w: %w", vector.Namespace, vector.Name, ErrBuildConfigFailed, err)
					}
					if err != nil {
						return err
					}

					byteConfig, err := cfg.MarshalJSON()
					if err != nil {
						return err
					}

					vaCtrl.ConfigBytes = byteConfig
					vaCtrl.Config = cfg

					configCheck := configcheck.New(
						vaCtrl.ConfigBytes,
						vaCtrl.Client,
						vaCtrl.ClientSet,
						&vaCtrl.Spec.VectorCommon,
						vaCtrl.Name,
						vaCtrl.Namespace,
						r.ConfigCheckTimeout,
						configcheck.ConfigCheckInitiatorPipieline,
						cfg.SecretAssets(),
					)

					reason, err := configCheck.Run(ctx)
					if errors.Is(err, configcheck.ErrConfigcheckSkipped) {
						return nil
					}
					if reason != "" {
						return fmt.Errorf("aggregator %s/%s config check failed: %s", vector.Namespace, vector.Name, reason)
					}
					return err
				})
			}

		} else {

			for _, vector := range clusterVectorAggregators {
				var selectorLabels map[string]string
				if vector.Spec.Selector != nil {
					selectorLabels = vector.Spec.Selector.MatchLabels
				}
				if !k8s.MatchLabels(selectorLabels, pipelineLabels) {
					continue
				}
				eg.Go(func() error {
					vaCtrl := aggregator.NewController(vector, r.Client, r.Clientset)
					cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{
						AggregatorName:       vaCtrl.Name,
						ApiEnabled:           vaCtrl.Spec.Api.Enabled,
						PlaygroundEnabled:    vaCtrl.Spec.Api.Playground,
						InternalMetrics:      vaCtrl.Spec.InternalMetrics,
						ExpireMetricsSecs:    vaCtrl.Spec.ExpireMetricsSecs,
						PipelineSecretGetter: pipelineSecretGetter(r.APIReader, ctx),
					}, pipelineCR)
					if err != nil {
						return fmt.Errorf("cluster aggregator %s/%s build config failed: %w: %w", vector.Namespace, vector.Name, ErrBuildConfigFailed, err)
					}
					if err != nil {
						return err
					}

					byteConfig, err := cfg.MarshalJSON()
					if err != nil {
						return err
					}

					vaCtrl.ConfigBytes = byteConfig
					vaCtrl.Config = cfg

					configCheck := configcheck.New(
						vaCtrl.ConfigBytes,
						vaCtrl.Client,
						vaCtrl.ClientSet,
						&vaCtrl.Spec.VectorCommon,
						vaCtrl.Name,
						vaCtrl.Namespace,
						r.ConfigCheckTimeout,
						configcheck.ConfigCheckInitiatorPipieline,
						cfg.SecretAssets(),
					)

					reason, err := configCheck.Run(ctx)
					if errors.Is(err, configcheck.ErrConfigcheckSkipped) {
						return nil
					}
					if reason != "" {
						return fmt.Errorf("cluster aggregator %s/%s config check failed: %s", vector.Namespace, vector.Name, reason)
					}
					return err
				})
			}
		}

	}

	if err = eg.Wait(); err != nil {
		log.Error(err, "Configcheck error")
		var secretErr *config.SecretResolveError
		if errors.As(err, &secretErr) {
			// Symmetric with the resolveRelatedSecrets error path above (see the
			// SetRelatedSecretsHash(nil) comment there): resolveRelatedSecrets already
			// succeeded earlier this same reconcile and left a *successful*
			// RelatedSecretsHash on pipelineCR (set unconditionally before this
			// eg.Go/eg.Wait block runs). A transient GET failure here, on
			// Build*Config's own later read of that same secret, must not let
			// SetFailedStatus below persist that stale successful hash - otherwise a
			// future reconcile with the exact same (unchanged) secret data resolves to
			// the same hash, matches it, and the pipeline is skipped forever via
			// "Pipeline has no changes", with nothing left to wake it since the
			// secret's data never actually changed.
			pipelineCR.SetRelatedSecretsHash(nil)
		}
		if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, err.Error(), basePipeline); err != nil {
			return ctrl.Result{}, err
		}
		if secretErr != nil {
			return ctrl.Result{RequeueAfter: relatedSecretsResolveRetryDelay}, nil
		}
		if errors.Is(err, ErrBuildConfigFailed) {
			return ctrl.Result{}, nil
		}
		if r.EnableReconciliationInvalidPipelines {
			return ctrl.Result{RequeueAfter: r.ReconciliationInvalidPipelinesRetryDelay}, nil
		}
		return ctrl.Result{}, nil
	}

	if err = pipeline.SetSuccessStatus(ctx, r.Client, pipelineCR, basePipeline); err != nil {
		return ctrl.Result{}, err
	}

	for _, vector := range vectorAgents {
		r.VectorAgentEventCh <- event.GenericEvent{Object: vector}
	}
	for _, vector := range vectorAggregators {
		r.VectorAggregatorsEventCh <- event.GenericEvent{Object: vector}
	}
	for _, vector := range clusterVectorAggregators {
		r.ClusterVectorAggregatorsEventCh <- event.GenericEvent{Object: vector}
	}

	log.Info("finish Reconcile Pipeline")
	return ctrl.Result{}, nil
}

// resolvePipelineRole pins or infers the pipeline role. Validating here rather than in the config
// builder fails only the offending pipeline, not the aggregator's whole config.
func resolvePipelineRole(cfg *config.PipelineConfig, p pipeline.Pipeline) (*v1alpha1.VectorPipelineRole, error) {
	role, err := cfg.VectorRole(p.GetSpec().Role)
	if err != nil {
		return nil, err
	}
	if *role == v1alpha1.VectorPipelineRoleAggregator && p.GetNamespace() != "" {
		if err := cfg.ValidateAggregatorSources(); err != nil {
			return nil, err
		}
	}
	return role, nil
}

func (r *PipelineReconciler) getPipeline(ctx context.Context, req ctrl.Request) (pipeline pipeline.Pipeline, err error) {
	if req.Namespace != "" {
		vp := &v1alpha1.VectorPipeline{}
		err := r.Get(ctx, req.NamespacedName, vp)
		if err != nil {
			return nil, client.IgnoreNotFound(err)
		}
		return vp, nil
	}
	cvp := &v1alpha1.ClusterVectorPipeline{}
	err = r.Get(ctx, req.NamespacedName, cvp)
	if err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return cvp, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VectorPipeline{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 20}).
		Watches(&v1alpha1.ClusterVectorPipeline{}, &handler.EnqueueRequestForObject{}).
		WithEventFilter(specAndAnnotationsPredicate).
		// Watch on Secrets, reusing the manager's cache: the workload reconcilers already
		// run Owns(&corev1.Secret{}), a full structural informer, so this adds no second
		// watch stream - only another handler on the informer already there.
		// mapSecretToPipelines needs the Secret's identity, never its Data; payloads are
		// read separately through the uncached APIReader.
		//
		// Wired through WatchesRawSource rather than Watches because WithEventFilter above
		// applies specAndAnnotationsPredicate to every watch on this builder (ANDed with
		// any per-watch predicate), and that predicate fires only on a generation or
		// annotations change. Secrets have no status subresource, so the API server never
		// bumps metadata.generation on a data-only update - it would silently swallow every
		// rotation. ResourceVersionChangedPredicate is the correct signal here.
		WatchesRawSource(source.Kind[client.Object](
			mgr.GetCache(),
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretToPipelines),
			predicate.ResourceVersionChangedPredicate{},
		)).
		Complete(r)
}

// mapSecretToPipelines resolves a changed Secret to the pipelines that declared it via
// spec.secret, so a Secret rotation gets requeued as a normal pipeline reconcile.
// Returns nil for secrets no pipeline references, which is the overwhelming common case.
func (r *PipelineReconciler) mapSecretToPipelines(_ context.Context, obj client.Object) []reconcile.Request {
	if r.SecretIndex == nil {
		return nil
	}

	pipelines := r.SecretIndex.PipelinesFor(types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()})
	if len(pipelines) == 0 {
		return nil
	}

	requests := make([]reconcile.Request, len(pipelines))
	for i, p := range pipelines {
		requests[i] = reconcile.Request{NamespacedName: p}
	}
	return requests
}

var specAndAnnotationsPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
			return true
		}

		if !reflect.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations()) {
			return true
		}

		return false
	},
}
