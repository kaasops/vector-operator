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
	"io"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdateService(ctx context.Context, svc *corev1.Service, c client.Client) error {
	return reconcileService(ctx, svc, c)
}

func CreateOrUpdateSecret(ctx context.Context, secret *corev1.Secret, c client.Client) error {
	return reconcileSecret(ctx, secret, c)
}

func CreateOrUpdateDaemonSet(ctx context.Context, daemonSet *appsv1.DaemonSet, c client.Client) error {
	return reconcileDaemonSet(ctx, daemonSet, c)
}

func CreateOrUpdateStatefulSet(ctx context.Context, statefulSet *appsv1.StatefulSet, c client.Client) error {
	return reconcileStatefulSet(ctx, statefulSet, c)
}

func CreateOrUpdateServiceAccount(ctx context.Context, secret *corev1.ServiceAccount, c client.Client) error {
	return reconcileServiceAccount(ctx, secret, c)
}

func CreateOrUpdateClusterRole(ctx context.Context, secret *rbacv1.ClusterRole, c client.Client) error {
	return reconcileClusterRole(ctx, secret, c)
}

func CreateOrUpdateClusterRoleBinding(ctx context.Context, secret *rbacv1.ClusterRoleBinding, c client.Client) error {
	return reconcileClusterRoleBinding(ctx, secret, c)
}

func CreatePod(ctx context.Context, pod *corev1.Pod, c client.Client) error {
	err := c.Create(ctx, pod)
	if err != nil {
		return err
	}
	return nil
}

func GetPod(ctx context.Context, pod *corev1.Pod, c client.Client) (*corev1.Pod, error) {
	result := &corev1.Pod{}
	err := c.Get(ctx, client.ObjectKeyFromObject(pod), result)
	if err != nil {
		return nil, err
	}
	return result, nil
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

func GetPodLogs(ctx context.Context, pod *corev1.Pod, cs *kubernetes.Clientset) (string, error) {
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

func reconcileService(ctx context.Context, obj runtime.Object, c client.Client) error {

	existing := &corev1.Service{}
	desired := obj.(*corev1.Service)

	err := c.Create(ctx, desired)
	if errors.IsAlreadyExists(err) {
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepDerivative(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) ||
			!equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations) ||
			!equality.Semantic.DeepDerivative(desired.Spec, existing.Spec) {
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func reconcileSecret(ctx context.Context, obj runtime.Object, c client.Client) error {

	existing := &corev1.Secret{}
	desired := obj.(*corev1.Secret)

	err := c.Create(ctx, desired)
	if errors.IsAlreadyExists(err) {
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepDerivative(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) ||
			!equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations) ||
			!equality.Semantic.DeepDerivative(desired.Data, existing.Data) {
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Data = desired.Data
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func reconcileDaemonSet(ctx context.Context, obj runtime.Object, c client.Client) error {

	existing := &appsv1.DaemonSet{}
	desired := obj.(*appsv1.DaemonSet)

	err := c.Create(ctx, desired)
	if errors.IsAlreadyExists(err) {
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepDerivative(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) ||
			!equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations) ||
			!equality.Semantic.DeepDerivative(desired.Spec, existing.Spec) {
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func reconcileStatefulSet(ctx context.Context, obj runtime.Object, c client.Client) error {

	existing := &appsv1.StatefulSet{}
	desired := obj.(*appsv1.StatefulSet)

	err := c.Create(ctx, desired)
	if errors.IsAlreadyExists(err) {
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepDerivative(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) ||
			!equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations) ||
			!equality.Semantic.DeepDerivative(desired.Spec, existing.Spec) {
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Spec = desired.Spec
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func reconcileServiceAccount(ctx context.Context, obj runtime.Object, c client.Client) error {

	existing := &corev1.ServiceAccount{}
	desired := obj.(*corev1.ServiceAccount)

	err := c.Create(ctx, desired)
	if errors.IsAlreadyExists(err) {
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepDerivative(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) ||
			!equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations) {
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func reconcileClusterRole(ctx context.Context, obj runtime.Object, c client.Client) error {

	existing := &rbacv1.ClusterRole{}
	desired := obj.(*rbacv1.ClusterRole)

	err := c.Create(ctx, desired)
	if errors.IsAlreadyExists(err) {
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepDerivative(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) ||
			!equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations) ||
			!equality.Semantic.DeepDerivative(desired.Rules, existing.Rules) {
			existing.Labels = desired.Labels
			existing.Annotations = desired.Annotations
			existing.Rules = desired.Rules
			return c.Update(ctx, existing)
		}
		return nil
	}
	return err
}

func reconcileClusterRoleBinding(ctx context.Context, obj runtime.Object, c client.Client) error {

	existing := &rbacv1.ClusterRoleBinding{}
	desired := obj.(*rbacv1.ClusterRoleBinding)

	err := c.Create(ctx, desired)
	if errors.IsAlreadyExists(err) {
		err := c.Get(ctx, client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepDerivative(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) ||
			!equality.Semantic.DeepDerivative(desired.Annotations, existing.Annotations) ||
			!equality.Semantic.DeepDerivative(desired.RoleRef, existing.RoleRef) ||
			!equality.Semantic.DeepDerivative(desired.Subjects, existing.Subjects) {
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

func NamespaceNameToLabel(namespace string) string {
	return "kubernetes.io/metadata.name=" + namespace
}
