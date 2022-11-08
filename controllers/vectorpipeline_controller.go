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
	"sigs.k8s.io/controller-runtime/pkg/log"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"
)

// VectorPipelineReconciler reconciles a VectorPipeline object
type VectorPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset *kubernetes.Clientset
}

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectorpipelines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VectorPipeline object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *VectorPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("VectorPipeline", req.Name)

	log.Info("start Reconcile VectorPipeline")

	// Get CR VectorPipeline
	vectorPipelineCR, err := r.findVectorPipelineCustomResourceInstance(ctx, req)
	if err != nil {
		log.Error(err, "Failed to get Vector Pipeline")
		return ctrl.Result{}, err
	}
	if vectorPipelineCR == nil {
		log.Info("VectorPIpeline CR not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}

	// Check Pipeline hash
	checkResult, err := pipeline.CheckHash(vectorPipelineCR)
	if err != nil {
		return ctrl.Result{}, err
	}
	if checkResult {
		log.Info("VectorPipeline has no changes. Finish Reconcile VectorPipeline")
		return ctrl.Result{}, nil
	}

	// Generate Pipeline ConfigCheck for all Vectors
	vectorInstances := &vectorv1alpha1.VectorList{}
	err = r.List(ctx, vectorInstances)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(vectorInstances.Items) == 0 {
		log.Info("Vertors not found")
		return ctrl.Result{}, nil
	}

	for _, vector := range vectorInstances.Items {
		if vector.DeletionTimestamp != nil {
			continue
		}

		// Init Controller for Vector Agent
		vaCtrl := vectoragent.NewController(&vector, r.Client, r.Clientset)
		if vaCtrl.Vector.Spec.Agent.DataDir == "" {
			vaCtrl.Vector.Spec.Agent.DataDir = "/vector-data-dir"
		}

		if err := config.ReconcileConfig(ctx, r.Client, vectorPipelineCR, vaCtrl); err != nil {
			return ctrl.Result{}, err
		}

	}

	log.Info("finish Reconcile VectorPipeline")
	return ctrl.Result{}, nil
}

func (r *VectorPipelineReconciler) findVectorPipelineCustomResourceInstance(ctx context.Context, req ctrl.Request) (*vectorv1alpha1.VectorPipeline, error) {
	// fetch the master instance
	vectorPipelineCR := &vectorv1alpha1.VectorPipeline{}
	err := r.Get(ctx, req.NamespacedName, vectorPipelineCR)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return vectorPipelineCR, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.VectorPipeline{}).
		Complete(r)
}
