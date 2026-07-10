package aggregator

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

// dataVolumeName is the name of the volume that backs the Vector data_dir. The
// aggregator container mounts it at /vector-data-dir in both the Deployment and
// StatefulSet paths, so a persistent volume claim must use this exact name.
const dataVolumeName = "data"

func (ctrl *Controller) ensureVectorAggregatorStatefulSet(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-statefulset", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator StatefulSet")
	if err := ctrl.validatePersistence(); err != nil {
		return err
	}
	statefulSet := ctrl.createVectorAggregatorStatefulSet()
	if ctrl.globalConfigChanged() {
		// restart pods
		if statefulSet.Spec.Template.Annotations == nil {
			statefulSet.Spec.Template.Annotations = make(map[string]string)
		}
		statefulSet.Spec.Template.Annotations[common.AnnotationRestartedAt] = time.Now().Format(time.RFC3339)
	}
	if err := k8s.CreateOrUpdateResource(ctx, statefulSet, ctrl.Client); err != nil {
		return err
	}
	// Remove a Deployment left over from before persistence was enabled.
	return ctrl.deleteObsoleteWorkload(ctx, &appsv1.Deployment{})
}

func (ctrl *Controller) createVectorAggregatorStatefulSet() *appsv1.StatefulSet {
	labels := ctrl.labelsForVectorAggregator()
	matchLabels := ctrl.matchLabelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()

	return &appsv1.StatefulSet{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Spec: appsv1.StatefulSetSpec{
			ServiceName: ctrl.getHeadlessServiceName(),
			// Each replica owns an independent volume and buffer, so pods can start
			// in parallel rather than waiting for lower ordinals to become ready.
			PodManagementPolicy:                  appsv1.ParallelPodManagement,
			PersistentVolumeClaimRetentionPolicy: ctrl.Spec.Persistence.RetentionPolicy,
			VolumeClaimTemplates:                 ctrl.volumeClaimTemplates(),
			Replicas:                             ctrl.aggregatorReplicas(),
			Selector:                             &metav1.LabelSelector{MatchLabels: matchLabels},
			Template:                             ctrl.aggregatorPodTemplateSpec(),
		},
	}
}

// validatePersistence rejects a persistence config that would render pods
// referencing a data volume that does not exist. The aggregator container always
// mounts a volume named dataVolumeName, so a raw volumeClaimTemplates escape hatch
// must include a claim with that name. The default claim already uses it.
func (ctrl *Controller) validatePersistence() error {
	if len(ctrl.Spec.Persistence.VolumeClaimTemplates) == 0 {
		return nil
	}
	for _, vct := range ctrl.Spec.Persistence.VolumeClaimTemplates {
		if vct.Name == dataVolumeName {
			return nil
		}
	}
	return fmt.Errorf("persistence.volumeClaimTemplates must include a claim named %q, which backs the Vector data_dir", dataVolumeName)
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
				Name: dataVolumeName,
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
