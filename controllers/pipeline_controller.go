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

package controllers

import (
	"context"
	"errors"
	"sync"
	"time"

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"
)

type PipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset                  *kubernetes.Clientset
	PipelineCheckWG            *sync.WaitGroup
	PipelineDeleteEventTimeout time.Duration
}

var VectorAgentReconciliationSourceChannel = make(chan event.GenericEvent)

const PipelineDeleteEventTimeout time.Duration = 3 * time.Second

func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Pipeline", req.Name)

	log.Info("start Reconcile Pipeline")
	pipelineCR, err := r.findPipelineCustomResourceInstance(ctx, req)
	if err != nil {
		log.Error(err, "Failed to get Pipeline")
		return ctrl.Result{}, err
	}
	vectorInstances, err := listVectorCustomResourceInstances(ctx, r.Client)

	if err != nil {
		log.Error(err, "Failed to get Instances")
		return ctrl.Result{}, nil
	}

	if len(vectorInstances) == 0 {
		log.Info("Vertors not found")
		return ctrl.Result{}, nil
	}

	if pipelineCR == nil {
		log.Info("Pipeline CR not found. Ignoring since object must be deleted")
		for _, vector := range vectorInstances {
			r.PipelineCheckWG.Add(1)
			go func() {
				VectorAgentReconciliationSourceChannel <- event.GenericEvent{Object: vector}
				log.Info("Waiting if other deletions will be done during timeout")
				time.Sleep(r.PipelineDeleteEventTimeout)
				r.PipelineCheckWG.Done()
			}()
			return ctrl.Result{}, nil
		}
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

	for _, vector := range vectorInstances {
		if vector.DeletionTimestamp != nil {
			continue
		}

		// Init Controller for Vector Agent
		vaCtrl := vectoragent.NewController(vector, r.Client, r.Clientset)

		vaCtrl.SetDefault()
		// Get Vector Config file
		configBuilder := config.NewBuilder(vaCtrl, pipelineCR)

		byteConfig, err := configBuilder.GetByteConfigWithValidate()
		if err != nil {
			if err := pipeline.SetFailedStatus(ctx, r.Client, pipelineCR, err); err != nil {
				return ctrl.Result{}, err
			}
			if err = pipeline.SetLastAppliedPipelineStatus(ctx, r.Client, pipelineCR); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		vaCtrl.Config = byteConfig
		r.PipelineCheckWG.Add(1)
		go r.runPipelineCheck(ctx, pipelineCR, vaCtrl)
	}

	log.Info("finish Reconcile Pipeline")
	return ctrl.Result{}, nil
}

func (r *PipelineReconciler) findPipelineCustomResourceInstance(ctx context.Context, req ctrl.Request) (pipeline pipeline.Pipeline, err error) {
	if req.Namespace != "" {
		vp := &vectorv1alpha1.VectorPipeline{}
		err := r.Get(ctx, req.NamespacedName, vp)
		if err != nil {
			if api_errors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		return vp, nil
	} else {
		cvp := &vectorv1alpha1.ClusterVectorPipeline{}
		err := r.Get(ctx, req.NamespacedName, cvp)
		if err != nil {
			if api_errors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		return cvp, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.VectorPipeline{}).
		Watches(&source.Kind{Type: &vectorv1alpha1.ClusterVectorPipeline{}}, &handler.EnqueueRequestForObject{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (r *PipelineReconciler) runPipelineCheck(ctx context.Context, p pipeline.Pipeline, vaCtrl *vectoragent.Controller) {
	log := log.FromContext(ctx).WithValues("Pipeline", p.GetName())
	// Init CheckConfig
	configCheck := configcheck.New(
		vaCtrl.Config,
		vaCtrl.Client,
		vaCtrl.ClientSet,
		vaCtrl.Vector.Name,
		vaCtrl.Vector.Namespace,
		vaCtrl.Vector.Spec.Agent.Image,
		vaCtrl.Vector.Spec.Agent.Env,
		vaCtrl.Vector.Spec.Agent.Tolerations,
	)
	configCheck.Initiator = configcheck.ConfigCheckInitiatorPipieline
	defer r.PipelineCheckWG.Done()
	// Start ConfigCheck
	err := configCheck.Run(ctx)
	if errors.Is(err, configcheck.ValidationError) {
		if err = pipeline.SetFailedStatus(ctx, r.Client, p, err); err != nil {
			log.Error(err, "Failed to set pipeline status")
			return
		}
		if err = pipeline.SetLastAppliedPipelineStatus(ctx, r.Client, p); err != nil {
			log.Error(err, "Failed to set pipeline status")
			return
		}
		return
	}
	if err != nil {
		log.Error(err, "Configcheck error")
		return
	}

	if err = pipeline.SetSuccessStatus(ctx, r.Client, p); err != nil {
		log.Error(err, "Failed to set pipeline status")
		return
	}

	if err = pipeline.SetLastAppliedPipelineStatus(ctx, r.Client, p); err != nil {
		log.Error(err, "Failed to set pipeline status")
		return
	}
	// Start vector reconcilation
	if *p.GetConfigCheckResult() {
		VectorAgentReconciliationSourceChannel <- event.GenericEvent{Object: vaCtrl.Vector}
	}
}
