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
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers/factory/helper"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"github.com/kaasops/vector-operator/controllers/factory/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Controller struct {
	client.Client
	Vector *vectorv1alpha1.Vector

	// Temp. Wait this issue - https://github.com/kubernetes-sigs/controller-runtime/issues/452
	Clientset *kubernetes.Clientset
}

func NewController(v *vectorv1alpha1.Vector, c client.Client, cs *kubernetes.Clientset) *Controller {
	return &Controller{
		Client:    c,
		Vector:    v,
		Clientset: cs,
	}
}

func (ctrl *Controller) EnsureVectorAgent() (done bool, result ctrl.Result, err error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent")

	if done, result, err = ctrl.ensureVectorAgentRBAC(); done {
		return
	}

	if ctrl.Vector.Spec.Agent.Service {
		if done, result, err = ctrl.ensureVectorAgentService(); done {
			return
		}
	}

	if done, result, err = ctrl.ensureVectorAgentConfig(); done {
		return
	}

	if done, result, err = ctrl.ensureVectorAgentDaemonSet(); done {
		return
	}

	return
}

func (ctrl *Controller) ensureVectorAgentRBAC() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent RBAC")

	if done, _, err := ctrl.ensureVectorAgentServiceAccount(); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := ctrl.ensureVectorAgentClusterRole(); done {
		return helper.ReconcileResult(err)
	}
	if done, _, err := ctrl.ensureVectorAgentClusterRoleBinding(); done {
		return helper.ReconcileResult(err)
	}

	return helper.ReconcileResult(nil)
}

func (ctrl *Controller) ensureVectorAgentServiceAccount() (bool, ctrl.Result, error) {
	vectorAgentServiceAccount := ctrl.createVectorAgentServiceAccount()

	_, err := k8sutils.CreateOrUpdateServiceAccount(vectorAgentServiceAccount, ctrl.Client)

	return helper.ReconcileResult(err)
}

func (ctrl *Controller) ensureVectorAgentClusterRole() (bool, ctrl.Result, error) {
	vectorAgentClusterRole := ctrl.createVectorAgentClusterRole()

	_, err := k8sutils.CreateOrUpdateClusterRole(vectorAgentClusterRole, ctrl.Client)

	return helper.ReconcileResult(err)
}

func (ctrl *Controller) ensureVectorAgentClusterRoleBinding() (bool, ctrl.Result, error) {
	vectorAgentClusterRoleBinding := ctrl.createVectorAgentClusterRoleBinding()

	_, err := k8sutils.CreateOrUpdateClusterRoleBinding(vectorAgentClusterRoleBinding, ctrl.Client)

	return helper.ReconcileResult(err)
}

func (ctrl *Controller) ensureVectorAgentService() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-service", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent Service")

	vectorAgentService := ctrl.createVectorAgentService()

	_, err := k8sutils.CreateOrUpdateService(vectorAgentService, ctrl.Client)

	return helper.ReconcileResult(err)
}

func (ctrl *Controller) ensureVectorAgentConfig() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-secret", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent Secret")

	vectorAgentSecret, err := ctrl.createVectorAgentConfig(ctx)
	if err != nil {
		return helper.ReconcileResult(err)
	}

	_, err = k8sutils.CreateOrUpdateSecret(vectorAgentSecret, ctrl.Client)

	return helper.ReconcileResult(err)
}

func (ctrl *Controller) ensureVectorAgentDaemonSet() (bool, ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("vector-agent-daemon-set", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent DaemonSet")

	vectorAgentDaemonSet := ctrl.createVectorAgentDaemonSet()

	_, err := k8sutils.CreateOrUpdateDaemonSet(vectorAgentDaemonSet, ctrl.Client)

	return helper.ReconcileResult(err)
}

func (ctrl *Controller) labelsForVectorAgent() map[string]string {
	return map[string]string{
		label.ManagedByLabelKey:  "vector-operator",
		label.NameLabelKey:       "vector",
		label.ComponentLabelKey:  "Agent",
		label.InstanceLabelKey:   ctrl.Vector.Name,
		label.VectorExcludeLabel: "true",
	}
}

func (ctrl *Controller) objectMetaVectorAgent(labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ctrl.Vector.Name + "-agent",
		Namespace:       ctrl.Vector.Namespace,
		Labels:          labels,
		OwnerReferences: ctrl.getControllerReference(),
	}
}

func (ctrl *Controller) getNameVectorAgent() string {
	name := ctrl.Vector.Name + "-agent"
	return name
}

func (ctrl *Controller) getControllerReference() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         ctrl.Vector.APIVersion,
			Kind:               ctrl.Vector.Kind,
			Name:               ctrl.Vector.GetName(),
			UID:                ctrl.Vector.GetUID(),
			BlockOwnerDeletion: pointer.BoolPtr(true),
			Controller:         pointer.BoolPtr(true),
		},
	}
}

func setSucceesStatus(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client) error {
	var status = true
	v.Status.ConfigCheckResult = &status
	v.Status.Reason = nil

	return k8sutils.UpdateStatus(ctx, v, c)
}

func setFailedStatus(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client, err error) error {
	var status = false
	var reason = err.Error()
	v.Status.ConfigCheckResult = &status
	v.Status.Reason = &reason

	return k8sutils.UpdateStatus(ctx, v, c)
}

func SetLastAppliedPipelineStatus(ctx context.Context, v *vectorv1alpha1.Vector, c client.Client, hash *uint32) error {

	v.Status.LastAppliedConfigHash = hash
	if err := k8sutils.UpdateStatus(ctx, v, c); err != nil {
		return err
	}
	return nil
}
