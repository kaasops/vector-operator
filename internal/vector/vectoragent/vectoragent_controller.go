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
	"k8s.io/utils/ptr"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (ctrl *Controller) EnsureVectorAgent(ctx context.Context, configOnly bool) error {
	log := log.FromContext(ctx).WithValues("vector-agent", ctrl.Vector.Name)
	log.Info("start Reconcile Vector Agent")

	monitoringCRD, err := k8s.ResourceExists(ctrl.ClientSet.Discovery(), monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}

	if err := ctrl.ensureVectorAgentConfig(ctx); err != nil {
		return err
	}
	if !configOnly {
		if err := ctrl.ensureVectorAgentRBAC(ctx); err != nil {
			return err
		}

		if ctrl.Vector.Spec.Agent.Api.Enabled {
			if err := ctrl.ensureVectorAgentService(ctx); err != nil {
				return err
			}
		}

		if ctrl.Vector.Spec.Agent.InternalMetrics && monitoringCRD {
			if err := ctrl.ensureVectorAgentPodMonitor(ctx); err != nil {
				return err
			}
		}

		if err := ctrl.ensureVectorAgentDaemonSet(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (ctrl *Controller) ensureVectorAgentRBAC(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-rbac", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent RBAC")

	if err := ctrl.ensureVectorAgentServiceAccount(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureVectorAgentClusterRole(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureVectorAgentClusterRoleBinding(ctx); err != nil {
		return err
	}

	return nil
}

func (ctrl *Controller) ensureVectorAgentServiceAccount(ctx context.Context) error {
	vectorAgentServiceAccount := ctrl.createVectorAgentServiceAccount()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentServiceAccount, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentClusterRole(ctx context.Context) error {
	vectorAgentClusterRole := ctrl.createVectorAgentClusterRole()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentClusterRole, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentClusterRoleBinding(ctx context.Context) error {
	vectorAgentClusterRoleBinding := ctrl.createVectorAgentClusterRoleBinding()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentClusterRoleBinding, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentService(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-service", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent Service")

	vectorAgentService := ctrl.createVectorAgentService()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentService, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentConfig(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-secret", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent Secret")

	vectorAgentSecret, err := ctrl.createVectorAgentConfig(ctx)
	if err != nil {
		return err
	}

	return k8s.CreateOrUpdateResource(ctx, vectorAgentSecret, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentDaemonSet(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-daemon-set", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent DaemonSet")

	vectorAgentDaemonSet := ctrl.createVectorAgentDaemonSet()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentDaemonSet, ctrl.Client)
}

func (ctrl *Controller) ensureVectorAgentPodMonitor(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-agent-podmonitor", ctrl.Vector.Name)

	log.Info("start Reconcile Vector Agent PodMonitor")

	vectorAgentPodMonitor := ctrl.createVectorAgentPodMonitor()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentPodMonitor, ctrl.Client)
}

func (ctrl *Controller) labelsForVectorAgent() map[string]string {
	return map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector",
		k8s.ComponentLabelKey: "Agent",
		k8s.InstanceLabelKey:  ctrl.Vector.Name,
	}
}

func (ctrl *Controller) annotationsForVectorAgent() map[string]string {
	return ctrl.Vector.Spec.Agent.Annotations
}

func (ctrl *Controller) objectMetaVectorAgent(labels map[string]string, annotations map[string]string, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ctrl.Vector.Name + "-agent",
		Namespace:       namespace,
		Labels:          labels,
		Annotations:     annotations,
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
			BlockOwnerDeletion: ptr.To(true),
			Controller:         ptr.To(true),
		},
	}
}
