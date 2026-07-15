package aggregator

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
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
	if err := k8s.CreateOrUpdateResource(ctx, deployment, ctrl.Client); err != nil {
		return err
	}
	// Remove a StatefulSet left over from before persistence was disabled. Its
	// PVCs are kept, since the default retention policy is Retain.
	return ctrl.deleteObsoleteWorkload(ctx, &appsv1.StatefulSet{})
}

func (ctrl *Controller) createVectorAggregatorDeployment() *appsv1.Deployment {
	labels := ctrl.labelsForVectorAggregator()
	matchLabels := ctrl.matchLabelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	return &appsv1.Deployment{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: appsv1.DeploymentSpec{
			Replicas: ctrl.aggregatorReplicas(),
			Selector: &metav1.LabelSelector{MatchLabels: matchLabels},
			Template: ctrl.aggregatorPodTemplateSpec(),
		},
	}
}

// aggregatorPodTemplateSpec builds the pod template shared by the Deployment and
// StatefulSet workloads, so the pod shape stays identical across both paths.
func (ctrl *Controller) aggregatorPodTemplateSpec() corev1.PodTemplateSpec {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	containers := []corev1.Container{*ctrl.VectorAggregatorContainer()}
	var initContainers []corev1.Container
	if ctrl.Spec.CompressConfigFile {
		initContainers = append(initContainers, *ctrl.ConfigReloaderInitContainer())
		containers = append(containers, *ctrl.ConfigReloaderSidecarContainer())
	}

	return corev1.PodTemplateSpec{
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
			PriorityClassName:  ctrl.Spec.PriorityClassName,
			HostNetwork:        ctrl.Spec.HostNetwork,
			HostAliases:        ctrl.Spec.HostAliases,
			InitContainers:     initContainers,
			Containers:         containers,
		},
	}
}

// aggregatorReplicas returns the desired replica count, or nil when autoscaling
// is enabled so the operator does not fight the HPA.
func (ctrl *Controller) aggregatorReplicas() *int32 {
	if ctrl.Spec.Autoscaling.Enabled {
		return nil
	}
	return ctrl.Spec.Replicas
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

	// Merge user-defined volumes with required volumes.
	// User-defined volumes take precedence over required volumes with the same name.
	// Build a set of user-defined volume names to check for conflicts.
	existingVolumes := make(map[string]bool, len(volume))
	for _, v := range volume {
		existingVolumes[v.Name] = true
	}

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

	// Define required volumes for Vector aggregator
	requiredVolumes := []corev1.Volume{
		{
			Name:         "config",
			VolumeSource: configVolumeSource,
		},
	}

	// In persistent mode the data volume comes from the StatefulSet volume claim
	// template, so only add the hostPath data volume for the Deployment path.
	// Keep it right after "config": reordering the volume list changes the pod
	// template and rolls every non-persistent aggregator on operator upgrade.
	if !ctrl.persistenceEnabled() {
		requiredVolumes = append(requiredVolumes, corev1.Volume{
			Name: dataVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: ctrl.Spec.DataDir,
				},
			},
		})
	}

	requiredVolumes = append(requiredVolumes,
		corev1.Volume{
			Name: "procfs",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/proc",
				},
			},
		},
		corev1.Volume{
			Name: "sysfs",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys",
				},
			},
		},
	)

	// Only add volumes that don't already exist
	for _, reqVol := range requiredVolumes {
		if !existingVolumes[reqVol.Name] {
			volume = append(volume, reqVol)
		}
	}

	if ctrl.Spec.CompressConfigFile && !existingVolumes["app-config-compress"] {
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

	// Merge user-defined volumeMounts with required volumeMounts.
	// User-defined volumeMounts take precedence over required volumeMounts with the same name.
	// Build a set of user-defined volumeMount names to check for conflicts.
	existingVolumeMounts := make(map[string]bool, len(volumeMount))
	for _, vm := range volumeMount {
		existingVolumeMounts[vm.Name] = true
	}

	// Define required volumeMounts for Vector aggregator
	requiredVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/etc/vector",
		},
		{
			Name:      dataVolumeName,
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
	}

	// Only add volumeMounts that don't already exist
	for _, reqVm := range requiredVolumeMounts {
		if !existingVolumeMounts[reqVm.Name] {
			volumeMount = append(volumeMount, reqVm)
		}
	}

	if ctrl.Spec.CompressConfigFile && !existingVolumeMounts["app-config-compress"] {
		volumeMount = append(volumeMount, corev1.VolumeMount{
			Name:      "app-config-compress",
			MountPath: "/tmp/archive",
		})
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
