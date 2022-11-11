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
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type objCase struct {
	name    string
	initObj client.Object
	obj     client.Object
	want    error
}

var nameRequeriedError = api_errors.NewInvalid(
	schema.GroupKind{},
	"",
	field.ErrorList{
		field.Required(
			field.NewPath("metadata.name"),
			"name is required",
		),
	},
)

func getInitObjectMeta() metav1.ObjectMeta {
	ObjectMeta := metav1.ObjectMeta{
		Name:      "init",
		Namespace: "test-namespace",
	}

	return ObjectMeta
}

func TestCreateOrUpdateResource(t *testing.T) {
	createOrUpdateResourceCase := func(initObj, obj client.Object, want error) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(initObj).Build()
			result := k8s.CreateOrUpdateResource(context.Background(), obj, cl)
			req.Equal(result, want)
		}
	}

	var cases []objCase

	// Deployment cases
	deploymentCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.Deployment{
				ObjectMeta: getInitObjectMeta(),
				Spec: appsv1.DeploymentSpec{
					MinReadySeconds: 2,
				},
			},
			want: nil,
		},
	}
	cases = append(cases, deploymentCases...)

	// StatefulSet cases
	statefulSetCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.StatefulSet{
				ObjectMeta: getInitObjectMeta(),
				Spec: appsv1.StatefulSetSpec{
					MinReadySeconds: 2,
				},
			},
			want: nil,
		},
	}
	cases = append(cases, statefulSetCases...)

	// DaemonSet cases
	daemonSetCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &appsv1.DaemonSet{
				ObjectMeta: getInitObjectMeta(),
				Spec: appsv1.DaemonSetSpec{
					MinReadySeconds: 2,
				},
			},
			want: nil,
		},
	}
	cases = append(cases, daemonSetCases...)

	// Secret cases
	secretCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
				Data: map[string][]byte{
					"test": []byte("test"),
				},
			},
			want: nil,
		},
	}
	cases = append(cases, secretCases...)

	// Service cases
	serviceCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Service{
				ObjectMeta: getInitObjectMeta(),
				Spec: corev1.ServiceSpec{
					ClusterIP: "1.1.1.1",
				},
			},
			want: nil,
		},
	}
	cases = append(cases, serviceCases...)

	// ServiceAccount cases
	serviceAccountCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &corev1.ServiceAccount{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &corev1.ServiceAccount{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.ServiceAccount{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &corev1.ServiceAccount{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &corev1.ServiceAccount{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &corev1.ServiceAccount{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			want: nil,
		},
	}
	cases = append(cases, serviceAccountCases...)

	// ClusterRole cases
	clusterRoleCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &rbacv1.ClusterRole{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &rbacv1.ClusterRole{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRole{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &rbacv1.ClusterRole{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &rbacv1.ClusterRole{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &rbacv1.ClusterRole{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			want: nil,
		},
	}
	cases = append(cases, clusterRoleCases...)

	// ClusterRoleBinding cases
	clusterRoleBindingCases := []objCase{
		{
			name: "Create Simple case",
			initObj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			want: nil,
		},
		{
			name: "Create Alredy exist case",
			initObj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Create with Another Namespace case",
			initObj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: nil,
		},
		{
			name: "Create without Name case",
			initObj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{},
			},
			want: nameRequeriedError,
		},
		{
			name: "Update exist case",
			initObj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			want: nil,
		},
	}
	cases = append(cases, clusterRoleBindingCases...)

	// Not supported type case
	notSuppurtedcase := []objCase{
		{
			name: "Update exist case",
			initObj: &rbacv1.RoleBinding{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			want: k8s.NewNotSupportedError(&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "init",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			),
		},
	}
	cases = append(cases, notSuppurtedcase...)

	for _, tc := range cases {
		t.Run(tc.name, createOrUpdateResourceCase(tc.initObj, tc.obj, tc.want))
	}
}

func TestCreatePod(t *testing.T) {
	createPodCase := func(objInit, obj *corev1.Pod, want error) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()
			err := k8s.CreatePod(context.Background(), obj, cl)
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
			err: nil,
			// err: api_errors.NewAlreadyExists(
			// 	schema.GroupResource{
			// 		Group:    "",
			// 		Resource: "pods",
			// 	},
			// 	"init",
			// ),
		},
	}

	for _, tc := range podCases {
		t.Run(tc.name, createPodCase(initObj, tc.obj, tc.err))
	}
}

func TestGetPod(t *testing.T) {
	getPodCase := func(objInit, obj, wantPod *corev1.Pod, want error) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()
			result, err := k8s.GetPod(context.Background(), client.ObjectKeyFromObject(obj), cl)
			if result != nil {
				req.Equal(result.ObjectMeta, wantPod.ObjectMeta)
			}
			req.Equal(err, want)
		}
	}

	type podCase struct {
		name    string
		initObj *corev1.Pod
		obj     *corev1.Pod
		wantObj *corev1.Pod
		err     error
	}

	podCases := []podCase{
		{
			name: "Get not exist pod case",
			initObj: &corev1.Pod{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			wantObj: nil,
			err: api_errors.NewNotFound(
				schema.GroupResource{
					Group:    "",
					Resource: "pods",
				},
				"test",
			),
		},
		{
			name: "Get exist pod case",
			initObj: &corev1.Pod{
				ObjectMeta: getInitObjectMeta(),
			},
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

	for _, tc := range podCases {
		t.Run(tc.name, getPodCase(tc.initObj, tc.obj, tc.wantObj, tc.err))
	}
}

func TestDeletePod(t *testing.T) {
	deletePodCase := func(objInit, obj client.Object, want error) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()
			pod := obj.(*corev1.Pod)
			err := k8s.DeletePod(context.Background(), pod, cl)
			req.Equal(err, want)
		}
	}

	var cases []objCase

	// DeletePodcases
	deletePodCases := []objCase{
		{
			name: "Delete Simple case",
			initObj: &corev1.Pod{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Pod{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Delete not exist case",
			initObj: &corev1.Pod{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: api_errors.NewNotFound(
				schema.GroupResource{
					Group:    "",
					Resource: "pods",
				},
				"test",
			),
		},
	}
	cases = append(cases, deletePodCases...)

	for _, tc := range cases {
		t.Run(tc.name, deletePodCase(tc.initObj, tc.obj, tc.want))
	}
}

func TestGetSecret(t *testing.T) {
	getSecretCase := func(objInit, obj, wantSecret *corev1.Secret, want error) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()
			result, err := k8s.GetSecret(context.Background(), client.ObjectKeyFromObject(obj), cl)
			if result != nil {
				req.Equal(result.ObjectMeta, wantSecret.ObjectMeta)
			}
			req.Equal(err, want)
		}
	}

	type secretCase struct {
		name    string
		initObj *corev1.Secret
		obj     *corev1.Secret
		wantObj *corev1.Secret
		err     error
	}

	podCases := []secretCase{
		{
			name: "Get not exist secret case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
			},
			wantObj: nil,
			err: api_errors.NewNotFound(
				schema.GroupResource{
					Group:    "",
					Resource: "secrets",
				},
				"test",
			),
		},
		{
			name: "Get exist secret case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			wantObj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "init",
					Namespace:       "test-namespace",
					ResourceVersion: "999",
				},
			},
			err: nil,
		},
	}

	for _, tc := range podCases {
		t.Run(tc.name, getSecretCase(tc.initObj, tc.obj, tc.wantObj, tc.err))
	}
}

func TestDeleteSecret(t *testing.T) {
	deleteSecretCase := func(objInit, obj client.Object, want error) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			cl := fake.NewClientBuilder().WithObjects(objInit).Build()
			secret := obj.(*corev1.Secret)
			err := k8s.DeleteSecret(context.Background(), secret, cl)
			req.Equal(err, want)
		}
	}

	var cases []objCase

	// DeletePodcases
	deleteSecretCases := []objCase{
		{
			name: "Delete Simple case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			want: nil,
		},
		{
			name: "Delete not exist case",
			initObj: &corev1.Secret{
				ObjectMeta: getInitObjectMeta(),
			},
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace2",
				},
			},
			want: api_errors.NewNotFound(
				schema.GroupResource{
					Group:    "",
					Resource: "secrets",
				},
				"test",
			),
		},
	}
	cases = append(cases, deleteSecretCases...)

	for _, tc := range cases {
		t.Run(tc.name, deleteSecretCase(tc.initObj, tc.obj, tc.want))
	}
}

func TestUpdateStatus(t *testing.T) {
	updateStatusCase := func(objInit, obj client.Object, want error) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
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

	for _, tc := range testCases {
		t.Run(tc.name, updateStatusCase(tc.initObj, tc.updateObj, tc.err))
	}
}

func TestNamespaceNameToLabel(t *testing.T) {
	namespaceNameToLabelCase := func(in, want string) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()
			req := require.New(t)

			result := k8s.NamespaceNameToLabel(in)
			req.Equal(result, want)
		}
	}

	type testCase struct {
		name string
		in   string
		want string
	}

	testCases := []testCase{
		{
			name: "Simple case",
			in:   "test",
			want: "kubernetes.io/metadata.name=test",
		},
		{
			name: "Zero case",
			in:   "",
			want: "kubernetes.io/metadata.name=",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, namespaceNameToLabelCase(tc.in, tc.want))
	}
}
