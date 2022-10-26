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

func EnsureVectorAgent(v *vectorv1alpha1.Vector, rclient client.Client, cs *kubernetes.Clientset) (done bool, result ctrl.Result, err error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent", v.Name)

	log.Info("start Reconcile Vector Agent")

	if done, result, err = ensureVectorAgentRBAC(v, rclient); done {
		return
	}

	if v.Spec.Agent.Service {
		if done, result, err = ensureVectorAgentService(v, rclient); done {
			return
		}
	}

	if done, result, err = ensureVectorAgentConfig(v, rclient, cs); done {
		return
	}

	if done, result, err = ensureVectorAgentDaemonSet(v, rclient); done {
		return
	}

	return
}

func ensureVectorAgentRBAC(v *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", v.Name)

	log.Info("start Reconcile Vector Agent RBAC")

	if done, _, err := ensureVectorAgentServiceAccount(v, rclient); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := ensureVectorAgentClusterRole(v, rclient); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := ensureVectorAgentClusterRoleBinding(v, rclient); done {
		return helper.ReconcileResult(err)
	}

	return helper.ReconcileResult(nil)
}

func ensureVectorAgentServiceAccount(v *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	vectorAgentServiceAccount := createVectorAgentServiceAccount(v)

	_, err := k8sutils.CreateOrUpdateServiceAccount(vectorAgentServiceAccount, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentClusterRole(v *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	vectorAgentClusterRole := createVectorAgentClusterRole(v)

	_, err := k8sutils.CreateOrUpdateClusterRole(vectorAgentClusterRole, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentClusterRoleBinding(v *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	vectorAgentClusterRoleBinding := createVectorAgentClusterRoleBinding(v)

	_, err := k8sutils.CreateOrUpdateClusterRoleBinding(vectorAgentClusterRoleBinding, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentService(v *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-service", v.Name)

	log.Info("start Reconcile Vector Agent Service")

	vectorAgentService := createVectorAgentService(v)

	_, err := k8sutils.CreateOrUpdateService(vectorAgentService, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentConfig(v *vectorv1alpha1.Vector, rclient client.Client, cs *kubernetes.Clientset) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-secret", v.Name)

	log.Info("start Reconcile Vector Agent Secret")

	vectorAgentSecret, err := createVectorAgentConfig(ctx, v, rclient, cs)
	if err != nil {
		return helper.ReconcileResult(err)
	}

	_, err = k8sutils.CreateOrUpdateSecret(vectorAgentSecret, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentDaemonSet(v *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-daemon-set", v.Name)

	log.Info("start Reconcile Vector Agent DaemonSet")

	vectorAgentDaemonSet := createVectorAgentDaemonSet(v)

	_, err := k8sutils.CreateOrUpdateDaemonSet(vectorAgentDaemonSet, rclient)

	return helper.ReconcileResult(err)
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
		Name:            v.Name + "-agent",
		Namespace:       v.Namespace,
		Labels:          labels,
		OwnerReferences: getControllerReference(v),
	}
}

func getNameVectorAgent(v *vectorv1alpha1.Vector) string {
	name := v.Name + "-agent"
	return name
}

func getControllerReference(owner *vectorv1alpha1.Vector) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         owner.APIVersion,
			Kind:               owner.Kind,
			Name:               owner.GetName(),
			UID:                owner.GetUID(),
			BlockOwnerDeletion: pointer.BoolPtr(true),
			Controller:         pointer.BoolPtr(true),
		},
	}
}

func setSucceesStatus(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client) error {
	var status = true
	v.Status.ConfigCheckResult = &status
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
