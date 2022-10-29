// /*
// Copyright 2022.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package k8s_test

import (
	"context"
	"testing"

	// . "github.com/onsi/ginkgo/v2"
	// . "github.com/onsi/gomega"
	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// type objects interface {
// 	*corev1.Service | *corev1.Secret | *appsv1.DaemonSet | *appsv1.StatefulSet | *corev1.ServiceAccount | *rbacv1.ClusterRole | *rbacv1.ClusterRoleBinding
// }

func TestCreatePod(t *testing.T) {
	createPodCase := func(objInit, obj *corev1.Pod, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreatePod(obj, cl)
			req.Equal(err, want)
		}
	}

	objInit := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}
	objCase1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	objCase2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	grCase2 := schema.GroupResource{
		Group:    "",
		Resource: "pods",
	}
	errorCase2 := apierrors.NewAlreadyExists(grCase2, objCase2.ObjectMeta.Name)

	t.Run("Create not exist pod case", createPodCase(objInit, objCase1, nil))
	t.Run("Create alredy exist pod case", createPodCase(objInit, objCase2, errorCase2))
}

// func CreatePod(pod *corev1.Pod, c client.Client) error {
// 	err := c.Create(context.TODO(), pod)
// 	if err != nil {
// 			return err
// 	}
// 	return nil
// }

func TestGetPod(t *testing.T) {
	getPodCase := func(objInit, obj, wantPod *corev1.Pod, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			result, err := k8s.GetPod(obj, cl)
			if result != nil {
				req.Equal(result.ObjectMeta, wantPod.ObjectMeta)
			}
			req.Equal(err, want)
		}
	}

	objInit := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}
	objCase1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	gvrCase1 := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
	errorCase1 := apierrors.NewNotFound(gvrCase1.GroupResource(), objCase1.ObjectMeta.Name)
	objCase2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	t.Run("Get not exist pod case", getPodCase(objInit, objCase1, nil, errorCase1))
	t.Run("Get exist pod case", getPodCase(objInit, objCase2, objInit, nil))
}

func TestUpdateStatus(t *testing.T) {
	updateStatusCase := func(objInit, obj client.Object, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.UpdateStatus(context.Background(), obj, cl)

			req.Equal(err, want)
		}
	}

	objInit := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}
	objCase1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	objCase2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	objCase3 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	objCase4 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	objCase5 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 10,
		},
	}
	gvrCase5 := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}
	errorCase5 := apierrors.NewNotFound(gvrCase5.GroupResource(), objCase5.ObjectMeta.Name)

	init_objCase6 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 5,
		},
	}
	objCase6 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 10,
		},
	}

	t.Run("Update Simple case", updateStatusCase(objInit, objCase1, nil))
	t.Run("Update Alredy exist case", updateStatusCase(objInit, objCase2, nil))
	t.Run("Update with Another Namespace case", updateStatusCase(objInit, objCase3, nil))
	t.Run("Update without Name case", updateStatusCase(objInit, objCase4, errorCase4))
	t.Run("Update not exist Deployment with wrong init case", updateStatusCase(objInit, objCase5, errorCase5))
	t.Run("Update Deployment case", updateStatusCase(init_objCase6, objCase6, nil))

}

func TestCreateOrUpdateService(t *testing.T) {
	reconcileServiceCase := func(objInit, obj *corev1.Service, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreateOrUpdateService(obj, cl)

			req.Equal(err, want)
		}
	}

	serviceInit := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}

	serviceCase1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	serviceCase2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	serviceCase3 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	serviceCase4 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileServiceCase(serviceInit, serviceCase1, nil))
	t.Run("Create Alredy exist case", reconcileServiceCase(serviceInit, serviceCase2, nil))
	t.Run("Create with Another Namespace case", reconcileServiceCase(serviceInit, serviceCase3, nil))
	t.Run("Create without Name case", reconcileServiceCase(serviceInit, serviceCase4, errorCase4))
}

func TestCreateOrUpdateSecret(t *testing.T) {
	reconcileSecretCase := func(objInit, obj *corev1.Secret, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreateOrUpdateSecret(obj, cl)

			req.Equal(err, want)
		}
	}

	secretInit := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	secretCase1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	secretCase2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	secretCase3 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	secretCase4 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileSecretCase(secretInit, secretCase1, nil))
	t.Run("Create Alredy exist case", reconcileSecretCase(secretInit, secretCase2, nil))
	t.Run("Create with Another Namespace case", reconcileSecretCase(secretInit, secretCase3, nil))
	t.Run("Create without Name case", reconcileSecretCase(secretInit, secretCase4, errorCase4))
}

