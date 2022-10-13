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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
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
	log := log.FromContext(ctx).WithValues("Vector", req.NamespacedName)

	log.Info("start Reconcile Vector")

	pipelineCR, done, result, err := r.findVectorPipelineCustomResourceInstance(ctx, log, req)
	if done {
		return result, err
	}

	config := r.NewVectorConfig(pipelineCR)
	yamlconf, err := r.VectorConfigToYaml(&config)
	if err != nil {
		return result, err
	}
	fmt.Println(string(yamlconf))

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.VectorPipeline{}).
		Complete(r)
}

func (r *VectorPipelineReconciler) findVectorPipelineCustomResourceInstance(ctx context.Context, log logr.Logger, req ctrl.Request) (*vectorv1alpha1.VectorPipeline, bool, ctrl.Result, error) {
	// fetch the master instance
	pipelineCR := &vectorv1alpha1.VectorPipeline{}
	err := r.Get(ctx, req.NamespacedName, pipelineCR)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Vector Pipeline CR not found. Ignoring since object must be deleted")
			return nil, true, ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get PipelineCR")
		return nil, true, ctrl.Result{}, err
	}
	log.Info("Get Vector Pipeline " + pipelineCR.Name)
	return pipelineCR, false, ctrl.Result{}, nil
}
