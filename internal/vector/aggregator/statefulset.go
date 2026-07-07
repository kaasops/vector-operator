package aggregator

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

func (ctrl *Controller) ensureVectorAggregatorStatefulSet(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-statefulset", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator StatefulSet")
	statefulSet := ctrl.createVectorAggregatorStatefulSet()
	if ctrl.globalConfigChanged() {
		// restart pods
		if statefulSet.Spec.Template.Annotations == nil {
			statefulSet.Spec.Template.Annotations = make(map[string]string)
		}
		statefulSet.Spec.Template.Annotations[common.AnnotationRestartedAt] = time.Now().Format(time.RFC3339)
	}
	return k8s.CreateOrUpdateResource(ctx, statefulSet, ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorStatefulSet() *appsv1.StatefulSet {
	labels := ctrl.labelsForVectorAggregator()
	matchLabels := ctrl.matchLabelsForVectorAggregator()
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

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: appsv1.StatefulSetSpec{
			ServiceName: ctrl.getHeadlessServiceName(),
			// Each replica owns an independent volume and buffer, so pods can start
			// in parallel rather than waiting for lower ordinals to become ready.
			PodManagementPolicy:                  appsv1.ParallelPodManagement,
			PersistentVolumeClaimRetentionPolicy: ctrl.Spec.Persistence.RetentionPolicy,
			VolumeClaimTemplates:                 ctrl.volumeClaimTemplates(),
			Selector:                             &metav1.LabelSelector{MatchLabels: matchLabels},
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
					PriorityClassName:  ctrl.Spec.PriorityClassName,
					HostNetwork:        ctrl.Spec.HostNetwork,
					HostAliases:        ctrl.Spec.HostAliases,
					InitContainers:     initContainers,
					Containers:         containers,
				},
			},
		},
	}

	// Set replicas if autoscaling is disabled
	if ctrl.Spec.Autoscaling.Enabled {
		statefulSet.Spec.Replicas = nil
	} else {
		statefulSet.Spec.Replicas = ctrl.Spec.Replicas
	}

	return statefulSet
}

// volumeClaimTemplates builds the persistent volume claim templates for the
// StatefulSet. Raw templates from the spec take precedence; otherwise a single
// "data" claim is built from the convenience persistence fields and mounted at
// the Vector data_dir.
func (ctrl *Controller) volumeClaimTemplates() []corev1.PersistentVolumeClaim {
	if len(ctrl.Spec.Persistence.VolumeClaimTemplates) > 0 {
		return ctrl.Spec.Persistence.VolumeClaimTemplates
	}

	return []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      ctrl.Spec.Persistence.AccessModes,
				StorageClassName: ctrl.Spec.Persistence.StorageClassName,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: ctrl.Spec.Persistence.Size,
					},
				},
			},
		},
	}
}
