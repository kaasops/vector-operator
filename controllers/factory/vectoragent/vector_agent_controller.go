package vectoragent

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/helper"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"github.com/kaasops/vector-operator/controllers/factory/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type VectorAgentReconciler struct {
	client.Client
	Vector *vectorv1alpha1.Vector

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset *kubernetes.Clientset
}

func NewReconciler(v *vectorv1alpha1.Vector, c client.Client, cs *kubernetes.Clientset) *VectorAgentReconciler {
	return &VectorAgentReconciler{
		Client:    c,
		Vector:    v,
		Clientset: cs,
	}
}

func (vr *VectorAgentReconciler) EnsureVectorAgent() (done bool, result ctrl.Result, err error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent", vr.Vector.Name)

	log.Info("start Reconcile Vector Agent")

	if done, result, err = vr.ensureVectorAgentRBAC(); done {
		return
	}

	if vr.Vector.Spec.Agent.Service {
		if done, result, err = vr.ensureVectorAgentService(); done {
			return
		}
	}

	if done, result, err = vr.ensureVectorAgentConfig(); done {
		return
	}

	if done, result, err = vr.ensureVectorAgentDaemonSet(); done {
		return
	}

	return
}

func (vr *VectorAgentReconciler) ensureVectorAgentRBAC() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", vr.Vector.Name)

	log.Info("start Reconcile Vector Agent RBAC")

	if done, _, err := vr.ensureVectorAgentServiceAccount(); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := vr.ensureVectorAgentClusterRole(); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := vr.ensureVectorAgentClusterRoleBinding(); done {
		return helper.ReconcileResult(err)
	}

	return helper.ReconcileResult(nil)
}

func (vr *VectorAgentReconciler) ensureVectorAgentServiceAccount() (bool, ctrl.Result, error) {
	vectorAgentServiceAccount := vr.createVectorAgentServiceAccount()

	_, err := k8sutils.CreateOrUpdateServiceAccount(vectorAgentServiceAccount, vr.Client)

	return helper.ReconcileResult(err)
}

func (vr *VectorAgentReconciler) ensureVectorAgentClusterRole() (bool, ctrl.Result, error) {
	vectorAgentClusterRole := vr.createVectorAgentClusterRole()

	_, err := k8sutils.CreateOrUpdateClusterRole(vectorAgentClusterRole, vr.Client)

	return helper.ReconcileResult(err)
}

func (vr *VectorAgentReconciler) ensureVectorAgentClusterRoleBinding() (bool, ctrl.Result, error) {
	vectorAgentClusterRoleBinding := vr.createVectorAgentClusterRoleBinding()

	_, err := k8sutils.CreateOrUpdateClusterRoleBinding(vectorAgentClusterRoleBinding, vr.Client)

	return helper.ReconcileResult(err)
}

func (vr *VectorAgentReconciler) ensureVectorAgentService() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-service", vr.Vector.Name)

	log.Info("start Reconcile Vector Agent Service")

	vectorAgentService := vr.createVectorAgentService()

	_, err := k8sutils.CreateOrUpdateService(vectorAgentService, vr.Client)

	return helper.ReconcileResult(err)
}

func (vr *VectorAgentReconciler) ensureVectorAgentConfig() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-secret", vr.Vector.Name)

	log.Info("start Reconcile Vector Agent Secret")

	vectorAgentSecret, err := vr.createVectorAgentConfig(ctx)
	if err != nil {
		return helper.ReconcileResult(err)
	}

	_, err = k8sutils.CreateOrUpdateSecret(vectorAgentSecret, vr.Client)

	return helper.ReconcileResult(err)
}

func (vr *VectorAgentReconciler) ensureVectorAgentDaemonSet() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-daemon-set", vr.Vector.Name)

	log.Info("start Reconcile Vector Agent DaemonSet")

	vectorAgentDaemonSet := vr.createVectorAgentDaemonSet()

	_, err := k8sutils.CreateOrUpdateDaemonSet(vectorAgentDaemonSet, vr.Client)

	return helper.ReconcileResult(err)
}

func (vr *VectorAgentReconciler) labelsForVectorAgent() map[string]string {
	return map[string]string{
		label.ManagedByLabelKey:  "vector-operator",
		label.NameLabelKey:       "vector",
		label.ComponentLabelKey:  "Agent",
		label.InstanceLabelKey:   vr.Vector.Name,
		label.VectorExcludeLabel: "true",
	}
}

func (vr *VectorAgentReconciler) objectMetaVectorAgent(labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            vr.Vector.Name + "-agent",
		Namespace:       vr.Vector.Namespace,
		Labels:          labels,
		OwnerReferences: vr.getControllerReference(),
	}
}

func (vr *VectorAgentReconciler) getNameVectorAgent() string {
	name := vr.Vector.Name + "-agent"
	return name
}

func (vr *VectorAgentReconciler) getControllerReference() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         vr.Vector.APIVersion,
			Kind:               vr.Vector.Kind,
			Name:               vr.Vector.GetName(),
			UID:                vr.Vector.GetUID(),
			BlockOwnerDeletion: pointer.BoolPtr(true),
			Controller:         pointer.BoolPtr(true),
		},
	}
}

func setSucceesStatus(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client) error {
	var status = true
	v.Status.ConfigCheckResult = &status
	v.Status.Reason = nil

	return k8sutils.UpdateStatus(ctx, v, c)
}

func setFailedStatus(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client, err error) error {
	var status = false
	var reason = err.Error()
	v.Status.ConfigCheckResult = &status
	v.Status.Reason = &reason

	return k8sutils.UpdateStatus(ctx, v, c)
}

func SetLastAppliedPipelineStatus(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client, hash *uint32) error {

	v.Status.LastAppliedConfigHash = hash
	if err := k8sutils.UpdateStatus(ctx, v, c); err != nil {
		return err
	}
	return nil
}
