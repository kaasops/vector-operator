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

package configcheck

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (cc *ConfigCheck) createVectorConfigCheckPod() *corev1.Pod {
	labels := labelsForVectorConfigCheck()
	annotations := cc.annotationsForVectorConfigCheck()
	var initContainers []corev1.Container

	if cc.CompressedConfig {
		initContainers = append(initContainers, *cc.ConfigReloaderInitContainer())
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cc.getNameVectorConfigCheck(),
			Namespace:   cc.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "vector-configcheck",
			Volumes:            cc.generateVectorConfigCheckVolume(),
			SecurityContext:    cc.SecurityContext,
			ImagePullSecrets:   cc.ImagePullSecrets,
			Tolerations:        cc.Tolerations,
			InitContainers:     initContainers,
			Containers: []corev1.Container{
				{
					Name:            "config-check",
					Image:           cc.Image,
					Resources:       cc.Resources,
					Args:            []string{"--require-healthy=false", "validate", "/etc/vector/*.json"},
					Env:             cc.generateVectorConfigCheckEnvs(),
					SecurityContext: cc.ContainerSecurityContext,
					VolumeMounts:    cc.generateVectorConfigCheckVolumeMounts(),
				},
			},
			RestartPolicy: "Never",
		},
	}

	return pod
}

func (cc *ConfigCheck) generateVectorConfigCheckVolume() []corev1.Volume {
	configVolumeSource := corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: cc.getNameVectorConfigCheck(),
		},
	}
	if cc.CompressedConfig {
		configVolumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}

	}
	volume := []corev1.Volume{
		{
			Name:         "config",
			VolumeSource: configVolumeSource,
		},
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "var-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/",
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
	}

	if cc.CompressedConfig {
		volume = append(volume, corev1.Volume{
			Name: "app-config-compress",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cc.getNameVectorConfigCheck(),
				},
			},
		})
	}

	return volume
}

func (cc *ConfigCheck) generateVectorConfigCheckVolumeMounts() []corev1.VolumeMount {
	volumeMount := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/etc/vector/",
		},
		{
			Name:      "data",
			MountPath: "/vector-data-dir",
		},
		{
			Name:      "var-log",
			MountPath: "/var/log/",
		},
		{
			Name:      "var-lib",
			MountPath: "/var/lib/",
		},
		{
			Name:      "procfs",
			MountPath: "/host/proc",
		},
		{
			Name:      "sysfs",
			MountPath: "/host/sys",
		},
	}

	if cc.CompressedConfig {
		volumeMount = append(volumeMount, []corev1.VolumeMount{
			{
				Name:      "app-config-compress",
				MountPath: "/tmp/archive",
			},
		}...)
	}

	return volumeMount
}

func (cc *ConfigCheck) generateVectorConfigCheckEnvs() []corev1.EnvVar {
	envs := cc.Envs

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

func (cc *ConfigCheck) ConfigReloaderInitContainer() *corev1.Container {
	return &corev1.Container{
		Name:            "init-config-reloader",
		Image:           cc.ConfigReloaderImage,
		ImagePullPolicy: cc.ImagePullPolicy,
		Resources:       cc.ConfigReloaderResources,
		SecurityContext: cc.ContainerSecurityContext,
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
