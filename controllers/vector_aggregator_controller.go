package controllers

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *VectorReconciler) ensureVectorAggregator(vectorCR *vectorv1alpha1.Vector) (done bool, result ctrl.Result, err error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-aggregator", vectorCR.Name)

	log.Info("start Reconcile Vector Aggregator")

	if done, result, err = r.ensureVectorRBAC(vectorCR); done {
		return
	}

	if done, result, err = r.ensureVectorAggregatorService(vectorCR); done {
		return
	}

	if done, result, err = r.ensureVectorAggregatorSecret(vectorCR); done {
		return
	}

	if done, result, err = r.ensureVectorAggregatorStatefulSet(vectorCR); done {
		return
	}

	return
}

func (r *VectorReconciler) ensureVectorRBAC(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-aggregator-rbac", vectorCR.Name)

	log.Info("start Reconcile Vector Aggregator RBAC")

	if done, _, err := r.ensureVectorAggregatorServiceAccount(vectorCR); done {
		return ReconcileResult(err)
	}

	return ReconcileResult(nil)
}

func (r *VectorReconciler) ensureVectorAggregatorServiceAccount(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	vectorAggregatorServiceAccount := r.createVectorAggregatorServiceAccount(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAggregatorServiceAccount, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateServiceAccount(vectorAggregatorServiceAccount)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAggregatorService(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-aggregator-service", vectorCR.Name)

	log.Info("start Reconcile Vector Aggregator Service")

	vectorAggregatorService := r.createVectorAggregatorService(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAggregatorService, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateService(vectorAggregatorService)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAggregatorSecret(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-aggregator-secret", vectorCR.Name)

	log.Info("start Reconcile Vector Aggregator Secret")

	vectorAggregatorSecret := r.createVectorAggregatorSecret(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAggregatorSecret, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateSecret(vectorAggregatorSecret)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAggregatorStatefulSet(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-aggregator-statefulset", vectorCR.Name)

	log.Info("start Reconcile Vector Aggregator StatefulSet")

	vectorAggregatorStatefulSet := r.createVectorAggregatorStatefulSet(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAggregatorStatefulSet, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateStatefulSet(vectorAggregatorStatefulSet)

	return ReconcileResult(err)
}

func labelsForVectorAggregator(name string) map[string]string {
	return map[string]string{
		label.ManagedByLabelKey: "vector-operator",
		label.NameLabelKey:      "vector",
		label.ComponentLabelKey: "Aggregator",
		label.InstanceLabelKey:  name,
	}
}

func objectMetaVectorAggregator(v *vectorv1alpha1.Vector, labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      v.Name + "-aggregator",
		Namespace: v.Namespace,
		Labels:    labels,
	}
}

func getNameVectorAggregator(v *vectorv1alpha1.Vector) string {
	name := v.Name + "-aggregator"
	return name
}
