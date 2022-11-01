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

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline/clustervectorpipeline"
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
	if vectorPipelineCR == nil {
		log.Info("VectorPIpeline CR not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}

	// Generate VectorPipeline Controller
	vpCtrl := clustervectorpipeline.NewController(vectorPipelineCR)
	// Generate Pipeline Controller
	pCtrl := pipeline.NewController(ctx, r.Client, vpCtrl)

	// Check Pipeline hash
	checkResult, err := pCtrl.CheckHash()
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

		// Get Vector Config file
		config, err := config.New(ctx, vaCtrl)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := config.FillForVectorPipeline(pCtrl); err != nil {
			return ctrl.Result{}, err
		}

		// Get Config in Json ([]byte)
		byteConfig, err := config.GetByteConfig()
		if err != nil {
			return ctrl.Result{}, err
		}

		// Init CheckConfig
		configCheck := configcheck.New(ctx, byteConfig, vaCtrl.Client, vaCtrl.ClientSet, vaCtrl.Vector.Name, vaCtrl.Vector.Namespace, vaCtrl.Vector.Spec.Agent.Image)

		// Start ConfigCheck
		err = configCheck.Run()
		if _, ok := err.(*configcheck.ConfigCheckError); ok {
			if err := pCtrl.SetFailedStatus(err); err != nil {
				return ctrl.Result{}, err
			}
			if err = pCtrl.SetLastAppliedPipelineStatus(); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err != nil {
			return ctrl.Result{}, err
		}

		if err = pCtrl.SetSucceesStatus(); err != nil {
			return ctrl.Result{}, err
		}

		if err = pCtrl.SetLastAppliedPipelineStatus(); err != nil {
			return ctrl.Result{}, err
		}

	}

	log.Info("finish Reconcile VectorPipeline")
	return ctrl.Result{}, nil
}

func (r *ClusterVectorPipelineReconciler) findClusterVectorPipelineCustomResourceInstance(ctx context.Context, req ctrl.Request) (*vectorv1alpha1.ClusterVectorPipeline, error) {
	// fetch the master instance
	cvp := &vectorv1alpha1.ClusterVectorPipeline{}
	err := r.Get(ctx, req.NamespacedName, cvp)
	if err != nil {
		if errors.IsNotFound(err) {
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
