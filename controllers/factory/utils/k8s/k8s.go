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

package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrNotSupported = errors.New("Not Supported type for create or update kubernetes resource")
)

func NewNotSupportedError(obj client.Object) error {
	return fmt.Errorf("%w.\n %+v", ErrNotSupported, obj)
}

func CreateOrUpdateResource(ctx context.Context, obj client.Object, c client.Client) error {
	switch obj.(type) {
	case *appsv1.Deployment:
		return createOrUpdateDeployment(ctx, obj, c)
	case *appsv1.StatefulSet:
		return createOrUpdateStatefulSet(ctx, obj, c)
	case *appsv1.DaemonSet:
		return createOrUpdateDaemonSet(ctx, obj, c)
	case *corev1.Secret:
		return createOrUpdateSecret(ctx, obj, c)
	case *corev1.Service:
		return createOrUpdateService(ctx, obj, c)
	case *corev1.ServiceAccount:
		return createOrUpdateServiceAccount(ctx, obj, c)
	case *rbacv1.ClusterRole:
		return createOrUpdateClusterRole(ctx, obj, c)
	case *rbacv1.ClusterRoleBinding:
		return createOrUpdateClusterRoleBinding(ctx, obj, c)
	case *monitorv1.ServiceMonitor:
		return createOrUpdateServiceMonitor(ctx, obj, c)
	default:
		return NewNotSupportedError(obj)
	}
}

func createOrUpdateDeployment(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*appsv1.Deployment)

	// Create Deployment
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &appsv1.Deployment{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.Spec,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.Spec,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func createOrUpdateStatefulSet(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*appsv1.StatefulSet)

	// Create StatefulSet
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &appsv1.StatefulSet{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.Spec,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.Spec,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func createOrUpdateDaemonSet(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*appsv1.DaemonSet)

	// Create DaemonSet
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &appsv1.DaemonSet{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.Spec,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.Spec,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func createOrUpdateSecret(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*corev1.Secret)

	// Create Secret
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &corev1.Secret{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.Data,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.Data,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Data = desired.Data
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func createOrUpdateService(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*corev1.Service)

	// Create Service
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &corev1.Service{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.Spec,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.Spec,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func createOrUpdateServiceAccount(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*corev1.ServiceAccount)

	// Create ServiceAccount
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &corev1.ServiceAccount{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func createOrUpdateClusterRole(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*rbacv1.ClusterRole)

	// Create ClusterRole
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &rbacv1.ClusterRole{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.Rules,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.Rules,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Rules = desired.Rules
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func createOrUpdateClusterRoleBinding(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*rbacv1.ClusterRoleBinding)

	// Create ClusterRoleBinding
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &rbacv1.ClusterRoleBinding{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// Init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.RoleRef,
			desired.Subjects,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.RoleRef,
			existing.Subjects,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.RoleRef = desired.RoleRef
			existing.Subjects = desired.Subjects
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

//

func createOrUpdateServiceMonitor(ctx context.Context, obj runtime.Object, c client.Client) error {
	desired := obj.(*monitorv1.ServiceMonitor)

	// Create ServiceMonitor
	err := c.Create(ctx, desired)
	if api_errors.IsAlreadyExists(err) {
		// If alredy exist - compare with existed
		existing := &monitorv1.ServiceMonitor{}
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}

		// init Interface for compare
		desiredFields := []interface{}{
			desired.GetAnnotations(),
			desired.GetLabels(),
			desired.Spec,
		}
		existingFields := []interface{}{
			existing.GetAnnotations(),
			existing.GetLabels(),
			existing.Spec,
		}

		// Compare
		if !equality.Semantic.DeepDerivative(desiredFields, existingFields) {
			// Update if not equal
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

// Something else:

func CreatePod(ctx context.Context, pod *corev1.Pod, c client.Client) error {
	err := c.Create(ctx, pod)
	if api_errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func GetPod(ctx context.Context, namespacedName types.NamespacedName, c client.Client) (*corev1.Pod, error) {
	result := &corev1.Pod{}
	err := c.Get(ctx, namespacedName, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func FetchPod(ctx context.Context, pod *corev1.Pod, c client.Client) (*corev1.Pod, error) {
	err := c.Get(ctx, client.ObjectKeyFromObject(pod), pod)
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func DeletePod(ctx context.Context, pod *corev1.Pod, c client.Client) error {
	if err := c.Delete(ctx, pod); err != nil {
		return err
	}
	return nil
}

func GetSecret(ctx context.Context, namespacedName types.NamespacedName, c client.Client) (*corev1.Secret, error) {
	result := &corev1.Secret{}
	err := c.Get(ctx, namespacedName, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func DeleteSecret(ctx context.Context, secret *corev1.Secret, c client.Client) error {
	if err := c.Delete(ctx, secret); err != nil {
		return err
	}
	return nil
}

func GetPodLogs(ctx context.Context, pod *corev1.Pod, cs kubernetes.Interface) (string, error) {
	count := int64(100)
	podLogOptions := corev1.PodLogOptions{
		TailLines: &count,
	}

	req := cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOptions)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	str := buf.String()

	return str, nil
}

func UpdateStatus(ctx context.Context, obj client.Object, c client.Client) error {
	return c.Status().Update(ctx, obj)
}

func NamespaceNameToLabel(namespace string) string {
	return "kubernetes.io/metadata.name=" + namespace
}
