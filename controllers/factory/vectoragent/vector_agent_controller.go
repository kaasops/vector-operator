package vectoragent

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/helper"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"github.com/kaasops/vector-operator/controllers/factory/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func EnsureVectorAgent(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (done bool, result ctrl.Result, err error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent", vectorCR.Name)

	log.Info("start Reconcile Vector Agent")

	if done, result, err = ensureVectorAgentRBAC(vectorCR, rclient); done {
		return
	}

	if vectorCR.Spec.Agent.Service {
		if done, result, err = ensureVectorAgentService(vectorCR, rclient); done {
			return
		}
	}

	if done, result, err = ensureVectorAgentConfig(vectorCR, rclient); done {
		return
	}

	if done, result, err = ensureVectorAgentDaemonSet(vectorCR, rclient); done {
		return
	}

	return
}

func ensureVectorAgentRBAC(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", vectorCR.Name)

	log.Info("start Reconcile Vector Agent RBAC")

	if done, _, err := ensureVectorAgentServiceAccount(vectorCR, rclient); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := ensureVectorAgentClusterRole(vectorCR, rclient); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := ensureVectorAgentClusterRoleBinding(vectorCR, rclient); done {
		return helper.ReconcileResult(err)
	}

	return helper.ReconcileResult(nil)
}

func ensureVectorAgentServiceAccount(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	vectorAgentServiceAccount := createVectorAgentServiceAccount(vectorCR)

	_, err := k8sutils.CreateOrUpdateServiceAccount(vectorAgentServiceAccount, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentClusterRole(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	vectorAgentClusterRole := createVectorAgentClusterRole(vectorCR)

	_, err := k8sutils.CreateOrUpdateClusterRole(vectorAgentClusterRole, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentClusterRoleBinding(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	vectorAgentClusterRoleBinding := createVectorAgentClusterRoleBinding(vectorCR)

	_, err := k8sutils.CreateOrUpdateClusterRoleBinding(vectorAgentClusterRoleBinding, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentService(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-service", vectorCR.Name)

	log.Info("start Reconcile Vector Agent Service")

	vectorAgentService := createVectorAgentService(vectorCR)

	_, err := k8sutils.CreateOrUpdateService(vectorAgentService, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentConfig(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-secret", vectorCR.Name)

	log.Info("start Reconcile Vector Agent Secret")

	vectorAgentSecret, err := createVectorAgentConfig(ctx, vectorCR, rclient)
	if err != nil {
		return helper.ReconcileResult(err)
	}

	_, err = k8sutils.CreateOrUpdateSecret(vectorAgentSecret, rclient)

	return helper.ReconcileResult(err)
}

func ensureVectorAgentDaemonSet(vectorCR *vectorv1alpha1.Vector, rclient client.Client) (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-daemon-set", vectorCR.Name)

	log.Info("start Reconcile Vector Agent DaemonSet")

	vectorAgentDaemonSet := createVectorAgentDaemonSet(vectorCR)

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
