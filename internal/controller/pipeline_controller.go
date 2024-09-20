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
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"time"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/config/configcheck"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/kaasops/vector-operator/internal/vector/vectoragent"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type PipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset                   *kubernetes.Clientset
	ConfigCheckTimeout          time.Duration
	VectorAgentReconciliationCh chan event.GenericEvent
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
		log.Error(err, "Failed to get Instances")
		return ctrl.Result{}, nil
	}

	if len(vectorAgents) == 0 {
		log.Info("Vectors not found")
		return ctrl.Result{}, nil
	}

	if pipelineCR == nil {
		log.Info("Pipeline CR not found. Ignoring since object must be deleted")
		for _, vector := range vectorAgents {
			r.VectorAgentReconciliationCh <- event.GenericEvent{Object: vector}
		}
		return ctrl.Result{}, nil
	}

	// Check Pipeline hash
	checkResult, err := pipeline.CheckHash(pipelineCR)
	if err != nil {
		return ctrl.Result{}, err
	}
	if checkResult {
		log.Info("Pipeline has no changes. Finish Reconcile Pipeline")
		return ctrl.Result{}, nil
	}

	eg := errgroup.Group{}

	for _, vector := range vectorAgents {
		eg.Go(func() error {
			vaCtrl := vectoragent.NewController(vector, r.Client, r.Clientset)
			byteConfig, err := config.BuildByteConfig(vaCtrl, pipelineCR)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrBuildConfigFailed, err)
			}

			vaCtrl.Config = byteConfig
			configCheck := configcheck.New(
				vaCtrl.Config,
				vaCtrl.Client,
				vaCtrl.ClientSet,
				vaCtrl.Vector,
				r.ConfigCheckTimeout,
				configcheck.ConfigCheckInitiatorPipieline,
			)

			reason, err := configCheck.Run(ctx)
			if reason != "" {
				return errors.New(reason)
			}
			return err
		})
	}

	if err = eg.Wait(); err != nil {
		log.Error(err, "Configcheck error")
		if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, err.Error()); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, IgnoreBuildConfigFailed(err)
	}

	if err = pipeline.SetSuccessStatus(ctx, r.Client, pipelineCR); err != nil {
		return ctrl.Result{}, err
	}

	for _, vector := range vectorAgents {
		r.VectorAgentReconciliationCh <- event.GenericEvent{Object: vector}
	}

	log.Info("finish Reconcile Pipeline")
	return ctrl.Result{}, nil
}

func (r *PipelineReconciler) getPipeline(ctx context.Context, req ctrl.Request) (pipeline pipeline.Pipeline, err error) {
	if req.Namespace != "" {
		vp := &vectorv1alpha1.VectorPipeline{}
		err := r.Get(ctx, req.NamespacedName, vp)
		if err != nil {
			return nil, client.IgnoreNotFound(err)
		}
		return vp, nil
	}
	cvp := &vectorv1alpha1.ClusterVectorPipeline{}
	err = r.Get(ctx, req.NamespacedName, cvp)
	if err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return cvp, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.VectorPipeline{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 20}).
		Watches(&vectorv1alpha1.ClusterVectorPipeline{}, &handler.EnqueueRequestForObject{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func IgnoreBuildConfigFailed(err error) error {
	if errors.Is(err, ErrBuildConfigFailed) {
		return nil
	}
	return err
}
