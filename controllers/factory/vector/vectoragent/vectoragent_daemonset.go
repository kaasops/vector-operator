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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (ctrl *Controller) createVectorAgentDaemonSet() *appsv1.DaemonSet {
	labels := ctrl.labelsForVectorAgent()
	var initContainers []corev1.Container
	var containers []corev1.Container
	containers = append(containers, *ctrl.VectorAgentContainer())

	if ctrl.Vector.Spec.Agent.CompressConfigFile {
		initContainers = append(initContainers, *ctrl.ConfigReloaderInitContainer())
	}

	if ctrl.Vector.Spec.Agent.CompressConfigFile {
		containers = append(containers, *ctrl.ConfigReloaderSidecarContainer())
	}

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: ctrl.objectMetaVectorAgent(labels),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: ctrl.objectMetaVectorAgent(labels),
				Spec: corev1.PodSpec{
					ServiceAccountName: ctrl.getNameVectorAgent(),
					Volumes:            ctrl.generateVectorAgentVolume(),
					SecurityContext:    ctrl.Vector.Spec.Agent.SecurityContext,
					ImagePullSecrets:   ctrl.Vector.Spec.Agent.ImagePullSecrets,
					Affinity:           ctrl.Vector.Spec.Agent.Affinity,
					RuntimeClassName:   ctrl.Vector.Spec.Agent.RuntimeClassName,
					SchedulerName:      ctrl.Vector.Spec.Agent.SchedulerName,
					Tolerations:        ctrl.Vector.Spec.Agent.Tolerations,
					PriorityClassName:  ctrl.Vector.Spec.Agent.PodSecurityPolicyName,
					HostNetwork:        ctrl.Vector.Spec.Agent.HostNetwork,
					HostAliases:        ctrl.Vector.Spec.Agent.HostAliases,
					InitContainers:     initContainers,
					Containers:         containers,
				},
			},
		},
	}

	return daemonset
}

func (ctrl *Controller) generateVectorAgentVolume() []corev1.Volume {
	volume := ctrl.Vector.Spec.Agent.Volumes
	configVolumeSource := corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: ctrl.getNameVectorAgent(),
		},
	}
	if ctrl.Vector.Spec.Agent.CompressConfigFile {
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
					Path: ctrl.Vector.Spec.Agent.DataDir,
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

	if ctrl.Vector.Spec.Agent.CompressConfigFile {
		volume = append(volume, corev1.Volume{
			Name: "app-config-compress",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ctrl.getNameVectorAgent(),
				},
			},
		})
	}

	return volume
}

func (ctrl *Controller) generateVectorAgentVolumeMounts() []corev1.VolumeMount {
	volumeMount := ctrl.Vector.Spec.Agent.VolumeMounts

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

	if ctrl.Vector.Spec.Agent.CompressConfigFile {
		volumeMount = append(volumeMount, []corev1.VolumeMount{
			{
				Name:      "app-config-compress",
				MountPath: "/tmp/archive",
			},
		}...)
	}

	return volumeMount
}

func (ctrl *Controller) generateVectorAgentEnvs() []corev1.EnvVar {
	envs := ctrl.Vector.Spec.Agent.Env

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

func (ctrl *Controller) VectorAgentContainer() *corev1.Container {
	return &corev1.Container{
		Name:  ctrl.getNameVectorAgent(),
		Image: ctrl.Vector.Spec.Agent.Image,
		Args:  []string{"--config-dir", "/etc/vector", "--watch-config"},
		// Command: []string{"/bin/sleep", "1000000000000000"},
		Env: ctrl.generateVectorAgentEnvs(),
		Ports: []corev1.ContainerPort{
			{
				Name:          "prom-exporter",
				ContainerPort: 9598,
				Protocol:      "TCP",
			},
		},
		VolumeMounts:    ctrl.generateVectorAgentVolumeMounts(),
		ReadinessProbe:  ctrl.Vector.Spec.Agent.ReadinessProbe,
		LivenessProbe:   ctrl.Vector.Spec.Agent.LivenessProbe,
		Resources:       ctrl.Vector.Spec.Agent.Resources,
		SecurityContext: ctrl.Vector.Spec.Agent.ContainerSecurityContext,
		ImagePullPolicy: ctrl.Vector.Spec.Agent.ImagePullPolicy,
	}
}

func (ctrl *Controller) ConfigReloaderInitContainer() *corev1.Container {
	return &corev1.Container{
		Name:            "init-config-reloader",
		Image:           ctrl.Vector.Spec.Agent.ConfigReloaderImage,
		ImagePullPolicy: corev1.PullPolicy(ctrl.Vector.Spec.Agent.ImagePullPolicy),
		Resources:       ctrl.Vector.Spec.Agent.ConfigReloaderResources,
		SecurityContext: ctrl.Vector.Spec.Agent.ContainerSecurityContext,
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
		Image:           ctrl.Vector.Spec.Agent.ConfigReloaderImage,
		ImagePullPolicy: corev1.PullPolicy(ctrl.Vector.Spec.Agent.ImagePullPolicy),
		Resources:       ctrl.Vector.Spec.Agent.ConfigReloaderResources,
		SecurityContext: ctrl.Vector.Spec.Agent.ContainerSecurityContext,
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
