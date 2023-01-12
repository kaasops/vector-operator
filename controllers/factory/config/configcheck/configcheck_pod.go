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

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cc.getNameVectorConfigCheck(),
			Namespace: cc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "vector-configcheck",
			Volumes:            cc.generateVectorConfigCheckVolume(),
			SecurityContext:    &corev1.PodSecurityContext{},
			Tolerations:        cc.Tolerations,
			Containers: []corev1.Container{
				{
					Name:      "config-check",
					Image:     cc.Image,
					Resources: cc.Resources,
					Args:      []string{"validate", "/etc/vector/*.json"},
					Env:       cc.generateVectorConfigCheckEnvs(),
					Ports: []corev1.ContainerPort{
						{
							Name:          "prom-exporter",
							ContainerPort: 9090,
							Protocol:      "TCP",
						},
					},
					VolumeMounts: cc.generateVectorConfigCheckVolumeMounts(),
				},
			},
			RestartPolicy: "Never",
		},
	}

	return pod
}

func (cc *ConfigCheck) generateVectorConfigCheckVolume() []corev1.Volume {
	volume := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cc.getNameVectorConfigCheck(),
				},
			},
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
