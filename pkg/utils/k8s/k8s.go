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

	victoriametricsv1beta1 "github.com/VictoriaMetrics/operator/api/v1beta1"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	ErrNotSupported = errors.New("not Supported type for create or update kubernetes resource")
)

func NewNotSupportedError(obj client.Object) error {
	return fmt.Errorf("%w.\n %+v", ErrNotSupported, obj)
}

func CreateOrUpdateResource(ctx context.Context, obj client.Object, c client.Client) error {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return createOrUpdateDeployment(ctx, o, c)
	case *appsv1.StatefulSet:
		return createOrUpdateStatefulSet(ctx, o, c)
	case *appsv1.DaemonSet:
		return createOrUpdateDaemonSet(ctx, o, c)
	case *corev1.Secret:
		return createOrUpdateSecret(ctx, o, c)
	case *corev1.Service:
		return createOrUpdateService(ctx, o, c)
	case *corev1.ServiceAccount:
		return createOrUpdateServiceAccount(ctx, o, c)
	case *rbacv1.ClusterRole:
		return createOrUpdateClusterRole(ctx, o, c)
	case *rbacv1.ClusterRoleBinding:
		return createOrUpdateClusterRoleBinding(ctx, o, c)
	case *monitorv1.PodMonitor:
		return createOrUpdatePodMonitor(ctx, o, c)
	case *victoriametricsv1beta1.VMPodScrape:
		return createOrUpdatePodSrape(ctx, o, c)
	default:
		return NewNotSupportedError(o)
	}
}

func createOrUpdateDeployment(ctx context.Context, desired *appsv1.Deployment, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update Deployment: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdateStatefulSet(ctx context.Context, desired *appsv1.StatefulSet, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update StatefulSet: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdateDaemonSet(ctx context.Context, desired *appsv1.DaemonSet, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update Daemonset: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdateSecret(ctx context.Context, desired *corev1.Secret, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Data = desired.Data
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update Secret: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdateService(ctx context.Context, desired *corev1.Service, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update Deployment: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdateServiceAccount(ctx context.Context, desired *corev1.ServiceAccount, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update ServiceAccount: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdateClusterRole(ctx context.Context, desired *rbacv1.ClusterRole, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Rules = desired.Rules
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update ClusterRole: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdateClusterRoleBinding(ctx context.Context, desired *rbacv1.ClusterRoleBinding, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.RoleRef = desired.RoleRef
		existing.Subjects = desired.Subjects
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update ClusterRoleBinding: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdatePodMonitor(ctx context.Context, desired *monitorv1.PodMonitor, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update PodMonitor: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
}

func createOrUpdatePodSrape(ctx context.Context, desired *victoriametricsv1beta1.VMPodScrape, c client.Client) error {
	existing := desired.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		existing.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update VMPodScrape: %w", err)
	}
	existing.DeepCopyInto(desired)
	return nil
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

func ListSecret(ctx context.Context, c client.Client, listOpts client.ListOptions) ([]corev1.Secret, error) {
	secretList := corev1.SecretList{}
	err := c.List(ctx, &secretList, &listOpts)
	if err != nil {
		return nil, err
	}
	return secretList.Items, nil
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

// ResourceExists returns true if the given resource kind exists
// in the given api groupversion
func ResourceExists(dc discovery.DiscoveryInterface, apiGroupVersion, kind string) (bool, error) {
	apiList, err := dc.ServerResourcesForGroupVersion(apiGroupVersion)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	for _, r := range apiList.APIResources {
		if r.Kind == kind {
			return true, nil
		}
	}

	return false, nil
}
