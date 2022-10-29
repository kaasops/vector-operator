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
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdateService(svc *corev1.Service, c client.Client) error {
	return reconcileService(svc, c)
}

func CreateOrUpdateSecret(secret *corev1.Secret, c client.Client) error {
	return reconcileSecret(secret, c)
}

func CreateOrUpdateDaemonSet(daemonSet *appsv1.DaemonSet, c client.Client) error {
	return reconcileDaemonSet(daemonSet, c)
}

func CreateOrUpdateStatefulSet(statefulSet *appsv1.StatefulSet, c client.Client) error {
	return reconcileStatefulSet(statefulSet, c)
}

func CreateOrUpdateServiceAccount(secret *corev1.ServiceAccount, c client.Client) error {
	return reconcileServiceAccount(secret, c)
}

func CreateOrUpdateClusterRole(secret *rbacv1.ClusterRole, c client.Client) error {
	return reconcileClusterRole(secret, c)
}

func CreateOrUpdateClusterRoleBinding(secret *rbacv1.ClusterRoleBinding, c client.Client) error {
	return reconcileClusterRoleBinding(secret, c)
}

func CreatePod(pod *corev1.Pod, c client.Client) error {
	err := c.Create(context.TODO(), pod)
	if err != nil {
		return err
	}
	return nil
}

func GetPod(pod *corev1.Pod, c client.Client) (*corev1.Pod, error) {
	result := &corev1.Pod{}
	err := c.Get(context.TODO(), client.ObjectKeyFromObject(pod), result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func GetPodLogs(pod *corev1.Pod, cs *kubernetes.Clientset) (string, error) {
	count := int64(100)
	podLogOptions := corev1.PodLogOptions{
		TailLines: &count,
	}

	req := cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOptions)
	podLogs, err := req.Stream(context.TODO())
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

func reconcileService(obj runtime.Object, c client.Client) error {

	existing := &corev1.Service{}
	desired := obj.(*corev1.Service)

	err := c.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Spec = desired.Spec
			existing.Labels = desired.Labels
			err := c.Update(context.TODO(), existing)
			return err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func reconcileSecret(obj runtime.Object, c client.Client) error {

	existing := &corev1.Secret{}
	desired := obj.(*corev1.Secret)

	err := c.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Data = desired.Data
			existing.Labels = desired.Labels
			return c.Update(context.TODO(), existing)
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func reconcileDaemonSet(obj runtime.Object, c client.Client) error {

	existing := &appsv1.DaemonSet{}
	desired := obj.(*appsv1.DaemonSet)

	err := c.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Spec = desired.Spec
			existing.Labels = desired.Labels
			return c.Update(context.TODO(), existing)
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func reconcileStatefulSet(obj runtime.Object, c client.Client) error {

	existing := &appsv1.StatefulSet{}
	desired := obj.(*appsv1.StatefulSet)

	err := c.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Spec = desired.Spec
			existing.Labels = desired.Labels
			return c.Update(context.TODO(), existing)
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func reconcileServiceAccount(obj runtime.Object, c client.Client) error {

	existing := &corev1.ServiceAccount{}
	desired := obj.(*corev1.ServiceAccount)

	err := c.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func reconcileClusterRole(obj runtime.Object, c client.Client) error {

	existing := &rbacv1.ClusterRole{}
	desired := obj.(*rbacv1.ClusterRole)

	err := c.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.Rules = desired.Rules
			return c.Update(context.TODO(), existing)
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func reconcileClusterRoleBinding(obj runtime.Object, c client.Client) error {

	existing := &rbacv1.ClusterRoleBinding{}
	desired := obj.(*rbacv1.ClusterRoleBinding)

	err := c.Create(context.TODO(), desired)
	if err != nil && errors.IsAlreadyExists(err) {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(desired), existing)
		if err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(existing, desired) {
			existing.RoleRef = desired.RoleRef
			existing.Subjects = desired.Subjects
			return c.Update(context.TODO(), existing)
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}
