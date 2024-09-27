/*
Copyright 2024.

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

package controller

import (
	"context"
	"errors"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/config/configcheck"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/kaasops/vector-operator/internal/utils/hash"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"github.com/kaasops/vector-operator/internal/vector/vectoragent"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// VectorReconciler reconciles a Vector object
type VectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset          *kubernetes.Clientset
	ConfigCheckTimeout time.Duration
	DiscoveryClient    *discovery.DiscoveryClient
	EventChan          chan event.GenericEvent
}

//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.kaasops.io,resources=vectors/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get;list
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;patch;delete

func (r *VectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Vector", req.NamespacedName)
	log.Info("Start Reconcile Vector")
	if req.Namespace == "" {
		vectors, err := listVectorAgents(ctx, r.Client)
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
		For(&v1alpha1.Vector{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WatchesRawSource(source.Channel(r.EventChan, &handler.EnqueueRequestForObject{})).
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

func listVectorAgents(ctx context.Context, client client.Client) (vectors []*v1alpha1.Vector, err error) {
	vectorList := v1alpha1.VectorList{}
	err = client.List(ctx, &vectorList)
	if err != nil {
		return nil, err
	}
	for _, vector := range vectorList.Items {
		if vector.DeletionTimestamp != nil {
			continue
		}
		vectors = append(vectors, &vector)
	}
	return vectors, nil
}

func (r *VectorReconciler) findVectorCustomResourceInstance(ctx context.Context, req ctrl.Request) (*v1alpha1.Vector, error) {
	// fetch the master instance
	vectorCR := &v1alpha1.Vector{}
	err := r.Get(ctx, req.NamespacedName, vectorCR)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	setAgentTypeMetaIfNeeded(vectorCR)
	return vectorCR, nil
}

func (r *VectorReconciler) reconcileVectors(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, configOnly bool, vectors ...*v1alpha1.Vector) (ctrl.Result, error) {
	if len(vectors) == 0 {
		return ctrl.Result{}, nil
	}

	for _, vector := range vectors {
		if vector.DeletionTimestamp != nil {
			continue
		}
		setAgentTypeMetaIfNeeded(vector)
		if _, err := r.createOrUpdateVector(ctx, client, clientset, vector, configOnly); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *VectorReconciler) createOrUpdateVector(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, v *v1alpha1.Vector, configOnly bool) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Vector", v.Name)
	// Init Controller for Vector Agent
	vaCtrl := vectoragent.NewController(v, client, clientset)

	// Get Vector Config file
	pipelines, err := pipeline.GetValidPipelines(ctx, vaCtrl.Client, pipeline.FilterPipelines{
		Scope:    pipeline.AllPipelines,
		Selector: vaCtrl.Vector.Spec.Selector,
		Role:     v1alpha1.VectorPipelineRoleAgent,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get Config in Json ([]byte)
	byteConfig, err := config.BuildAgentConfig(config.VectorConfigParams{
		ApiEnabled:        vaCtrl.Vector.Spec.Agent.Api.Enabled,
		PlaygroundEnabled: vaCtrl.Vector.Spec.Agent.Api.Playground,
		UseApiServerCache: vaCtrl.Vector.Spec.UseApiServerCache,
		InternalMetrics:   vaCtrl.Vector.Spec.Agent.InternalMetrics,
	}, pipelines...)
	if err != nil {
		if err := vaCtrl.SetFailedStatus(ctx, err.Error()); err != nil {
			return ctrl.Result{}, err
		}
		log.Error(err, "Build config failed")
		return ctrl.Result{}, nil
	}
	cfgHash := hash.Get(byteConfig)

	if !vaCtrl.Vector.Spec.Agent.ConfigCheck.Disabled {
		if vaCtrl.Vector.Status.LastAppliedConfigHash == nil || *vaCtrl.Vector.Status.LastAppliedConfigHash != cfgHash {
			configCheck := configcheck.New(
				byteConfig,
				vaCtrl.Client,
				vaCtrl.ClientSet,
				&vaCtrl.Vector.Spec.Agent.VectorCommon,
				vaCtrl.Vector.Name,
				vaCtrl.Vector.Namespace,
				r.ConfigCheckTimeout,
				configcheck.ConfigCheckInitiatorVector,
			)
			reason, err := configCheck.Run(ctx)
			if err != nil {
				if errors.Is(err, configcheck.ValidationError) {
					if err := vaCtrl.SetFailedStatus(ctx, reason); err != nil {
						return ctrl.Result{}, err
					}
					log.Error(err, "Invalid config")
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}
		}
	}

	vaCtrl.Config = byteConfig

	// Start Reconcile Vector Agent
	if err := vaCtrl.EnsureVectorAgent(ctx, configOnly); err != nil {
		return ctrl.Result{}, err
	}

	if err := vaCtrl.SetSuccessStatus(ctx, &cfgHash); err != nil {
		// TODO: Handle err: Operation cannot be fulfilled on vectors.observability.kaasops.io \"vector-sample\": the object has been modified; please apply your changes to the latest version and try again
		if api_errors.IsConflict(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func setAgentTypeMetaIfNeeded(cr *v1alpha1.Vector) {
	// https://github.com/kubernetes/kubernetes/issues/80609
	if cr.Kind == "" || cr.APIVersion == "" {
		cr.Kind = "Vector"
		cr.APIVersion = "observability.kaasops.io/v1alpha1"
	}
}
