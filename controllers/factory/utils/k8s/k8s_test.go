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

func getInitObjectMeta() metav1.ObjectMeta {
	ObjectMeta := metav1.ObjectMeta{
		Name:      "init",
		Namespace: "test-namespace",
	}

	return ObjectMeta
}

var reconcileObjectCase = func(objInit, obj interface{}, want error) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()
		t.Parallel()

		req := require.New(t)

		switch obj.(type) {
		case *corev1.Service:
			serviceInit := objInit.(*corev1.Service)
			service := obj.(*corev1.Service)

			cl := fake.NewClientBuilder().WithObjects(serviceInit).Build()
			err := k8s.CreateOrUpdateService(service, cl)
			req.Equal(err, want)
		case *corev1.Secret:
			secretInit := objInit.(*corev1.Secret)
			secret := obj.(*corev1.Secret)

			cl := fake.NewClientBuilder().WithObjects(secretInit).Build()
			err := k8s.CreateOrUpdateSecret(secret, cl)
			req.Equal(err, want)
		case *appsv1.DaemonSet:
			daemonSetInit := objInit.(*appsv1.DaemonSet)
			daemonSet := obj.(*appsv1.DaemonSet)

			cl := fake.NewClientBuilder().WithObjects(daemonSetInit).Build()
			err := k8s.CreateOrUpdateDaemonSet(daemonSet, cl)
			req.Equal(err, want)
		case *appsv1.StatefulSet:
			statefulSetInit := objInit.(*appsv1.StatefulSet)
			statefulSet := obj.(*appsv1.StatefulSet)

			cl := fake.NewClientBuilder().WithObjects(statefulSetInit).Build()
			err := k8s.CreateOrUpdateStatefulSet(statefulSet, cl)
			req.Equal(err, want)
		case *corev1.ServiceAccount:
			serviceAccountInit := objInit.(*corev1.ServiceAccount)
			serviceAccount := obj.(*corev1.ServiceAccount)

			cl := fake.NewClientBuilder().WithObjects(serviceAccountInit).Build()
			err := k8s.CreateOrUpdateServiceAccount(serviceAccount, cl)
			req.Equal(err, want)
		case *rbacv1.ClusterRole:
			clusterRoleInit := objInit.(*rbacv1.ClusterRole)
			clusterRole := obj.(*rbacv1.ClusterRole)

			cl := fake.NewClientBuilder().WithObjects(clusterRoleInit).Build()
			err := k8s.CreateOrUpdateClusterRole(clusterRole, cl)
			req.Equal(err, want)
		case *rbacv1.ClusterRoleBinding:
			clusterRoleBindingInit := objInit.(*rbacv1.ClusterRoleBinding)
			clusterRoleBinding := obj.(*rbacv1.ClusterRoleBinding)

			cl := fake.NewClientBuilder().WithObjects(clusterRoleBindingInit).Build()
			err := k8s.CreateOrUpdateClusterRoleBinding(clusterRoleBinding, cl)
			req.Equal(err, want)
		}
	}
}

var nameRequeriedError = apierrors.NewInvalid(
	schema.GroupKind{},
	"",
	field.ErrorList{
		field.Required(
			field.NewPath("metadata.name"),
			"name is required",
		),
	},
)

