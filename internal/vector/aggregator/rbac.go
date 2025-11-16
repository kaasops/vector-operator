package aggregator

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

const ApiPort = 8686

func (ctrl *Controller) ensureVectorAggregatorRBAC(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-rbac", ctrl.Name)

	log.Info("start Reconcile Vector Aggregator RBAC")

	if err := ctrl.ensureVectorAggregatorServiceAccount(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureVectorAggregatorClusterRole(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureVectorAggregatorClusterRoleBinding(ctx); err != nil {
		return err
	}
	return nil
}

func (ctrl *Controller) ensureVectorAggregatorServiceAccount(ctx context.Context) error {
	return k8s.CreateOrUpdateResource(ctx, ctrl.createVectorAggregatorServiceAccount(), ctrl.Client)
}

func (ctrl *Controller) ensureVectorAggregatorClusterRole(ctx context.Context) error {
	return k8s.CreateOrUpdateResource(ctx, ctrl.createVectorAggregatorClusterRole(), ctrl.Client)
}

func (ctrl *Controller) ensureVectorAggregatorClusterRoleBinding(ctx context.Context) error {
	return k8s.CreateOrUpdateResource(ctx, ctrl.createVectorAggregatorClusterRoleBinding(), ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorServiceAccount() *corev1.ServiceAccount {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
	}

	return serviceAccount
}

func (ctrl *Controller) createVectorAggregatorClusterRole() *rbacv1.ClusterRole {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ""),
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces", "nodes", "pods"},
				Verbs:     []string{"list", "watch"},
			},
		},
	}

	return clusterRole
}

func (ctrl *Controller) createVectorAggregatorClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ""),
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     ctrl.getNameVectorAggregator(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrl.getNameVectorAggregator(),
				Namespace: ctrl.Namespace,
			},
		},
	}

	return clusterRoleBinding
}

func (ctrl *Controller) deleteVectorAggregatorClusterRole(ctx context.Context) error {
	return ctrl.Client.Delete(ctx, ctrl.createVectorAggregatorClusterRole())
}

func (ctrl *Controller) deleteVectorAggregatorClusterRoleBinding(ctx context.Context) error {
	return ctrl.Client.Delete(ctx, ctrl.createVectorAggregatorClusterRoleBinding())
}
