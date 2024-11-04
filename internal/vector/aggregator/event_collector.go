package aggregator

import (
	"context"
	"encoding/json"
	"github.com/kaasops/vector-operator/internal/evcollector"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (ctrl *Controller) ensureEventCollector(ctx context.Context) error {
	cfg := ctrl.Config.GetEventCollectorConfig(ctrl.Namespace)
	if cfg == nil {
		return ctrl.cleanupEventCollector(ctx)
	}
	cfg.MaxBatchSize = ctrl.Spec.EventCollector.MaxBatchSize

	// config
	eventCollectorConfig, err := ctrl.createEventCollectorConfig(cfg)
	if err != nil {
		return err
	}

	err = k8s.CreateOrUpdateResource(ctx, eventCollectorConfig, ctrl.Client)
	if err != nil {
		return err
	}

	// rbac
	if err := ctrl.ensureEventCollectorRBAC(ctx); err != nil {
		return err
	}

	// service
	if err := k8s.CreateOrUpdateResource(ctx, ctrl.createEventCollectorService(), ctrl.Client); err != nil {
		return err
	}

	// deployment
	if err := k8s.CreateOrUpdateResource(ctx, ctrl.createEventCollectorDeployment(), ctrl.Client); err != nil {
		return err
	}

	return nil
}

func (ctrl *Controller) cleanupEventCollector(ctx context.Context) error {
	if err := ctrl.Delete(ctx, ctrl.createEventCollectorDeployment()); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	if err := ctrl.Delete(ctx, ctrl.createEventCollectorService()); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	if err := ctrl.Delete(ctx, ctrl.createEventCollectorClusterRoleBinding()); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	if err := ctrl.Delete(ctx, ctrl.createEventCollectorClusterRole()); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	if err := ctrl.Delete(ctx, ctrl.createEventCollectorServiceAccount()); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	cfg, err := ctrl.createEventCollectorConfig(nil)
	if err != nil {
		return err
	}
	if err := ctrl.Delete(ctx, cfg); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (ctrl *Controller) createEventCollectorService() *corev1.Service {
	labels := ctrl.labelsForEventCollector()
	annotations := ctrl.annotationsForVectorAggregator()
	svc := &corev1.Service{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Protocol:   corev1.ProtocolTCP,
					Port:       8080,
					TargetPort: intstr.FromInt32(8080),
				},
			},
			Selector: labels,
		},
	}
	svc.ObjectMeta.Name = ctrl.Name + "-event-collector"
	return svc
}

func (ctrl *Controller) createEventCollectorConfig(params *evcollector.Config) (*corev1.ConfigMap, error) {
	labels := ctrl.labelsForEventCollector()
	annotations := ctrl.annotationsForVectorAggregator()
	bytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	config := map[string]string{
		"config.json": string(bytes),
	}
	cfg := &corev1.ConfigMap{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Data:       config,
	}
	cfg.ObjectMeta.Name = ctrl.Name + "-event-collector"
	return cfg, nil
}

func (ctrl *Controller) createEventCollectorDeployment() *appsv1.Deployment {
	labels := ctrl.labelsForEventCollector()
	annotations := ctrl.annotationsForVectorAggregator()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["prometheus.io/scrape"] = "true"
	annotations["prometheus.io/port"] = "8080"
	containers := []corev1.Container{*ctrl.eventCollectorContainer()}

	deployment := &appsv1.Deployment{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Replicas: ptr.To[int32](1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
				Spec: corev1.PodSpec{
					ServiceAccountName: ctrl.Name + "-event-collector",
					Volumes:            ctrl.generateEventCollectorVolume(),
					SecurityContext:    ctrl.Spec.SecurityContext,
					ImagePullSecrets:   ctrl.Spec.ImagePullSecrets,
					Affinity:           ctrl.Spec.Affinity,
					RuntimeClassName:   ctrl.Spec.RuntimeClassName,
					SchedulerName:      ctrl.Spec.SchedulerName,
					Tolerations:        ctrl.Spec.Tolerations,
					PriorityClassName:  ctrl.Spec.PodSecurityPolicyName,
					HostNetwork:        ctrl.Spec.HostNetwork,
					HostAliases:        ctrl.Spec.HostAliases,
					Containers:         containers,
				},
			},
		},
	}
	deployment.ObjectMeta.Name = ctrl.Name + "-event-collector"
	return deployment
}

func (ctrl *Controller) eventCollectorContainer() *corev1.Container {
	return &corev1.Container{
		Name:            "event-collector",
		Image:           ctrl.Spec.EventCollector.Image,
		ImagePullPolicy: ctrl.Spec.EventCollector.ImagePullPolicy,
		SecurityContext: ctrl.Spec.ContainerSecurityContext,
		Args:            []string{},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "event-collector-config",
				MountPath: "/etc/event-collector",
			},
		},
	}
}

func (ctrl *Controller) generateEventCollectorVolume() []corev1.Volume {
	return append(ctrl.Spec.Volumes, corev1.Volume{
		Name: "event-collector-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ctrl.Name + "-event-collector",
				},
			},
		},
	})
}

// rbac

func (ctrl *Controller) ensureEventCollectorRBAC(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-rbac", ctrl.Name)

	log.Info("start Reconcile Vector Aggregator RBAC")

	if err := ctrl.ensureEventCollectorServiceAccount(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureEventCollectorClusterRole(ctx); err != nil {
		return err
	}
	if err := ctrl.ensureEventCollectorClusterRoleBinding(ctx); err != nil {
		return err
	}
	return nil
}

func (ctrl *Controller) ensureEventCollectorServiceAccount(ctx context.Context) error {
	return k8s.CreateOrUpdateResource(ctx, ctrl.createEventCollectorServiceAccount(), ctrl.Client)
}

func (ctrl *Controller) ensureEventCollectorClusterRole(ctx context.Context) error {
	return k8s.CreateOrUpdateResource(ctx, ctrl.createEventCollectorClusterRole(), ctrl.Client)
}

func (ctrl *Controller) ensureEventCollectorClusterRoleBinding(ctx context.Context) error {
	return k8s.CreateOrUpdateResource(ctx, ctrl.createEventCollectorClusterRoleBinding(), ctrl.Client)
}

func (ctrl *Controller) createEventCollectorClusterRole() *rbacv1.ClusterRole {
	labels := ctrl.labelsForEventCollector()
	annotations := ctrl.annotationsForVectorAggregator()

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ""),
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"list", "watch"},
			},
		},
	}

	clusterRole.ObjectMeta.Name = ctrl.Name + "-event-collector"
	return clusterRole
}

func (ctrl *Controller) createEventCollectorServiceAccount() *corev1.ServiceAccount {
	labels := ctrl.labelsForEventCollector()
	annotations := ctrl.annotationsForVectorAggregator()

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
	}

	serviceAccount.ObjectMeta.Name = ctrl.Name + "-event-collector"

	return serviceAccount
}

func (ctrl *Controller) createEventCollectorClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	labels := ctrl.labelsForEventCollector()
	annotations := ctrl.annotationsForVectorAggregator()

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ""),
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     ctrl.Name + "-event-collector",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrl.Name + "-event-collector",
				Namespace: ctrl.Namespace,
			},
		},
	}
	clusterRoleBinding.ObjectMeta.Name = ctrl.Name + "-event-collector"
	return clusterRoleBinding
}

func (ctrl *Controller) labelsForEventCollector() map[string]string {
	return map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector",
		k8s.ComponentLabelKey: "EventCollector",
		k8s.InstanceLabelKey:  ctrl.Name,
	}
}
