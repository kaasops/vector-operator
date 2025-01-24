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
	"github.com/kaasops/vector-operator/internal/config/configcheck"
	"github.com/kaasops/vector-operator/internal/vector/aggregator"
	"github.com/kaasops/vector-operator/internal/vector/vectoragent"
	"golang.org/x/sync/errgroup"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
}

var (
	ErrBuildConfigFailed = errors.New("failed to build config")
)

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines;clustervectorpipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines/status;clustervectorpipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines/finalizers;clustervectorpipelines/finalizers,verbs=update

func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Pipeline", req.Name)

	log.Info("start Reconcile Pipeline")
	pipelineCR, err := r.getPipeline(ctx, req)
	if err != nil {
		log.Error(err, "Failed to get Pipeline")
		return ctrl.Result{}, err
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
		if notChanged {
			log.Info("Pipeline has no changes. Finish Reconcile Pipeline")
			return ctrl.Result{}, nil
		}
	}

	p := &config.PipelineConfig{}
	if err := config.UnmarshalJson(pipelineCR.GetSpec(), p); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to unmarshal pipeline %s: %w", pipelineCR.GetName(), err)
	}
	pipelineVectorRole, err := p.VectorRole()
	if err != nil {
		if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, err.Error()); err != nil {
			log.Error(err, "Failed to set pipeline status")
			return ctrl.Result{}, err
		}
		log.Error(err, "Failed to determine pipeline role")
		return ctrl.Result{}, nil
	}
	pipelineCR.SetRole(pipelineVectorRole)

	eg := errgroup.Group{}

	if *pipelineVectorRole == v1alpha1.VectorPipelineRoleAgent {

		for _, vector := range vectorAgents {
			eg.Go(func() error {
				vaCtrl := vectoragent.NewController(vector, r.Client, r.Clientset)
				byteConfig, err := config.BuildAgentConfig(config.VectorConfigParams{
					ApiEnabled:        vaCtrl.Vector.Spec.Agent.Api.Enabled,
					PlaygroundEnabled: vaCtrl.Vector.Spec.Agent.Api.Playground,
					UseApiServerCache: vaCtrl.Vector.Spec.UseApiServerCache,
					InternalMetrics:   vaCtrl.Vector.Spec.Agent.InternalMetrics,
					ExpireMetricsSecs: vaCtrl.Vector.Spec.Agent.ExpireMetricsSecs,
				}, pipelineCR)
				if err != nil {
					return fmt.Errorf("agent %s/%s build config failed: %w: %w", vector.Namespace, vector.Name, ErrBuildConfigFailed, err)
				}

				vaCtrl.Config = byteConfig
				configCheck := configcheck.New(
					vaCtrl.Config,
					vaCtrl.Client,
					vaCtrl.ClientSet,
					&vaCtrl.Vector.Spec.Agent.VectorCommon,
					vaCtrl.Vector.Name,
					vaCtrl.Vector.Namespace,
					r.ConfigCheckTimeout,
					configcheck.ConfigCheckInitiatorPipieline,
				)

				reason, err := configCheck.Run(ctx)
				if reason != "" {
					return fmt.Errorf("agent %s/%s config check failed: %s", vector.Namespace, vector.Name, reason)
				}
				return err
			})
		}

	} else {

		if pipelineCR.GetNamespace() != "" {
			for _, vector := range vectorAggregators {
				eg.Go(func() error {
					vaCtrl := aggregator.NewController(vector, r.Client, r.Clientset)
					cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{
						AggregatorName:    vaCtrl.Name,
						ApiEnabled:        vaCtrl.Spec.Api.Enabled,
						PlaygroundEnabled: vaCtrl.Spec.Api.Playground,
						InternalMetrics:   vaCtrl.Spec.InternalMetrics,
						ExpireMetricsSecs: vaCtrl.Spec.ExpireMetricsSecs,
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
					)

					reason, err := configCheck.Run(ctx)
					if reason != "" {
						return fmt.Errorf("aggregator %s/%s config check failed: %s", vector.Namespace, vector.Name, reason)
					}
					return err
				})
			}

		} else {

			for _, vector := range clusterVectorAggregators {
				eg.Go(func() error {
					vaCtrl := aggregator.NewController(vector, r.Client, r.Clientset)
					cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{
						AggregatorName:    vaCtrl.Name,
						ApiEnabled:        vaCtrl.Spec.Api.Enabled,
						PlaygroundEnabled: vaCtrl.Spec.Api.Playground,
						InternalMetrics:   vaCtrl.Spec.InternalMetrics,
						ExpireMetricsSecs: vaCtrl.Spec.ExpireMetricsSecs,
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
					)

					reason, err := configCheck.Run(ctx)
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
		if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, err.Error()); err != nil {
			return ctrl.Result{}, err
		}
		if errors.Is(err, ErrBuildConfigFailed) {
			return ctrl.Result{}, nil
		}
		if r.EnableReconciliationInvalidPipelines {
			return ctrl.Result{RequeueAfter: r.ReconciliationInvalidPipelinesRetryDelay}, nil
		}
		return ctrl.Result{}, nil
	}

	if err = pipeline.SetSuccessStatus(ctx, r.Client, pipelineCR); err != nil {
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
		Complete(r)
}

var specAndAnnotationsPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldObject := e.ObjectOld.(client.Object)
		newObject := e.ObjectNew.(client.Object)

		if oldObject.GetGeneration() != newObject.GetGeneration() {
			return true
		}

		if !reflect.DeepEqual(oldObject.GetAnnotations(), newObject.GetAnnotations()) {
			return true
		}

		return false
	},
}
