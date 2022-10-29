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

package k8s

import (
	"context"
	"testing"

	// . "github.com/onsi/ginkgo/v2"
	// . "github.com/onsi/gomega"
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

			err := CreatePod(obj, cl)
			req.Equal(err, want)
		}
	}

	obj_init := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}
	obj_case1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	obj_case2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	gr_case2 := schema.GroupResource{
		Group:    "",
		Resource: "pods",
	}
	error_case2 := apierrors.NewAlreadyExists(gr_case2, obj_case2.ObjectMeta.Name)

	t.Run("Create not exist pod case", createPodCase(obj_init, obj_case1, nil))
	t.Run("Create alredy exist pod case", createPodCase(obj_init, obj_case2, error_case2))
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

			result, err := GetPod(obj, cl)
			if result != nil {
				req.Equal(result.ObjectMeta, wantPod.ObjectMeta)
			}
			req.Equal(err, want)
		}
	}

	obj_init := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}
	obj_case1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	gvr_case1 := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
	error_case1 := apierrors.NewNotFound(gvr_case1.GroupResource(), obj_case1.ObjectMeta.Name)
	obj_case2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	t.Run("Get not exist pod case", getPodCase(obj_init, obj_case1, nil, error_case1))
	t.Run("Get exist pod case", getPodCase(obj_init, obj_case2, obj_init, nil))
}

func TestUpdateStatus(t *testing.T) {
	updateStatusCase := func(objInit, obj client.Object, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := UpdateStatus(context.Background(), obj, cl)

			req.Equal(err, want)
		}
	}

	obj_init := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}
	obj_case1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	obj_case2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	obj_case3 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	obj_case4 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	obj_case5 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 10,
		},
	}
	gvr_case5 := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}
	error_case5 := apierrors.NewNotFound(gvr_case5.GroupResource(), obj_case5.ObjectMeta.Name)

	init_obj_case6 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 5,
		},
	}
	obj_case6 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 10,
		},
	}

	t.Run("Update Simple case", updateStatusCase(obj_init, obj_case1, nil))
	t.Run("Update Alredy exist case", updateStatusCase(obj_init, obj_case2, nil))
	t.Run("Update with Another Namespace case", updateStatusCase(obj_init, obj_case3, nil))
	t.Run("Update without Name case", updateStatusCase(obj_init, obj_case4, error_case4))
	t.Run("Update not exist Deployment with wrong init case", updateStatusCase(obj_init, obj_case5, error_case5))
	t.Run("Update Deployment case", updateStatusCase(init_obj_case6, obj_case6, nil))

}

func TestCreateOrUpdateService(t *testing.T) {
	reconcileServiceCase := func(objInit, obj *corev1.Service, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := CreateOrUpdateService(obj, cl)

			req.Equal(err, want)
		}
	}

	service_init := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "init",
			Namespace:       "test-namespace",
			ResourceVersion: "",
		},
	}

	service_case1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	service_case2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	service_case3 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	service_case4 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileServiceCase(service_init, service_case1, nil))
	t.Run("Create Alredy exist case", reconcileServiceCase(service_init, service_case2, nil))
	t.Run("Create with Another Namespace case", reconcileServiceCase(service_init, service_case3, nil))
	t.Run("Create without Name case", reconcileServiceCase(service_init, service_case4, error_case4))
}

func TestCreateOrUpdateSecret(t *testing.T) {
	reconcileSecretCase := func(objInit, obj *corev1.Secret, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := CreateOrUpdateSecret(obj, cl)

			req.Equal(err, want)
		}
	}

	secret_init := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	secret_case1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	secret_case2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	secret_case3 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	secret_case4 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileSecretCase(secret_init, secret_case1, nil))
	t.Run("Create Alredy exist case", reconcileSecretCase(secret_init, secret_case2, nil))
	t.Run("Create with Another Namespace case", reconcileSecretCase(secret_init, secret_case3, nil))
	t.Run("Create without Name case", reconcileSecretCase(secret_init, secret_case4, error_case4))
}

