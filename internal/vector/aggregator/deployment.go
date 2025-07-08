package aggregator

import (
	"context"
	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

func (ctrl *Controller) ensureVectorAggregatorDeployment(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-deployment", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator Deployment")
	deployment := ctrl.createVectorAggregatorDeployment()
	if ctrl.globalConfigChanged() {
		// restart pods
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations[common.AnnotationRestartedAt] = time.Now().Format(time.RFC3339)
	}
	return k8s.CreateOrUpdateResource(ctx, deployment, ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorDeployment() *appsv1.Deployment {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()
	var initContainers []corev1.Container
	var containers []corev1.Container
	containers = append(containers, *ctrl.VectorAggregatorContainer())

	if ctrl.Spec.CompressConfigFile {
		initContainers = append(initContainers, *ctrl.ConfigReloaderInitContainer())
	}

	if ctrl.Spec.CompressConfigFile {
		containers = append(containers, *ctrl.ConfigReloaderSidecarContainer())
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Replicas: &ctrl.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
				Spec: corev1.PodSpec{
					ServiceAccountName: ctrl.getNameVectorAggregator(),
					Volumes:            ctrl.generateVectorAggregatorVolume(),
					SecurityContext:    ctrl.Spec.SecurityContext,
					ImagePullSecrets:   ctrl.Spec.ImagePullSecrets,
					Affinity:           ctrl.Spec.Affinity,
					RuntimeClassName:   ctrl.Spec.RuntimeClassName,
					SchedulerName:      ctrl.Spec.SchedulerName,
					Tolerations:        ctrl.Spec.Tolerations,
					PriorityClassName:  ctrl.Spec.PodSecurityPolicyName,
					HostNetwork:        ctrl.Spec.HostNetwork,
					HostAliases:        ctrl.Spec.HostAliases,
					InitContainers:     initContainers,
					Containers:         containers,
				},
			},
		},
	}

	return deployment
}

func (ctrl *Controller) VectorAggregatorContainer() *corev1.Container {
	container := &corev1.Container{
		Name:  ctrl.getNameVectorAggregator(),
		Image: ctrl.Spec.Image,
		Args:  []string{"--config-dir", "/etc/vector", "--watch-config"},
		Env:   ctrl.generateVectorAggregatorEnvs(),
		Ports: []corev1.ContainerPort{
			{
				Name:          "prom-exporter",
				ContainerPort: config.DefaultInternalMetricsSinkPort,
				Protocol:      "TCP",
			},
		},
		VolumeMounts:    ctrl.generateVectorAggregatorVolumeMounts(),
		ReadinessProbe:  ctrl.Spec.ReadinessProbe,
		LivenessProbe:   ctrl.Spec.LivenessProbe,
		Resources:       ctrl.Spec.Resources,
		SecurityContext: ctrl.Spec.ContainerSecurityContext,
		ImagePullPolicy: ctrl.Spec.ImagePullPolicy,
	}

	// Check if envFrom is provided and set it
	if len(ctrl.Spec.EnvFrom) > 0 {
		container.EnvFrom = ctrl.Spec.EnvFrom
	}

	return container
}

func (ctrl *Controller) ConfigReloaderInitContainer() *corev1.Container {
	return &corev1.Container{
		Name:            "init-config-reloader",
		Image:           ctrl.Spec.ConfigReloaderImage,
		ImagePullPolicy: ctrl.Spec.ImagePullPolicy,
		Resources:       ctrl.Spec.ConfigReloaderResources,
		SecurityContext: ctrl.Spec.ContainerSecurityContext,
		Args: []string{
			"--init-mode=true",
			"--volume-dir-archive=/tmp/archive",
			"--dir-for-unarchive=/etc/vector",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/etc/vector",
			},
			{
				Name:      "app-config-compress",
				MountPath: "/tmp/archive",
			},
		},
	}
}

func (ctrl *Controller) ConfigReloaderSidecarContainer() *corev1.Container {
	return &corev1.Container{
		Name:            "config-reloader",
		Image:           ctrl.Spec.ConfigReloaderImage,
		ImagePullPolicy: ctrl.Spec.ImagePullPolicy,
		Resources:       ctrl.Spec.ConfigReloaderResources,
		SecurityContext: ctrl.Spec.ContainerSecurityContext,
		Args: []string{
			"--init-mode=false",
			"--volume-dir-archive=/tmp/archive",
			"--dir-for-unarchive=/etc/vector",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/etc/vector",
			},
			{
				Name:      "app-config-compress",
				MountPath: "/tmp/archive",
			},
		},
	}
}

func (ctrl *Controller) generateVectorAggregatorVolume() []corev1.Volume {
	volume := ctrl.Spec.Volumes
	configVolumeSource := corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: ctrl.getNameVectorAggregator(),
		},
	}
	if ctrl.Spec.CompressConfigFile {
		configVolumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}

	}
	volume = append(volume, []corev1.Volume{
		{
			Name:         "config",
			VolumeSource: configVolumeSource,
		},
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: ctrl.Spec.DataDir,
				},
			},
		},
		{
			Name: "procfs",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/proc",
				},
			},
		},
		{
			Name: "sysfs",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys",
				},
			},
		},
	}...)

	if ctrl.Spec.CompressConfigFile {
		volume = append(volume, corev1.Volume{
			Name: "app-config-compress",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ctrl.getNameVectorAggregator(),
				},
			},
		})
	}

	return volume
}

func (ctrl *Controller) generateVectorAggregatorVolumeMounts() []corev1.VolumeMount {
	volumeMount := ctrl.Spec.VolumeMounts

	volumeMount = append(volumeMount, []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/etc/vector",
		},
		{
			Name:      "data",
			MountPath: "/vector-data-dir",
		},
		{
			Name:      "procfs",
			MountPath: "/host/proc",
		},
		{
			Name:      "sysfs",
			MountPath: "/host/sys",
		},
	}...)

	if ctrl.Spec.CompressConfigFile {
		volumeMount = append(volumeMount, []corev1.VolumeMount{
			{
				Name:      "app-config-compress",
				MountPath: "/tmp/archive",
			},
		}...)
	}

	return volumeMount
}

func (ctrl *Controller) generateVectorAggregatorEnvs() []corev1.EnvVar {
	envs := ctrl.Spec.Env

	envs = append(envs, []corev1.EnvVar{
		{
			Name: "VECTOR_SELF_NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "spec.nodeName",
				},
			},
		},
		{
			Name: "VECTOR_SELF_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
		{
			Name: "VECTOR_SELF_POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.namespace",
				},
			},
		},
		{
			Name:  "PROCFS_ROOT",
			Value: "/host/proc",
		},
		{
			Name:  "SYSFS_ROOT",
			Value: "/host/sys",
		},
	}...)

	return envs
}
