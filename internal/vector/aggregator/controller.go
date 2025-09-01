package aggregator

import (
	"context"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/buildinfo"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Aggregator interface {
	client.Object
}

type Controller struct {
	client.Client
	id                  string
	Name                string
	Namespace           string
	VectorAggregator    Aggregator
	APIVersion          string
	Kind                string
	Spec                *vectorv1alpha1.VectorAggregatorCommon
	Status              *vectorv1alpha1.VectorCommonStatus
	ConfigBytes         []byte
	Config              *config.VectorConfig
	ClientSet           *kubernetes.Clientset
	isClusterAggregator bool
}

func NewController(
	v Aggregator,
	c client.Client,
	cs *kubernetes.Clientset,
) *Controller {
	ctrl := &Controller{
		Client:           c,
		VectorAggregator: v,
		ClientSet:        cs,
	}

	switch agg := v.(type) {
	case *vectorv1alpha1.VectorAggregator:
		ctrl.isClusterAggregator = false
		ctrl.Spec = &agg.Spec.VectorAggregatorCommon
		ctrl.Name = agg.Name
		ctrl.Namespace = agg.Namespace
		ctrl.Status = &agg.Status.VectorCommonStatus
		ctrl.APIVersion = agg.APIVersion
		ctrl.Kind = agg.Kind
		ctrl.id = types.NamespacedName{Name: agg.Name, Namespace: agg.Namespace}.String()
	case *vectorv1alpha1.ClusterVectorAggregator:
		ctrl.isClusterAggregator = true
		ctrl.Spec = &agg.Spec.VectorAggregatorCommon
		ctrl.Name = agg.Name
		ctrl.Namespace = agg.Spec.ResourceNamespace
		ctrl.Status = &agg.Status.VectorCommonStatus
		ctrl.APIVersion = agg.APIVersion
		ctrl.Kind = agg.Kind
		ctrl.id = types.NamespacedName{Name: agg.Name}.String()
	}

	ctrl.setDefault()
	return ctrl
}

func (ctrl *Controller) EnsureVectorAggregator(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator")

	monitoringCRD, err := k8s.ResourceExists(ctrl.ClientSet.Discovery(), monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}

	if err = ctrl.ensureVectorAggregatorConfig(ctx); err != nil {
		return err
	}

	if err := ctrl.ensureVectorAggregatorRBAC(ctx); err != nil {
		return err
	}

	if err := ctrl.ensureVectorAggregatorService(ctx); err != nil {
		return err
	}

	if ctrl.Spec.InternalMetrics && monitoringCRD {
		if err := ctrl.ensureVectorAggregatorPodMonitor(ctx); err != nil {
			return err
		}
	}

	if err := ctrl.ensureVectorAggregatorDeployment(ctx); err != nil {
		return err
	}

	if err := ctrl.ensureEventCollector(ctx); err != nil {
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
	if ctrl.Spec.Image == "" {
		ctrl.Spec.Image = "timberio/vector:0.48.0-distroless-libc"
	}

	if ctrl.Spec.Resources.Requests == nil {
		ctrl.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.Spec.Resources.Limits == nil {
		ctrl.Spec.Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}

	if ctrl.Spec.DataDir == "" {
		ctrl.Spec.DataDir = "/var/lib/vector"
	}

	if ctrl.Spec.Volumes == nil {
		ctrl.Spec.Volumes = []corev1.Volume{
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

	if ctrl.Spec.ReadinessProbe == nil && ctrl.Spec.Api.Enabled && ctrl.Spec.Api.Healthcheck {
		ctrl.Spec.ReadinessProbe = &corev1.Probe{
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
	if ctrl.Spec.LivenessProbe == nil && ctrl.Spec.Api.Enabled && ctrl.Spec.Api.Healthcheck {
		ctrl.Spec.LivenessProbe = &corev1.Probe{
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

	if ctrl.Spec.VolumeMounts == nil {
		ctrl.Spec.VolumeMounts = []corev1.VolumeMount{
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
	if ctrl.Spec.CompressConfigFile && ctrl.Spec.ConfigReloaderImage == "" {
		ctrl.Spec.ConfigReloaderImage = "docker.io/kaasops/config-reloader:v0.1.4"
	}
	if ctrl.Spec.CompressConfigFile && ctrl.Spec.ConfigReloaderResources.Requests == nil {
		ctrl.Spec.ConfigReloaderResources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.Spec.CompressConfigFile && ctrl.Spec.ConfigReloaderResources.Limits == nil {
		ctrl.Spec.ConfigReloaderResources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}
	if ctrl.Spec.EventCollector.Image == "" {
		ctrl.Spec.EventCollector.Image = "kaasops/event-collector:" + buildinfo.Version
	}
	if ctrl.Spec.EventCollector.ImagePullPolicy == "" {
		ctrl.Spec.EventCollector.ImagePullPolicy = corev1.PullIfNotPresent
	}
	if ctrl.Spec.EventCollector.MaxBatchSize <= 0 {
		ctrl.Spec.EventCollector.MaxBatchSize = 250
	}
}

func (ctrl *Controller) SetSuccessStatus(ctx context.Context, hash, globCfgHash *uint32) error {
	var status = true
	ctrl.Status.ConfigCheckResult = &status
	ctrl.Status.Reason = nil
	ctrl.Status.LastAppliedConfigHash = hash
	ctrl.Status.LastAppliedGlobalConfigHash = globCfgHash
	return k8s.UpdateStatus(ctx, ctrl.VectorAggregator, ctrl.Client)
}

func (ctrl *Controller) SetFailedStatus(ctx context.Context, reason string) error {
	var status = false
	ctrl.Status.ConfigCheckResult = &status
	ctrl.Status.Reason = &reason
	return k8s.UpdateStatus(ctx, ctrl.VectorAggregator, ctrl.Client)
}

func (ctrl *Controller) matchLabelsForVectorAggregator() map[string]string {
	return map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector",
		k8s.ComponentLabelKey: "Aggregator",
		k8s.InstanceLabelKey:  ctrl.Name,
	}
}

func (ctrl *Controller) labelsForVectorAggregator() map[string]string {
	basicLabels := ctrl.matchLabelsForVectorAggregator()

	labels := k8s.MergeLabels(basicLabels, ctrl.Spec.Labels)

	return labels
}

func (ctrl *Controller) annotationsForVectorAggregator() map[string]string {
	return ctrl.Spec.Annotations
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
	name := ctrl.Name + "-aggregator"
	return name
}

func (ctrl *Controller) getControllerReference() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         ctrl.APIVersion,
			Kind:               ctrl.Kind,
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

func (ctrl *Controller) prefix() string {
	if ctrl.isClusterAggregator {
		return "cluster-"
	}
	return ""
}

func (ctrl *Controller) globalConfigChanged() bool {
	globalCfgHash := ctrl.Config.GetGlobalConfigHash()
	if ctrl.Status.LastAppliedGlobalConfigHash == nil {
		return false
	}
	return *ctrl.Status.LastAppliedGlobalConfigHash != *globalCfgHash
}
