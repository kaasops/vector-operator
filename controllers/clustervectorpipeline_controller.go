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

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"
)

// ClusterVectorPipelineReconciler reconciles a ClusterVectorPipeline object
type ClusterVectorPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset *kubernetes.Clientset
}

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=clustervectorpipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=clustervectorpipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=clustervectorpipelines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ClusterVectorPipeline object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *ClusterVectorPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("ClusterVectorPipeline", req.Name)

	log.Info("start Reconcile VectorPipeline")

	// Get CR VectorPipeline
	vectorPipelineCR, err := r.findClusterVectorPipelineCustomResourceInstance(ctx, req)
	if err != nil {
		log.Error(err, "Failed to get Vector Pipeline")
		return ctrl.Result{}, err
	}

	vectorInstances, err := listVectorCustomResourceInstances(ctx, r.Client)

	if err != nil {
		log.Error(err, "Failed to get Vector Instances")
		return ctrl.Result{}, nil
	}

	if len(vectorInstances) == 0 {
		log.Info("Vertors not found")
		return ctrl.Result{}, nil
	}

	if vectorPipelineCR == nil || vectorPipelineCR.DeletionTimestamp != nil {
		log.Info("ClusterVectorPIpeline CR not found. Ignoring since object must be deleted")
		for _, vector := range vectorInstances {
			ReconciliationSourceChannel <- event.GenericEvent{Object: vector}
			return ctrl.Result{}, nil
		}
	}

	// Check Pipeline hash
	checkResult, err := pipeline.CheckHash(vectorPipelineCR)
	if err != nil {
		return ctrl.Result{}, err
	}
	if checkResult {
		log.Info("ClusterVectorPipeline has no changes. Finish Reconcile VectorPipeline")
		return ctrl.Result{}, nil
	}

	for _, vector := range vectorInstances {
		if vector.DeletionTimestamp != nil {
			continue
		}

		// Init Controller for Vector Agent
		vaCtrl := vectoragent.NewController(vector, r.Client, r.Clientset)
		if vaCtrl.Vector.Spec.Agent.DataDir == "" {
			vaCtrl.Vector.Spec.Agent.DataDir = "/vector-data-dir"
		}

		if err := config.ReconcileConfig(ctx, r.Client, vectorPipelineCR, vaCtrl); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("finish Reconcile ClusterVectorPipeline")
	return reconcileVectors(ctx, r.Client, r.Clientset, true, vectorInstances...)
}

func (r *ClusterVectorPipelineReconciler) findClusterVectorPipelineCustomResourceInstance(ctx context.Context, req ctrl.Request) (*vectorv1alpha1.ClusterVectorPipeline, error) {
	// fetch the master instance
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

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterVectorPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.ClusterVectorPipeline{}).
		Complete(r)
}
