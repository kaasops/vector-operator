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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// VectorPipelineReconciler reconciles a VectorPipeline object
type VectorPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
	_ = log.FromContext(ctx)
	log := log.FromContext(ctx).WithValues("VectorPipeline", req.NamespacedName)

	log.Info("start Reconcile VectorPipeline")

	vectorInstances := &vectorv1alpha1.VectorList{}
	err := r.List(ctx, vectorInstances)
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
		currentVector := &vector
		CreateOrUpdateVector(ctx, currentVector, r.Client)

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.VectorPipeline{}).
		Complete(r)
}
