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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/vectorpipeline"
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
	_ = log.FromContext(ctx)
	log := log.FromContext(ctx).WithValues("VectorPipeline", req.NamespacedName)

	log.Info("start Reconcile VectorPipeline")

	vp, done, result, err := r.findVectorPipelineCustomResourceInstance(ctx, log, req)
	if done {
		return result, err
	}

	vectorInstances := &vectorv1alpha1.VectorList{}
	err = r.List(ctx, vectorInstances)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(vectorInstances.Items) == 0 {
		log.Info("Vertors not found")
		return ctrl.Result{}, nil
	}

	for _, v := range vectorInstances.Items {
		if v.DeletionTimestamp != nil {
			continue
		}

		err = checkConfig(ctx, &v, vp, r.Client, r.Clientset)
		if err != nil {
			return ctrl.Result{}, err
		}

	}

	log.Info("finish Reconcile VectorPipeline")
	return ctrl.Result{}, nil
}

func (r *VectorPipelineReconciler) findVectorPipelineCustomResourceInstance(ctx context.Context, log logr.Logger, req ctrl.Request) (*vectorv1alpha1.VectorPipeline, bool, ctrl.Result, error) {
	// fetch the master instance
	vp := &vectorv1alpha1.VectorPipeline{}
	err := r.Get(ctx, req.NamespacedName, vp)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("VectorPipeline CR not found. Ignoring since object must be deleted")
			return nil, true, ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Vector")
		return nil, true, ctrl.Result{}, err
	}
	log.Info("Get Vector Pipeline" + vp.Name)
	return vp, false, ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.VectorPipeline{}).
		Complete(r)
}

func checkConfig(ctx context.Context, v *vectorv1alpha1.Vector, vp *vectorv1alpha1.VectorPipeline, c client.Client, cs *kubernetes.Clientset) error {
	log := log.FromContext(context.TODO()).WithValues("Vector Pipeline", "ConfigCheck")

	var vps []*vectorv1alpha1.VectorPipeline
	vps = append(vps, vp)

	cfg, err := config.GenerateConfig(v, vps)
	if err != nil {
		return err
	}

	err = configcheck.Run(cfg, c, cs, vp.Name, vp.Namespace, v.Spec.Agent.Image)
	if _, ok := err.(*configcheck.ErrConfigCheck); ok {
		if err := vectorpipeline.SetFailedStatus(ctx, vp, c, err); err != nil {
			return err
		}
		log.Error(err, "Vector Config has error")
		return nil
	}
	if err != nil {
		return err
	}

	if err := vectorpipeline.SetSucceesStatus(ctx, vp, c); err != nil {
		return err
	}

	return nil
}
