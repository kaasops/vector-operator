package aggregator

import (
	"context"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (ctrl *Controller) ensureVectorAggregatorDeployment(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-aggregator-deployment", ctrl.VectorAggregator.Name)
	log.Info("start Reconcile Vector Aggregator Deployment")
	return k8s.CreateOrUpdateResource(ctx, ctrl.createVectorAggregatorDeployment(), ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorDeployment() *appsv1.Deployment {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()
	var initContainers []corev1.Container
	var containers []corev1.Container
	containers = append(containers, *ctrl.VectorAggregatorContainer())

	if ctrl.VectorAggregator.Spec.CompressConfigFile {
		initContainers = append(initContainers, *ctrl.ConfigReloaderInitContainer())
	}

	if ctrl.VectorAggregator.Spec.CompressConfigFile {
		containers = append(containers, *ctrl.ConfigReloaderSidecarContainer())
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.VectorAggregator.Namespace),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Replicas: &ctrl.VectorAggregator.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.VectorAggregator.Namespace),
				Spec: corev1.PodSpec{
					ServiceAccountName: ctrl.getNameVectorAggregator(),
					Volumes:            ctrl.generateVectorAggregatorVolume(),
					SecurityContext:    ctrl.VectorAggregator.Spec.SecurityContext,
					ImagePullSecrets:   ctrl.VectorAggregator.Spec.ImagePullSecrets,
					Affinity:           ctrl.VectorAggregator.Spec.Affinity,
					RuntimeClassName:   ctrl.VectorAggregator.Spec.RuntimeClassName,
					SchedulerName:      ctrl.VectorAggregator.Spec.SchedulerName,
					Tolerations:        ctrl.VectorAggregator.Spec.Tolerations,
					PriorityClassName:  ctrl.VectorAggregator.Spec.PodSecurityPolicyName,
					HostNetwork:        ctrl.VectorAggregator.Spec.HostNetwork,
					HostAliases:        ctrl.VectorAggregator.Spec.HostAliases,
					InitContainers:     initContainers,
					Containers:         containers,
				},
			},
		},
	}

	return deployment
}

func (ctrl *Controller) VectorAggregatorContainer() *corev1.Container {
	return &corev1.Container{
		Name:  ctrl.getNameVectorAggregator(),
		Image: ctrl.VectorAggregator.Spec.Image,
		Args:  []string{"--config-dir", "/etc/vector", "--watch-config"},
		Env:   ctrl.generateVectorAggregatorEnvs(),
		Ports: []corev1.ContainerPort{
			{
				Name:          "prom-exporter",
				ContainerPort: 9598,
				Protocol:      "TCP",
			},
		},
		VolumeMounts:    ctrl.generateVectorAggregatorVolumeMounts(),
		ReadinessProbe:  ctrl.VectorAggregator.Spec.ReadinessProbe,
		LivenessProbe:   ctrl.VectorAggregator.Spec.LivenessProbe,
		Resources:       ctrl.VectorAggregator.Spec.Resources,
		SecurityContext: ctrl.VectorAggregator.Spec.ContainerSecurityContext,
		ImagePullPolicy: ctrl.VectorAggregator.Spec.ImagePullPolicy,
	}
}

func (ctrl *Controller) ConfigReloaderInitContainer() *corev1.Container {
	return &corev1.Container{
		Name:            "init-config-reloader",
		Image:           ctrl.VectorAggregator.Spec.ConfigReloaderImage,
		ImagePullPolicy: ctrl.VectorAggregator.Spec.ImagePullPolicy,
		Resources:       ctrl.VectorAggregator.Spec.ConfigReloaderResources,
		SecurityContext: ctrl.VectorAggregator.Spec.ContainerSecurityContext,
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
		Image:           ctrl.VectorAggregator.Spec.ConfigReloaderImage,
		ImagePullPolicy: ctrl.VectorAggregator.Spec.ImagePullPolicy,
		Resources:       ctrl.VectorAggregator.Spec.ConfigReloaderResources,
		SecurityContext: ctrl.VectorAggregator.Spec.ContainerSecurityContext,
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
	volume := ctrl.VectorAggregator.Spec.Volumes
	configVolumeSource := corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: ctrl.getNameVectorAggregator(),
		},
	}
	if ctrl.VectorAggregator.Spec.CompressConfigFile {
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
					Path: ctrl.VectorAggregator.Spec.DataDir,
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

	if ctrl.VectorAggregator.Spec.CompressConfigFile {
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
	volumeMount := ctrl.VectorAggregator.Spec.VolumeMounts

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

	if ctrl.VectorAggregator.Spec.CompressConfigFile {
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
	envs := ctrl.VectorAggregator.Spec.Env

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
