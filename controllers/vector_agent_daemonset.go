package controllers

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func (r *VectorReconciler) createVectorAgentDaemonSet(v *vectorv1alpha1.Vector) *appsv1.DaemonSet {
	labels := labelsForVectorAgent(v.Name)

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: objectMetaVectorAgent(v, labels),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: objectMetaVectorAgent(v, labels),
				Spec: corev1.PodSpec{
					ServiceAccountName: getNameVectorAgent(v),
					Volumes:            generateVectorAgentVolume(v),
					SecurityContext:    &corev1.PodSecurityContext{},
					Containers: []corev1.Container{
						{
							Name:  getNameVectorAgent(v),
							Image: v.Spec.Agent.Image,
							Args:  []string{"--config-dir", "/etc/vector/"},
							Env:   generateVectorAgentEnvs(v),
							Ports: []corev1.ContainerPort{
								{
									Name:          "prom-exporter",
									ContainerPort: 9090,
									Protocol:      "TCP",
								},
							},
							VolumeMounts:    generateVectorAgentVolumeMounts(v),
							SecurityContext: &corev1.SecurityContext{},
						},
					},
				},
			},
		},
	}

	return daemonset
}

func generateVectorAgentVolume(v *vectorv1alpha1.Vector) []corev1.Volume {
	volume := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: getNameVectorAgent(v),
				},
			},
		},
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/vector",
				},
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

func generateVectorAgentVolumeMounts(spec *vectorv1alpha1.Vector) []corev1.VolumeMount {
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

func generateVectorAgentEnvs(spec *vectorv1alpha1.Vector) []corev1.EnvVar {
	envs := []corev1.EnvVar{
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
	}

	return envs
}
