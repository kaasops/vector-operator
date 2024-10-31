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
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/config/configcheck"
	"github.com/kaasops/vector-operator/internal/pipeline"
	"github.com/kaasops/vector-operator/internal/utils/hash"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"github.com/kaasops/vector-operator/internal/vector/aggregator"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/api/v1alpha1"
)

const aggregatorFinalizerName = "vectoraggregator.observability.kaasops.io/finalizer"

// VectorAggregatorReconciler reconciles a VectorAggregator object
type VectorAggregatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Clientset          *kubernetes.Clientset
	ConfigCheckTimeout time.Duration
	EventChan          chan event.GenericEvent
}

// +kubebuilder:rbac:groups=observability.kaasops.io,resources=vectoraggregators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.kaasops.io,resources=vectoraggregators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.kaasops.io,resources=vectoraggregators/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get;list
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VectorAggregator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *VectorAggregatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Start Reconcile VectorAggregator")

	if req.Namespace == "" { // cluster resources (ClusterRole and ClusterRoleBinding) don't have ns
		vectorAggregators, err := listVectorAggregators(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to list vector aggregators instances")
			return ctrl.Result{}, err
		}
		filtered := make([]*v1alpha1.VectorAggregator, 0, len(vectorAggregators))
		for _, vector := range vectorAggregators {
			if vector.Name == req.Name {
				filtered = append(filtered, vector)
			}
		}
		return r.reconcileVectorAggregators(ctx, r.Client, r.Clientset, filtered...)
	}

	vectorCR := &v1alpha1.VectorAggregator{}
	err := r.Get(ctx, req.NamespacedName, vectorCR)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	setAggregatorTypeMetaIfNeeded(vectorCR)

	if vectorCR.IsBeingDeleted() {
		if controllerutil.ContainsFinalizer(vectorCR, aggregatorFinalizerName) {
			if err := r.deleteVectorAggregator(ctx, vectorCR); err != nil {
				if !api_errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(vectorCR, aggregatorFinalizerName)
			if err := r.Update(ctx, vectorCR); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !vectorCR.HasFinalizer(aggregatorFinalizerName) {
		controllerutil.AddFinalizer(vectorCR, aggregatorFinalizerName)
		if err := r.Update(ctx, vectorCR); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return r.createOrUpdateVectorAggregator(ctx, r.Client, r.Clientset, vectorCR)
}

// SetupWithManager sets up the controller with the Manager.
func (r *VectorAggregatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	monitoringCRD, err := k8s.ResourceExists(r.Clientset.DiscoveryClient, monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VectorAggregator{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WatchesRawSource(source.Channel(r.EventChan, &handler.EnqueueRequestForObject{})).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
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

func listVectorAggregators(ctx context.Context, client client.Client) (vectors []*v1alpha1.VectorAggregator, err error) {
	vectorList := v1alpha1.VectorAggregatorList{}
	err = client.List(ctx, &vectorList)
	if err != nil {
		return nil, err
	}
	for _, v := range vectorList.Items {
		setAggregatorTypeMetaIfNeeded(&v)
		vectors = append(vectors, &v)
	}
	return vectors, nil
}

func (r *VectorAggregatorReconciler) createOrUpdateVectorAggregator(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, v *v1alpha1.VectorAggregator) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("VectorAggregator", v.Name)
	vaCtrl := aggregator.NewController(v, client, clientset)

	pipelines, err := pipeline.GetValidPipelines(ctx, vaCtrl.Client, pipeline.FilterPipelines{
		Scope:     pipeline.NamespacedPipeline,
		Selector:  v.Spec.Selector,
		Role:      v1alpha1.VectorPipelineRoleAggregator,
		Namespace: vaCtrl.Namespace,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{
		AggregatorName:    vaCtrl.Name,
		ApiEnabled:        vaCtrl.Spec.Api.Enabled,
		PlaygroundEnabled: vaCtrl.Spec.Api.Playground,
		InternalMetrics:   vaCtrl.Spec.InternalMetrics,
	}, pipelines...)
	if err != nil {
		if err := vaCtrl.SetFailedStatus(ctx, err.Error()); err != nil {
			return ctrl.Result{}, err
		}
		log.Error(err, "Build config failed")
		return ctrl.Result{}, nil
	}

	byteCfg, err := cfg.MarshalJSON()
	if err != nil {
		return ctrl.Result{}, err
	}
	cfgHash := hash.Get(byteCfg)

	if !vaCtrl.Spec.ConfigCheck.Disabled {
		if vaCtrl.Status.LastAppliedConfigHash == nil || *vaCtrl.Status.LastAppliedConfigHash != cfgHash {
			reason, err := configcheck.New(
				byteCfg,
				vaCtrl.Client,
				vaCtrl.ClientSet,
				&vaCtrl.Spec.VectorCommon,
				vaCtrl.Name,
				vaCtrl.Namespace,
				r.ConfigCheckTimeout,
				configcheck.ConfigCheckInitiatorVector,
			).Run(ctx)
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

	vaCtrl.ConfigBytes = byteCfg
	vaCtrl.Config = cfg

	if err := vaCtrl.EnsureVectorAggregator(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := vaCtrl.SetSuccessStatus(ctx, &cfgHash); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *VectorAggregatorReconciler) reconcileVectorAggregators(ctx context.Context, c client.Client, clientset *kubernetes.Clientset, aggregators ...*v1alpha1.VectorAggregator) (ctrl.Result, error) {
	if len(aggregators) == 0 {
		return ctrl.Result{}, nil
	}

	for _, ag := range aggregators {
		if ag.DeletionTimestamp != nil {
			continue
		}
		if _, err := r.createOrUpdateVectorAggregator(ctx, c, clientset, ag); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *VectorAggregatorReconciler) deleteVectorAggregator(ctx context.Context, v *v1alpha1.VectorAggregator) error {
	return aggregator.NewController(v, r.Client, r.Clientset).DeleteVectorAggregator(ctx)
}

func setAggregatorTypeMetaIfNeeded(cr *v1alpha1.VectorAggregator) {
	// https://github.com/kubernetes/kubernetes/issues/80609
	if cr.Kind == "" || cr.APIVersion == "" {
		cr.Kind = "VectorAggregator"
		cr.APIVersion = "observability.kaasops.io/v1alpha1"
	}
}
