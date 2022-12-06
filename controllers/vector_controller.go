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

	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/config/configcheck"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/utils/hash"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	rbacv1 "k8s.io/api/rbac/v1"

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

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

var VectorAgentReconciliationSourceChannel = make(chan event.GenericEvent)

func (r *VectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log := log.FromContext(ctx).WithValues("Vector", req.NamespacedName)
	log.Info("start Reconcile Vector")

	if req.Namespace == "" {
		vectors, err := listVectorCustomResourceInstances(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to list vector instances")
			return ctrl.Result{}, err
		}
		return reconcileVectors(ctx, r.Client, r.Clientset, false, vectors...)
	}

	vectorCR, err := r.findVectorCustomResourceInstance(ctx, req)
	if err != nil {
		log.Error(err, "Failed to get Vector")
		return ctrl.Result{}, err
	}
	if vectorCR == nil {
		log.Info("Vector CR not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}

	return createOrUpdateVector(ctx, r.Client, r.Clientset, vectorCR, false)
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.Vector{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Channel{Source: VectorAgentReconciliationSourceChannel}, &handler.EnqueueRequestForObject{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Complete(r)
}

func listVectorCustomResourceInstances(ctx context.Context, client client.Client) (vectors []*vectorv1alpha1.Vector, err error) {
	vectorlist := vectorv1alpha1.VectorList{}
	err = client.List(ctx, &vectorlist)
	if err != nil {
		return nil, err
	}
	for _, vector := range vectorlist.Items {
		vectors = append(vectors, &vector)
	}
	return vectors, nil
}

func (r *VectorReconciler) findVectorCustomResourceInstance(ctx context.Context, req ctrl.Request) (*vectorv1alpha1.Vector, error) {
	// fetch the master instance
	vectorCR := &vectorv1alpha1.Vector{}
	err := r.Get(ctx, req.NamespacedName, vectorCR)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return vectorCR, nil
}

func reconcileVectors(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, configOnly bool, vectors ...*vectorv1alpha1.Vector) (ctrl.Result, error) {
	if len(vectors) == 0 {
		return ctrl.Result{}, nil
	}

	for _, vector := range vectors {
		if vector.DeletionTimestamp != nil {
			continue
		}
		if _, err := createOrUpdateVector(ctx, client, clientset, vector, configOnly); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func createOrUpdateVector(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, v *vectorv1alpha1.Vector, configOnly bool) (ctrl.Result, error) {
	// Init Controller for Vector Agent
	vaCtrl := vectoragent.NewController(v, client, clientset)

	vaCtrl.SetDefault()

	// Get Vector Config file
	pipelines, err := pipeline.GetValidPipelines(ctx, vaCtrl.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	configBuilder := config.NewBuilder(vaCtrl, pipelines...)

	// Get Config in Json ([]byte)
	byteConfig, err := configBuilder.GetByteConfig()
	if err != nil {
		return ctrl.Result{}, err
	}
	cfgHash := hash.Get(byteConfig)

	if vaCtrl.Vector.Status.LastAppliedConfigHash == nil || *vaCtrl.Vector.Status.LastAppliedConfigHash != cfgHash {
		configCheck := configcheck.New(
			byteConfig,
			vaCtrl.Client,
			vaCtrl.ClientSet,
			vaCtrl.Vector.Name,
			vaCtrl.Vector.Namespace,
			vaCtrl.Vector.Spec.Agent.Image,
			vaCtrl.Vector.Spec.Agent.Env,
		)
		err := configCheck.Run(ctx)
		if errors.Is(err, configcheck.ValidationError) {
			if err := vaCtrl.SetFailedStatus(ctx, err); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	vaCtrl.Config = byteConfig

	// Start Reconcile Vector Agent
	if err := vaCtrl.EnsureVectorAgent(ctx, configOnly); err != nil {
		return ctrl.Result{}, err
	}

	if err := vaCtrl.SetLastAppliedPipelineStatus(ctx, &cfgHash); err != nil {
		//TODO: Handle err: Operation cannot be fulfilled on vectors.observability.kaasops.io \"vector-sample\": the object has been modified; please apply your changes to the latest version and try again
		if api_errors.IsConflict(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	if err := vaCtrl.SetSucceesStatus(ctx); err != nil {
		// TODO: Handle err: Operation cannot be fulfilled on vectors.observability.kaasops.io \"vector-sample\": the object has been modified; please apply your changes to the latest version and try again
		if api_errors.IsConflict(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
