/*
Copyright 2022.

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

package vectoragent

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func (ctrl *Controller) createVectorAgentServiceAccount() *corev1.ServiceAccount {
	labels := ctrl.labelsForVectorAgent()
	annotations := ctrl.annotationsForVectorAgent()

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels, annotations, ctrl.Vector.Namespace),
	}

	return serviceAccount
}

func (ctrl *Controller) createVectorAgentClusterRole() *rbacv1.ClusterRole {
	labels := ctrl.labelsForVectorAgent()
	annotations := ctrl.annotationsForVectorAgent()

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels, annotations, ""),
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

func (ctrl *Controller) createVectorAgentClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	labels := ctrl.labelsForVectorAgent()
	annotations := ctrl.annotationsForVectorAgent()

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels, annotations, ""),
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     ctrl.getNameVectorAgent(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrl.getNameVectorAgent(),
				Namespace: ctrl.Vector.Namespace,
			},
		},
	}

	return clusterRoleBinding
}