func TestCreateOrUpdateService(t *testing.T) {
	initObj := &corev1.Service{
		ObjectMeta: getInitObjectMeta(),
	}

	type secriveCase struct {
		name string
		obj  *corev1.Service
		err  error
	}

	secriveCases := []secriveCase{
		{
			name: "Create Simple case",
			obj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create Alredy exist case",
			obj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Create with Another Namespace case",
			obj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			err: nil,
		},
		{
			name: "Create without Name case",
			obj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
	}

	t.Parallel()
	for _, tc := range secriveCases {
		t.Run(tc.name, reconcileObjectCase(initObj, tc.obj, tc.err))
	}
}

func TestCreateOrUpdateSecret(t *testing.T) {
	initObj := &corev1.Secret{
		ObjectMeta: getInitObjectMeta(),
	}

	type secretCase struct {
		name string
		obj  *corev1.Secret
		err  error
	}

	secretCases := []secretCase{
		{
			name: "Create Simple case",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create Alredy exist case",
			obj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Create with Another Namespace case",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			err: nil,
		},
		{
			name: "Create without Name case",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
	}

	t.Parallel()
	for _, tc := range secretCases {
		t.Run(tc.name, reconcileObjectCase(initObj, tc.obj, tc.err))
	}
}

func TestCreateOrUpdateDaemonSet(t *testing.T) {
	initObj := &appsv1.DaemonSet{
		ObjectMeta: getInitObjectMeta(),
	}

	type daemonSetCase struct {
		name string
		obj  *appsv1.DaemonSet
		err  error
	}

	daemonSetCases := []daemonSetCase{
		{
			name: "Create Simple case",
			obj: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create Alredy exist case",
			obj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Create with Another Namespace case",
			obj: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			err: nil,
		},
		{
			name: "Create without Name case",
			obj: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
	}

	t.Parallel()
	for _, tc := range daemonSetCases {
		t.Run(tc.name, reconcileObjectCase(initObj, tc.obj, tc.err))
	}
}

func TestCreateOrUpdateStatefulSet(t *testing.T) {
	initObj := &appsv1.StatefulSet{
		ObjectMeta: getInitObjectMeta(),
	}

	type statefulSetCase struct {
		name string
		obj  *appsv1.StatefulSet
		err  error
	}

	statefulSetCases := []statefulSetCase{
		{
			name: "Create Simple case",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create Alredy exist case",
			obj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Create with Another Namespace case",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			err: nil,
		},
		{
			name: "Create without Name case",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
	}

	t.Parallel()
	for _, tc := range statefulSetCases {
		t.Run(tc.name, reconcileObjectCase(initObj, tc.obj, tc.err))
	}
}

func TestCreateOrUpdateServiceAccount(t *testing.T) {
	initObj := &corev1.ServiceAccount{
		ObjectMeta: getInitObjectMeta(),
	}

	type serviceAccountCase struct {
		name string
		obj  *corev1.ServiceAccount
		err  error
	}

	serviceAccountCases := []serviceAccountCase{
		{
			name: "Create Simple case",
			obj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create Alredy exist case",
			obj: &corev1.ServiceAccount{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Create with Another Namespace case",
			obj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			err: nil,
		},
		{
			name: "Create without Name case",
			obj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
	}

	t.Parallel()
	for _, tc := range serviceAccountCases {
		t.Run(tc.name, reconcileObjectCase(initObj, tc.obj, tc.err))
	}
}

func TestCreateOrUpdateClusterRole(t *testing.T) {
	initObj := &rbacv1.ClusterRole{
		ObjectMeta: getInitObjectMeta(),
	}

	type clusterRoleCase struct {
		name string
		obj  *rbacv1.ClusterRole
		err  error
	}

	clusterRoleCases := []clusterRoleCase{
		{
			name: "Create Simple case",
			obj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create Alredy exist case",
			obj: &rbacv1.ClusterRole{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Create with Another Namespace case",
			obj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			err: nil,
		},
		{
			name: "Create without Name case",
			obj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
	}

	t.Parallel()
	for _, tc := range clusterRoleCases {
		t.Run(tc.name, reconcileObjectCase(initObj, tc.obj, tc.err))
	}
}

func TestCreateOrUpdateClusterRoleBinding(t *testing.T) {
	initObj := &rbacv1.ClusterRoleBinding{
		ObjectMeta: getInitObjectMeta(),
	}

	type clusterRoleBindingCase struct {
		name string
		obj  *rbacv1.ClusterRoleBinding
		err  error
	}

	clusterRoleBindingCases := []clusterRoleBindingCase{
		{
			name: "Create Simple case",
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create Alredy exist case",
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Create with Another Namespace case",
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			err: nil,
		},
		{
			name: "Create without Name case",
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
	}

	t.Parallel()
	for _, tc := range clusterRoleBindingCases {
		t.Run(tc.name, reconcileObjectCase(initObj, tc.obj, tc.err))
	}
}

func TestCreatePod(t *testing.T) {
	createPodCase := func(objInit, obj *corev1.Pod, want error) func(t *testing.T) {
		t.Parallel()
		return func(t *testing.T) {
			t.Helper()

			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.CreatePod(obj, cl)
			req.Equal(err, want)
		}
	}

	initObj := &corev1.Pod{
		ObjectMeta: getInitObjectMeta(),
	}

	type podCase struct {
		name string
		obj  *corev1.Pod
		err  error
	}

	podCases := []podCase{
		{
			name: "Create not exist pod case",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			err: nil,
		},
		{
			name: "Create alredy exist pod case",
			obj: &corev1.Pod{
				ObjectMeta: getInitObjectMeta(),
			},
			err: apierrors.NewAlreadyExists(
				schema.GroupResource{
					Group:    "",
					Resource: "pods",
				},
				"init",
			),
		},
	}

	t.Parallel()
	for _, tc := range podCases {
		t.Run(tc.name, createPodCase(initObj, tc.obj, tc.err))
	}
}

func TestGetPod(t *testing.T) {
	getPodCase := func(objInit, obj, wantPod *corev1.Pod, want error) func(t *testing.T) {
		t.Parallel()
		return func(t *testing.T) {
			t.Helper()

			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			result, err := k8s.GetPod(obj, cl)
			if result != nil {
				req.Equal(result.ObjectMeta, wantPod.ObjectMeta)
			}
			req.Equal(err, want)
		}
	}

	initObj := &corev1.Pod{
		ObjectMeta: getInitObjectMeta(),
	}

	type podCase struct {
		name    string
		obj     *corev1.Pod
		wantObj *corev1.Pod
		err     error
	}

	podCases := []podCase{
		{
			name: "Get not exist pod case",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			wantObj: nil,
			err: apierrors.NewNotFound(
				schema.GroupResource{
					Group:    "",
					Resource: "pods",
				},
				"test",
			),
		},
		{
			name: "Get exist pod case",
			obj: &corev1.Pod{
				ObjectMeta: getInitObjectMeta(),
			},
			wantObj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "init",
					Namespace:       "test-namespace",
					ResourceVersion: "999",
				},
			},
			err: nil,
		},
	}

	t.Parallel()
	for _, tc := range podCases {
		t.Run(tc.name, getPodCase(initObj, tc.obj, tc.wantObj, tc.err))
	}
}

func TestUpdateStatus(t *testing.T) {
	updateStatusCase := func(objInit, obj client.Object, want error) func(t *testing.T) {
		t.Parallel()
		return func(t *testing.T) {
			t.Helper()

			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()

			err := k8s.UpdateStatus(context.Background(), obj, cl)

			req.Equal(err, want)
		}
	}

	type testCase struct {
		name      string
		initObj   *appsv1.Deployment
		updateObj *appsv1.Deployment
		err       error
	}

	testCases := []testCase{
		{
			name: "Update Simple case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			updateObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Update Alredy exist case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			updateObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			err: nil,
		},
		{
			name: "Update without Name case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			updateObj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{},
			},
			err: nameRequeriedError,
		},
		{
			name: "Update status case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			updateObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
				Status: appsv1.DeploymentStatus{
					Replicas: 10,
				},
			},
			err: nil,
		},
	}

	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, updateStatusCase(tc.initObj, tc.updateObj, tc.err))
	}
}
