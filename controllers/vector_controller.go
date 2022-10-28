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
	"time"

	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// VectorReconciler reconciles a Vector object
type VectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset *kubernetes.Clientset
}

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Vector object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *VectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log := log.FromContext(ctx).WithValues("Vector", req.NamespacedName)

	log.Info("start Reconcile Vector")

	v, done, result, err := r.findVectorCustomResourceInstance(ctx, log, req)
	if done {
		return result, err
	}

	if v.Spec.Agent.DataDir == "" {
		v.Spec.Agent.DataDir = "/vector-data-dir"
	}

	return r.CreateOrUpdateVector(ctx, v)
}

func (r *VectorReconciler) findVectorCustomResourceInstance(ctx context.Context, log logr.Logger, req ctrl.Request) (*vectorv1alpha1.Vector, bool, ctrl.Result, error) {
	// fetch the master instance
	v := &vectorv1alpha1.Vector{}
	err := r.Get(ctx, req.NamespacedName, v)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Vector CR not found. Ignoring since object must be deleted")
			return nil, true, ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Vector")
		return nil, true, ctrl.Result{}, err
	}

	return v, false, ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.Vector{}).
		Complete(r)
}

func (r *VectorReconciler) CreateOrUpdateVector(ctx context.Context, v *vectorv1alpha1.Vector) (ctrl.Result, error) {
	// Init Controller for Vector Agent
	vaCtrl := vectoragent.NewController(v, r.Client, r.Clientset)

	// Get Vector Config file
	config, err := config.New(ctx, vaCtrl)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := config.FillForVectorAgent(); err != nil {
		return ctrl.Result{}, err
	}

	// Get Config in Json ([]byte)
	byteCongif, err := config.GetByteConfig()
	if err != nil {
		return ctrl.Result{}, err
	}
	vaCtrl.Config = byteCongif

	// Start Reconcile Vector Agent
	if done, result, err := vaCtrl.EnsureVectorAgent(); done {
		return result, err
	}

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}
