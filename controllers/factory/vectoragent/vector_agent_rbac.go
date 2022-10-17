package vectoragent

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func createVectorAgentServiceAccount(v *vectorv1alpha1.Vector) *corev1.ServiceAccount {
	labels := labelsForVectorAgent(v.Name)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: objectMetaVectorAgent(v, labels),
	}

	return serviceAccount
}

func createVectorAgentClusterRole(v *vectorv1alpha1.Vector) *rbacv1.ClusterRole {
	labels := labelsForVectorAgent(v.Name)

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: objectMetaVectorAgent(v, labels),
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

func createVectorAgentClusterRoleBinding(v *vectorv1alpha1.Vector) *rbacv1.ClusterRoleBinding {
	labels := labelsForVectorAgent(v.Name)

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: objectMetaVectorAgent(v, labels),
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     getNameVectorAgent(v),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      getNameVectorAgent(v),
				Namespace: v.Namespace,
			},
		},
	}

	return clusterRoleBinding
}
