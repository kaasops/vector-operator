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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	v1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// ClusterVectorAggregatorReconciler reconciles a ClusterVectorAggregator object
type ClusterVectorAggregatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Clientset          *kubernetes.Clientset
	ConfigCheckTimeout time.Duration
	EventChan          chan event.GenericEvent
}

// +kubebuilder:rbac:groups=observability.kaasops.io,resources=clustervectoraggregators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.kaasops.io,resources=clustervectoraggregators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.kaasops.io,resources=clustervectoraggregators/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
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
// the ClusterVectorAggregator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ClusterVectorAggregatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	clusterAggregator := &v1alpha1.ClusterVectorAggregator{}
	err := r.Get(ctx, req.NamespacedName, clusterAggregator)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	setClusterAggregatorTypeMetaIfNeeded(clusterAggregator)

	return r.createOrUpdateVectorAggregator(ctx, r.Client, r.Clientset, clusterAggregator)
}

func (r *ClusterVectorAggregatorReconciler) createOrUpdateVectorAggregator(ctx context.Context, client client.Client, clientset *kubernetes.Clientset, v *v1alpha1.ClusterVectorAggregator) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("VectorAggregator", v.Name)
	vaCtrl := aggregator.NewController(v, client, clientset)
	if vaCtrl.Namespace == "" {
		if err := vaCtrl.SetFailedStatus(ctx, "spec.resourceNamespace is empty"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	pipelines, err := pipeline.GetValidPipelines(ctx, vaCtrl.Client, pipeline.FilterPipelines{
		Scope:    pipeline.ClusterPipelines,
		Selector: v.Spec.Selector,
		Role:     v1alpha1.VectorPipelineRoleAggregator,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	cfg, err := config.BuildAggregatorConfig(config.VectorConfigParams{
		AggregatorName:    vaCtrl.Name,
		ApiEnabled:        vaCtrl.Spec.Api.Enabled,
		PlaygroundEnabled: vaCtrl.Spec.Api.Playground,
		InternalMetrics:   vaCtrl.Spec.InternalMetrics,
		ExpireMetricsSecs: vaCtrl.Spec.ExpireMetricsSecs,
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

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterVectorAggregatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	monitoringCRD, err := k8s.ResourceExists(r.Clientset.DiscoveryClient, monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ClusterVectorAggregator{}).
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

func listClusterVectorAggregators(ctx context.Context, client client.Client) (vectors []*v1alpha1.ClusterVectorAggregator, err error) {
	vectorList := v1alpha1.ClusterVectorAggregatorList{}
	err = client.List(ctx, &vectorList)
	if err != nil {
		return nil, err
	}
	for _, v := range vectorList.Items {
		setClusterAggregatorTypeMetaIfNeeded(&v)
		vectors = append(vectors, &v)
	}
	return vectors, nil
}

func setClusterAggregatorTypeMetaIfNeeded(cr *v1alpha1.ClusterVectorAggregator) {
	// https://github.com/kubernetes/kubernetes/issues/80609
	if cr.Kind == "" || cr.APIVersion == "" {
		cr.Kind = "ClusterVectorAggregator"
		cr.APIVersion = "observability.kaasops.io/v1alpha1"
	}
}