func TestCreateOrUpdateDaemonSet(t *testing.T) {
	reconcileDaemonSetCase := func(objInit, obj *appsv1.DaemonSet, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreateOrUpdateDaemonSet(obj, cl)

			req.Equal(err, want)
		}
	}

	daemonSetInit := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	daemonSetCase1 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	daemonSetCase2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	daemonSetCase3 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	daemonSetCase4 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileDaemonSetCase(daemonSetInit, daemonSetCase1, nil))
	t.Run("Create Alredy exist case", reconcileDaemonSetCase(daemonSetInit, daemonSetCase2, nil))
	t.Run("Create with Another Namespace case", reconcileDaemonSetCase(daemonSetInit, daemonSetCase3, nil))
	t.Run("Create without Name case", reconcileDaemonSetCase(daemonSetInit, daemonSetCase4, errorCase4))
}

func TestCreateOrUpdateStatefulSet(t *testing.T) {
	reconcileStatefulSetCase := func(objInit, obj *appsv1.StatefulSet, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreateOrUpdateStatefulSet(obj, cl)

			req.Equal(err, want)
		}
	}

	statefulSetInit := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	statefulSetCase1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	statefulSetCase2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	statefulSetCase3 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	statefulSetCase4 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileStatefulSetCase(statefulSetInit, statefulSetCase1, nil))
	t.Run("Create Alredy exist case", reconcileStatefulSetCase(statefulSetInit, statefulSetCase2, nil))
	t.Run("Create with Another Namespace case", reconcileStatefulSetCase(statefulSetInit, statefulSetCase3, nil))
	t.Run("Create without Name case", reconcileStatefulSetCase(statefulSetInit, statefulSetCase4, errorCase4))
}

func TestCreateOrUpdateServiceAccount(t *testing.T) {
	reconcileServiceAccountCase := func(objInit, obj *corev1.ServiceAccount, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreateOrUpdateServiceAccount(obj, cl)

			req.Equal(err, want)
		}
	}

	serviceAccountInit := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	serviceAccountCase1 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	serviceAccountCase2 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	serviceAccountCase3 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	serviceAccountCase4 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileServiceAccountCase(serviceAccountInit, serviceAccountCase1, nil))
	t.Run("Create Alredy exist case", reconcileServiceAccountCase(serviceAccountInit, serviceAccountCase2, nil))
	t.Run("Create with Another Namespace case", reconcileServiceAccountCase(serviceAccountInit, serviceAccountCase3, nil))
	t.Run("Create without Name case", reconcileServiceAccountCase(serviceAccountInit, serviceAccountCase4, errorCase4))
}

func TestCreateOrUpdateClusterRole(t *testing.T) {
	reconcileClusterRole := func(objInit, obj *rbacv1.ClusterRole, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreateOrUpdateClusterRole(obj, cl)

			req.Equal(err, want)
		}
	}

	clusterRoleInit := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	clusterRoleCase1 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	clusterRoleCase2 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	clusterRoleCase3 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	clusterRoleCase4 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileClusterRole(clusterRoleInit, clusterRoleCase1, nil))
	t.Run("Create Alredy exist case", reconcileClusterRole(clusterRoleInit, clusterRoleCase2, nil))
	t.Run("Create with Another Namespace case", reconcileClusterRole(clusterRoleInit, clusterRoleCase3, nil))
	t.Run("Create without Name case", reconcileClusterRole(clusterRoleInit, clusterRoleCase4, errorCase4))
}

func TestCreateOrUpdateClusterRoleBinding(t *testing.T) {
	reconcileClusterRoleBinding := func(objInit, obj *rbacv1.ClusterRoleBinding, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreateOrUpdateClusterRoleBinding(obj, cl)

			req.Equal(err, want)
		}
	}

	clusterRoleBindingInit := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	clusterRoleBindingCase1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	clusterRoleBindingCase2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	clusterRoleBindingCase3 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	clusterRoleBindingCase4 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{},
	}
	errorCase4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileClusterRoleBinding(clusterRoleBindingInit, clusterRoleBindingCase1, nil))
	t.Run("Create Alredy exist case", reconcileClusterRoleBinding(clusterRoleBindingInit, clusterRoleBindingCase2, nil))
	t.Run("Create with Another Namespace case", reconcileClusterRoleBinding(clusterRoleBindingInit, clusterRoleBindingCase3, nil))
	t.Run("Create without Name case", reconcileClusterRoleBinding(clusterRoleBindingInit, clusterRoleBindingCase4, errorCase4))
}
