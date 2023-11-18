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
	"sync"
	"time"

	"github.com/kaasops/vector-operator/controllers/factory/config"
	"github.com/kaasops/vector-operator/controllers/factory/pipeline"
	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	"github.com/kaasops/vector-operator/controllers/factory/vector/vectoragent"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	rbacv1 "k8s.io/api/rbac/v1"

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

// VectorReconciler reconciles a Vector object
type VectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset            *kubernetes.Clientset
	PipelineCheckWG      *sync.WaitGroup
	PipelineCheckTimeout time.Duration
	ConfigCheckTimeout   time.Duration
	DiscoveryClient      *discovery.DiscoveryClient
}

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=sercrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;patch;delete

func (r *VectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log := log.FromContext(ctx).WithValues("Vector", req.NamespacedName)

	// Wait if PipelineCheck works right now
	if waitPipelineChecks(r.PipelineCheckWG, r.PipelineCheckTimeout) {
		log.Info("Timeout waiting pipeline checks, continue reconcile vector")
	}

	log.Info("Start Reconcile Vector")
	if req.Namespace == "" {
		// TODO: Why we reconcile another vectors?
		vectors, err := listVectorCustomResourceInstances(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to list vector instances")
			return ctrl.Result{}, err
		}
		return r.reconcileVectors(ctx, r.Client, r.Clientset, false, vectors...)
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

	return r.createOrUpdateVector(ctx, r.Client, r.Clientset, vectorCR, false)
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	monitoringCRD, err := k8s.ResourceExists(r.DiscoveryClient, monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&vectorv1alpha1.Vector{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Channel{Source: VectorAgentReconciliationSourceChannel}, &handler.EnqueueRequestForObject{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{})

	if monitoringCRD {
		builder.Owns(&monitorv1.PodMonitor{})
	}

	if err = builder.Complete(r); err != nil {
		return err
	}
	return nil
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

func (r *VectorReconciler) reconcileVectors(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, configOnly bool, vectors ...*vectorv1alpha1.Vector) (ctrl.Result, error) {
	if len(vectors) == 0 {
		return ctrl.Result{}, nil
	}

	for _, vector := range vectors {
		if vector.DeletionTimestamp != nil {
			continue
		}
		if _, err := r.createOrUpdateVector(ctx, client, clientset, vector, configOnly); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *VectorReconciler) createOrUpdateVector(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, v *vectorv1alpha1.Vector, configOnly bool) (ctrl.Result, error) {
	// Get Vector Config file
	pipelines, err := pipeline.GetValidPipelines(ctx, client)
	if err != nil {
		return ctrl.Result{}, err
	}
	configBuilder := config.NewBuilder(v, pipelines...)

	// Get Config in Json ([]byte)
	byteConfigs, err := configBuilder.GetByteConfigs()
	if err != nil {
		return ctrl.Result{}, err
	}

	for i, byteConfig := range byteConfigs {
		// Init Controller for Vector Agent
		vaCtrl := vectoragent.NewController(i, v, client, clientset, byteConfig)
		vaCtrl.SetDefault()

		// Start Reconcile Vector Agent
		if err := vaCtrl.EnsureVectorAgent(ctx, configOnly); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func waitPipelineChecks(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false
	case <-time.After(timeout):
		return true
	}
}
