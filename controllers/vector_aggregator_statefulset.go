package controllers

import (
	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *VectorReconciler) createVectorAggregatorStatefulSet(v *vectorv1alpha1.Vector) *appsv1.StatefulSet {
	labels := labelsForVectorAggregator(v.Name)
	replicas := int32(v.Spec.Aggregator.Replicas)
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: objectMetaVectorAggregator(v, labels),
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: objectMetaVectorAggregator(v, labels),
				Spec: corev1.PodSpec{
					ServiceAccountName: getNameVectorAggregator(v),
					Volumes:            generateVectorAggregatorVolume(v),
					SecurityContext:    &corev1.PodSecurityContext{},
					Containers: []corev1.Container{
						{
							Name:  getNameVectorAggregator(v),
							Image: v.Spec.Aggregator.Image,
							Args:  []string{"--config-dir", "/etc/vector/"},
							Env:   generateVectorAggregatorEnvs(v),
							Ports: []corev1.ContainerPort{
								{
									Name:          "vector",
									ContainerPort: 6000,
									Protocol:      "TCP",
								},
							},
							VolumeMounts:    generateVectorAggregatorVolumeMounts(v),
							SecurityContext: &corev1.SecurityContext{},
						},
					},
				},
			},
		},
	}

	return statefulset
}

func generateVectorAggregatorVolume(v *vectorv1alpha1.Vector) []corev1.Volume {
	volume := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: getNameVectorAggregator(v),
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
	}

	return volume
}

func generateVectorAggregatorVolumeMounts(spec *vectorv1alpha1.Vector) []corev1.VolumeMount {
	volumeMount := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/etc/vector/",
		},
		{
			Name:      "data",
			MountPath: "/vector-data-dir",
		},
	}

	return volumeMount
}

func generateVectorAggregatorEnvs(spec *vectorv1alpha1.Vector) []corev1.EnvVar {
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
	}

	return envs
}
