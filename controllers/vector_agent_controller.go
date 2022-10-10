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

func (r *VectorReconciler) ensureVectorAgent(vectorCR *vectorv1alpha1.Vector) (done bool, result ctrl.Result, err error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent", vectorCR.Name)

	log.Info("start Reconcile Vector Agent")

	if done, result, err = r.ensureVectorAgentRBAC(vectorCR); done {
		return
	}

	if vectorCR.Spec.Agent.Service {
		if done, result, err = r.ensureVectorAgentService(vectorCR); done {
			return
		}
	}

	if done, result, err = r.ensureVectorAgentSecret(vectorCR); done {
		return
	}

	if done, result, err = r.ensureVectorAgentDaemonSet(vectorCR); done {
		return
	}

	return
}

func (r *VectorReconciler) ensureVectorAgentRBAC(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", vectorCR.Name)

	log.Info("start Reconcile Vector Agent RBAC")

	if done, _, err := r.ensureVectorAgentServiceAccount(vectorCR); done {
		return ReconcileResult(err)
	}
	if done, _, err := r.ensureVectorAgentClusterRole(vectorCR); done {
		return ReconcileResult(err)
	}
	if done, _, err := r.ensureVectorAgentClusterRoleBinding(vectorCR); done {
		return ReconcileResult(err)
	}

	return ReconcileResult(nil)
}

func (r *VectorReconciler) ensureVectorAgentServiceAccount(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	vectorAgentServiceAccount := r.createVectorAgentServiceAccount(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAgentServiceAccount, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateServiceAccount(vectorAgentServiceAccount)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAgentClusterRole(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	vectorAgentClusterRole := r.createVectorAgentClusterRole(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAgentClusterRole, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateClusterRole(vectorAgentClusterRole)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAgentClusterRoleBinding(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	vectorAgentClusterRoleBinding := r.createVectorAgentClusterRoleBinding(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAgentClusterRoleBinding, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateClusterRoleBinding(vectorAgentClusterRoleBinding)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAgentService(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-service", vectorCR.Name)

	log.Info("start Reconcile Vector Agent Service")

	vectorAgentService := r.createVectorAgentService(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAgentService, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateService(vectorAgentService)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAgentSecret(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-secret", vectorCR.Name)

	log.Info("start Reconcile Vector Agent Secret")

	vectorAgentSecret := r.createVectorAgentSecret(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAgentSecret, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateSecret(vectorAgentSecret)

	return ReconcileResult(err)
}

func (r *VectorReconciler) ensureVectorAgentDaemonSet(vectorCR *vectorv1alpha1.Vector) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-daemon-set", vectorCR.Name)

	log.Info("start Reconcile Vector Agent DaemonSet")

	vectorAgentDaemonSet := r.createVectorAgentDaemonSet(vectorCR)

	if err := controllerutil.SetControllerReference(vectorCR, vectorAgentDaemonSet, r.Scheme); err != nil {
		return ReconcileResult(err)
	}
	_, err := r.CreateOrUpdateDaemonSet(vectorAgentDaemonSet)

	return ReconcileResult(err)
}

func labelsForVectorAgent(name string) map[string]string {
	return map[string]string{
		label.ManagedByLabelKey:  "vector-operator",
		label.NameLabelKey:       "vector",
		label.ComponentLabelKey:  "Agent",
		label.InstanceLabelKey:   name,
		label.VectorExcludeLabel: "true",
	}
}

func objectMetaVectorAgent(v *vectorv1alpha1.Vector, labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      v.Name + "-agent",
		Namespace: v.Namespace,
		Labels:    labels,
	}
}

func getNameVectorAgent(v *vectorv1alpha1.Vector) string {
	name := v.Name + "-agent"
	return name
}