func TestCreateOrUpdateDaemonSet(t *testing.T) {
	reconcileDaemonSetCase := func(objInit, obj *appsv1.DaemonSet, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := CreateOrUpdateDaemonSet(obj, cl)

			req.Equal(err, want)
		}
	}

	daemonSet_init := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	daemonSet_case1 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	daemonSet_case2 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	daemonSet_case3 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	daemonSet_case4 := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileDaemonSetCase(daemonSet_init, daemonSet_case1, nil))
	t.Run("Create Alredy exist case", reconcileDaemonSetCase(daemonSet_init, daemonSet_case2, nil))
	t.Run("Create with Another Namespace case", reconcileDaemonSetCase(daemonSet_init, daemonSet_case3, nil))
	t.Run("Create without Name case", reconcileDaemonSetCase(daemonSet_init, daemonSet_case4, error_case4))
}

func TestCreateOrUpdateStatefulSet(t *testing.T) {
	reconcileStatefulSetCase := func(objInit, obj *appsv1.StatefulSet, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := CreateOrUpdateStatefulSet(obj, cl)

			req.Equal(err, want)
		}
	}

	statefulSet_init := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	statefulSet_case1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	statefulSet_case2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	statefulSet_case3 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	statefulSet_case4 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileStatefulSetCase(statefulSet_init, statefulSet_case1, nil))
	t.Run("Create Alredy exist case", reconcileStatefulSetCase(statefulSet_init, statefulSet_case2, nil))
	t.Run("Create with Another Namespace case", reconcileStatefulSetCase(statefulSet_init, statefulSet_case3, nil))
	t.Run("Create without Name case", reconcileStatefulSetCase(statefulSet_init, statefulSet_case4, error_case4))
}

func TestCreateOrUpdateServiceAccount(t *testing.T) {
	reconcileServiceAccountCase := func(objInit, obj *corev1.ServiceAccount, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := CreateOrUpdateServiceAccount(obj, cl)

			req.Equal(err, want)
		}
	}

	serviceAccount_init := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	serviceAccount_case1 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	serviceAccount_case2 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	serviceAccount_case3 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	serviceAccount_case4 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileServiceAccountCase(serviceAccount_init, serviceAccount_case1, nil))
	t.Run("Create Alredy exist case", reconcileServiceAccountCase(serviceAccount_init, serviceAccount_case2, nil))
	t.Run("Create with Another Namespace case", reconcileServiceAccountCase(serviceAccount_init, serviceAccount_case3, nil))
	t.Run("Create without Name case", reconcileServiceAccountCase(serviceAccount_init, serviceAccount_case4, error_case4))
}

func TestCreateOrUpdateClusterRole(t *testing.T) {
	reconcileClusterRole := func(objInit, obj *rbacv1.ClusterRole, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := CreateOrUpdateClusterRole(obj, cl)

			req.Equal(err, want)
		}
	}

	clusterRole_init := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	clusterRole_case1 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	clusterRole_case2 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	clusterRole_case3 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	clusterRole_case4 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileClusterRole(clusterRole_init, clusterRole_case1, nil))
	t.Run("Create Alredy exist case", reconcileClusterRole(clusterRole_init, clusterRole_case2, nil))
	t.Run("Create with Another Namespace case", reconcileClusterRole(clusterRole_init, clusterRole_case3, nil))
	t.Run("Create without Name case", reconcileClusterRole(clusterRole_init, clusterRole_case4, error_case4))
}

func TestCreateOrUpdateClusterRoleBinding(t *testing.T) {
	reconcileClusterRoleBinding := func(objInit, obj *rbacv1.ClusterRoleBinding, want error) func(t *testing.T) {
		return func(t *testing.T) {
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := CreateOrUpdateClusterRoleBinding(obj, cl)

			req.Equal(err, want)
		}
	}

	clusterRoleBinding_init := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}

	clusterRoleBinding_case1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace",
		},
	}
	clusterRoleBinding_case2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init",
			Namespace: "test-namespace",
		},
	}
	clusterRoleBinding_case3 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-namespace2",
		},
	}
	clusterRoleBinding_case4 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{},
	}
	error_case4 := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{field.Required(field.NewPath("metadata.name"), "name is required")})

	t.Run("Create Simple case", reconcileClusterRoleBinding(clusterRoleBinding_init, clusterRoleBinding_case1, nil))
	t.Run("Create Alredy exist case", reconcileClusterRoleBinding(clusterRoleBinding_init, clusterRoleBinding_case2, nil))
	t.Run("Create with Another Namespace case", reconcileClusterRoleBinding(clusterRoleBinding_init, clusterRoleBinding_case3, nil))
	t.Run("Create without Name case", reconcileClusterRoleBinding(clusterRoleBinding_init, clusterRoleBinding_case4, error_case4))
}
