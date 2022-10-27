package vectoragent

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func (vr *VectorAgentReconciler) createVectorAgentServiceAccount() *corev1.ServiceAccount {
	labels := vr.labelsForVectorAgent()

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: vr.objectMetaVectorAgent(labels),
	}

	return serviceAccount
}

func (vr *VectorAgentReconciler) createVectorAgentClusterRole() *rbacv1.ClusterRole {
	labels := vr.labelsForVectorAgent()

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: vr.objectMetaVectorAgent(labels),
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

func (vr *VectorAgentReconciler) createVectorAgentClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	labels := vr.labelsForVectorAgent()

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: vr.objectMetaVectorAgent(labels),
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     vr.getNameVectorAgent(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      vr.getNameVectorAgent(),
				Namespace: vr.Vector.Namespace,
			},
		},
	}

	return clusterRoleBinding
}
