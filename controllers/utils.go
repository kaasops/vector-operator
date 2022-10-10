package controllers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *VectorReconciler) CreateOrUpdateService(svc *corev1.Service) (*reconcile.Result, error) {
	return r.reconcileService(svc)
}

func (r *VectorReconciler) CreateOrUpdateSecret(secret *corev1.Secret) (*reconcile.Result, error) {
	return r.reconcileSecret(secret)
}

func (r *VectorReconciler) CreateOrUpdateDaemonSet(daemonSet *appsv1.DaemonSet) (*reconcile.Result, error) {
	return r.reconcileDaemonSet(daemonSet)
}

func (r *VectorReconciler) CreateOrUpdateStatefulSet(statefulSet *appsv1.StatefulSet) (*reconcile.Result, error) {
	return r.reconcileStatefulSet(statefulSet)
}

func (r *VectorReconciler) CreateOrUpdateServiceAccount(secret *corev1.ServiceAccount) (*reconcile.Result, error) {
	return r.reconcileServiceAccount(secret)
}

func (r *VectorReconciler) CreateOrUpdateClusterRole(secret *rbacv1.ClusterRole) (*reconcile.Result, error) {
	return r.reconcileClusterRole(secret)
}

func (r *VectorReconciler) CreateOrUpdateClusterRoleBinding(secret *rbacv1.ClusterRoleBinding) (*reconcile.Result, error) {
	return r.reconcileClusterRoleBinding(secret)
}

func (r *VectorReconciler) reconcileService(obj runtime.Object) (*reconcile.Result, error) {

	existing := &corev1.Service{}
	desired := obj.(*corev1.Service)

	err := r.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := r.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return nil, err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Spec = desired.Spec
			existing.Labels = desired.Labels
			err := r.Update(context.TODO(), existing)
			return nil, err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	return nil, nil
}

func (r *VectorReconciler) reconcileSecret(obj runtime.Object) (*reconcile.Result, error) {

	existing := &corev1.Secret{}
	desired := obj.(*corev1.Secret)

	err := r.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := r.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return nil, err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Data = desired.Data
			existing.Labels = desired.Labels
			err := r.Update(context.TODO(), existing)
			return nil, err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	return nil, nil
}

func (r *VectorReconciler) reconcileDaemonSet(obj runtime.Object) (*reconcile.Result, error) {

	existing := &appsv1.DaemonSet{}
	desired := obj.(*appsv1.DaemonSet)

	err := r.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := r.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return nil, err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Spec = desired.Spec
			existing.Labels = desired.Labels
			err := r.Update(context.TODO(), existing)
			return nil, err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	return nil, nil
}

func (r *VectorReconciler) reconcileStatefulSet(obj runtime.Object) (*reconcile.Result, error) {

	existing := &appsv1.StatefulSet{}
	desired := obj.(*appsv1.StatefulSet)

	err := r.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := r.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return nil, err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Spec = desired.Spec
			existing.Labels = desired.Labels
			err := r.Update(context.TODO(), existing)
			return nil, err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	return nil, nil
}

func (r *VectorReconciler) reconcileServiceAccount(obj runtime.Object) (*reconcile.Result, error) {

	existing := &corev1.ServiceAccount{}
	desired := obj.(*corev1.ServiceAccount)

	err := r.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := r.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return nil, err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	return nil, nil
}

func (r *VectorReconciler) reconcileClusterRole(obj runtime.Object) (*reconcile.Result, error) {

	existing := &rbacv1.ClusterRole{}
	desired := obj.(*rbacv1.ClusterRole)

	err := r.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := r.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return nil, err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Rules = desired.Rules
			err := r.Update(context.TODO(), existing)
			return nil, err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	return nil, nil
}

func (r *VectorReconciler) reconcileClusterRoleBinding(obj runtime.Object) (*reconcile.Result, error) {

	existing := &rbacv1.ClusterRoleBinding{}
	desired := obj.(*rbacv1.ClusterRoleBinding)

	err := r.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := r.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return nil, err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.RoleRef = desired.RoleRef
			existing.Subjects = desired.Subjects
			err := r.Update(context.TODO(), existing)
			return nil, err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	return nil, nil
}
