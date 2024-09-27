package aggregator

import (
	"context"
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Controller struct {
	client.Client
	VectorAggregator *vectorv1alpha1.VectorAggregator
	ConfigBytes      []byte
	Config           *config.VectorConfig
	ClientSet        *kubernetes.Clientset
}

func NewController(v *vectorv1alpha1.VectorAggregator, c client.Client, cs *kubernetes.Clientset) *Controller {
	ctrl := &Controller{
		Client:           c,
		VectorAggregator: v,
		ClientSet:        cs,
	}
	ctrl.setDefault()
	return ctrl
}

func (ctrl *Controller) EnsureVectorAggregator(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-aggregator", ctrl.VectorAggregator.Name)
	log.Info("start Reconcile Vector Aggregator")

	monitoringCRD, err := k8s.ResourceExists(ctrl.ClientSet.Discovery(), monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}

	if err := ctrl.ensureVectorAggregatorConfig(ctx); err != nil {
		return err
	}

	if err := ctrl.ensureVectorAggregatorRBAC(ctx); err != nil {
		return err
	}

	if err := ctrl.ensureVectorAggregatorService(ctx); err != nil {
		return err
	}

	if ctrl.VectorAggregator.Spec.InternalMetrics && monitoringCRD {
		if err := ctrl.ensureVectorAggregatorPodMonitor(ctx); err != nil {
			return err
		}
	}

	if err := ctrl.ensureVectorAggregatorDeployment(ctx); err != nil {
		return err
	}
	return nil
}

func (ctrl *Controller) DeleteVectorAggregator(ctx context.Context) error {
	if err := ctrl.deleteVectorAggregatorClusterRole(ctx); err != nil {
		return err
	}
	if err := ctrl.deleteVectorAggregatorClusterRoleBinding(ctx); err != nil {
		return err
	}
	return nil
}

func (ctrl *Controller) setDefault() {
	if ctrl.VectorAggregator.Spec.Image == "" {
		ctrl.VectorAggregator.Spec.Image = "timberio/vector:0.28.1-distroless-libc"
	}

	if ctrl.VectorAggregator.Spec.Resources.Requests == nil {
		ctrl.VectorAggregator.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.VectorAggregator.Spec.Resources.Limits == nil {
		ctrl.VectorAggregator.Spec.Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}

	if ctrl.VectorAggregator.Spec.DataDir == "" {
		ctrl.VectorAggregator.Spec.DataDir = "/var/lib/vector"
	}

	if ctrl.VectorAggregator.Spec.Volumes == nil {
		ctrl.VectorAggregator.Spec.Volumes = []corev1.Volume{
			{
				Name: "var-log",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/log/",
					},
				},
			},
			{
				Name: "journal",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/log/journal",
					},
				},
			},
			{
				Name: "var-lib",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/lib/",
					},
				},
			},
		}
	}

	if ctrl.VectorAggregator.Spec.ReadinessProbe == nil && ctrl.VectorAggregator.Spec.Api.Enabled && ctrl.VectorAggregator.Spec.Api.Healthcheck {
		ctrl.VectorAggregator.Spec.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.IntOrString{
						Type:   intstr.Type(0),
						IntVal: 8686,
					},
				},
			},
			PeriodSeconds:       20,
			InitialDelaySeconds: 15,
			TimeoutSeconds:      3,
			SuccessThreshold:    0,
			FailureThreshold:    0,
		}
	}
	if ctrl.VectorAggregator.Spec.LivenessProbe == nil && ctrl.VectorAggregator.Spec.Api.Enabled && ctrl.VectorAggregator.Spec.Api.Healthcheck {
		ctrl.VectorAggregator.Spec.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.IntOrString{
						Type:   intstr.Type(0),
						IntVal: 8686,
					},
				},
			},
			PeriodSeconds:       20,
			InitialDelaySeconds: 15,
			TimeoutSeconds:      3,
			SuccessThreshold:    0,
			FailureThreshold:    0,
		}
	}

	if ctrl.VectorAggregator.Spec.VolumeMounts == nil {
		ctrl.VectorAggregator.Spec.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "var-log",
				MountPath: "/var/log/",
			},
			{
				Name:      "journal",
				MountPath: "/run/log/journal",
			},
			{
				Name:      "var-lib",
				MountPath: "/var/lib/",
			},
		}
	}
	if ctrl.VectorAggregator.Spec.CompressConfigFile && ctrl.VectorAggregator.Spec.ConfigReloaderImage == "" {
		ctrl.VectorAggregator.Spec.ConfigReloaderImage = "docker.io/kaasops/config-reloader:v0.1.4"
	}
	if ctrl.VectorAggregator.Spec.CompressConfigFile && ctrl.VectorAggregator.Spec.ConfigReloaderResources.Requests == nil {
		ctrl.VectorAggregator.Spec.ConfigReloaderResources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.VectorAggregator.Spec.CompressConfigFile && ctrl.VectorAggregator.Spec.ConfigReloaderResources.Limits == nil {
		ctrl.VectorAggregator.Spec.ConfigReloaderResources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}
}

func (ctrl *Controller) SetSuccessStatus(ctx context.Context, hash *uint32) error {
	var status = true
	ctrl.VectorAggregator.Status.ConfigCheckResult = &status
	ctrl.VectorAggregator.Status.Reason = nil
	ctrl.VectorAggregator.Status.LastAppliedConfigHash = hash
	return k8s.UpdateStatus(ctx, ctrl.VectorAggregator, ctrl.Client)
}

func (ctrl *Controller) SetFailedStatus(ctx context.Context, reason string) error {
	var status = false
	ctrl.VectorAggregator.Status.ConfigCheckResult = &status
	ctrl.VectorAggregator.Status.Reason = &reason
	return k8s.UpdateStatus(ctx, ctrl.VectorAggregator, ctrl.Client)
}

func (ctrl *Controller) labelsForVectorAggregator() map[string]string {
	return map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector",
		k8s.ComponentLabelKey: "Aggregator",
		k8s.InstanceLabelKey:  ctrl.VectorAggregator.Name,
	}
}

func (ctrl *Controller) annotationsForVectorAggregator() map[string]string {
	return ctrl.VectorAggregator.Spec.Annotations
}

func (ctrl *Controller) objectMetaVectorAggregator(labels map[string]string, annotations map[string]string, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ctrl.getNameVectorAggregator(),
		Namespace:       namespace,
		Labels:          labels,
		Annotations:     annotations,
		OwnerReferences: ctrl.getControllerReference(),
	}
}

func (ctrl *Controller) getNameVectorAggregator() string {
	name := ctrl.VectorAggregator.Name + "-aggregator"
	return name
}

func (ctrl *Controller) getControllerReference() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         ctrl.VectorAggregator.APIVersion,
			Kind:               ctrl.VectorAggregator.Kind,
			Name:               ctrl.VectorAggregator.GetName(),
			UID:                ctrl.VectorAggregator.GetUID(),
			BlockOwnerDeletion: ptr.To(true),
			Controller:         ptr.To(true),
		},
	}
}

func (ctrl *Controller) GetServiceName() string {
	return ctrl.getNameVectorAggregator()
}
