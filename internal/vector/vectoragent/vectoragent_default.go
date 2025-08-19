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
	"github.com/kaasops/vector-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (ctrl *Controller) SetDefault() {
	if ctrl.Vector.Spec.Agent == nil {
		ctrl.Vector.Spec.Agent = new(v1alpha1.VectorAgent)
	}
	if ctrl.Vector.Spec.Agent.Image == "" {
		ctrl.Vector.Spec.Agent.Image = "timberio/vector:0.47.0-distroless-libc"
	}

	if ctrl.Vector.Spec.Agent.Resources.Requests == nil {
		ctrl.Vector.Spec.Agent.Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.Vector.Spec.Agent.Resources.Limits == nil {
		ctrl.Vector.Spec.Agent.Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}

	if ctrl.Vector.Spec.Agent.DataDir == "" {
		ctrl.Vector.Spec.Agent.DataDir = "/var/lib/vector"
	}

	if ctrl.Vector.Spec.Agent.Volumes == nil {
		ctrl.Vector.Spec.Agent.Volumes = []corev1.Volume{
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

	if ctrl.Vector.Spec.Agent.ReadinessProbe == nil && ctrl.Vector.Spec.Agent.Api.Enabled && ctrl.Vector.Spec.Agent.Api.Healthcheck {
		ctrl.Vector.Spec.Agent.ReadinessProbe = &corev1.Probe{
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
	if ctrl.Vector.Spec.Agent.LivenessProbe == nil && ctrl.Vector.Spec.Agent.Api.Enabled && ctrl.Vector.Spec.Agent.Api.Healthcheck {
		ctrl.Vector.Spec.Agent.LivenessProbe = &corev1.Probe{
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

	if ctrl.Vector.Spec.Agent.VolumeMounts == nil {
		ctrl.Vector.Spec.Agent.VolumeMounts = []corev1.VolumeMount{
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
	if ctrl.Vector.Spec.Agent.CompressConfigFile && ctrl.Vector.Spec.Agent.ConfigReloaderImage == "" {
		ctrl.Vector.Spec.Agent.ConfigReloaderImage = "docker.io/kaasops/config-reloader:v0.1.4"
	}
	if ctrl.Vector.Spec.Agent.CompressConfigFile && ctrl.Vector.Spec.Agent.ConfigReloaderResources.Requests == nil {
		ctrl.Vector.Spec.Agent.ConfigReloaderResources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.Vector.Spec.Agent.CompressConfigFile && ctrl.Vector.Spec.Agent.ConfigReloaderResources.Limits == nil {
		ctrl.Vector.Spec.Agent.ConfigReloaderResources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}

}
