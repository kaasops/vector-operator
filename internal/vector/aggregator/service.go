package aggregator

import (
	"context"
	"fmt"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"github.com/stoewer/go-strcase"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"maps"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (ctrl *Controller) ensureVectorAggregatorService(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-aggregator-service", ctrl.VectorAggregator.Name)
	log.Info("start Reconcile Vector Aggregator Service")
	svcs, err := ctrl.createVectorAggregatorService()
	if err != nil {
		return err
	}
	if len(svcs) == 0 {
		return nil
	}
	for _, svc := range svcs {
		if err := k8s.CreateOrUpdateResource(ctx, svc, ctrl.Client); err != nil {
			return err
		}
	}
	return nil
}

func (ctrl *Controller) createVectorAggregatorService() ([]*corev1.Service, error) {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	svcList := make([]*corev1.Service, 0)

	for pipelineName, list := range ctrl.Config.GetSourcesServicePorts() {
		ann := make(map[string]string, len(annotations))
		maps.Copy(ann, annotations)

		ports := make([]corev1.ServicePort, 0, len(list))
		for _, sp := range list {
			if sp.IsKubernetesEvents {
				ann["observability.kaasops.io/k8s-events-namespace"] = sp.Namespace
			}
			ports = append(ports, corev1.ServicePort{
				Name:       strcase.KebabCase(sp.SourceName),
				Protocol:   sp.Protocol,
				Port:       sp.Port,
				TargetPort: intstr.FromInt32(sp.Port),
			})
		}
		svc := &corev1.Service{
			ObjectMeta: ctrl.objectMetaVectorAggregator(labels, ann, ctrl.VectorAggregator.Namespace),
			Spec: corev1.ServiceSpec{
				Ports:    ports,
				Selector: labels,
			},
		}
		svc.ObjectMeta.Name = strcase.KebabCase(fmt.Sprintf("%s-%s", svc.ObjectMeta.Name, pipelineName))
		svcList = append(svcList, svc)
	}

	if ctrl.VectorAggregator.Spec.Api.Enabled {
		svcList = append(svcList, &corev1.Service{
			ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.VectorAggregator.Namespace),
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:       "api",
						Protocol:   corev1.ProtocolTCP,
						Port:       ApiPort,
						TargetPort: intstr.FromInt32(ApiPort),
					},
				},
				Selector: labels,
			},
		})
	}

	return svcList, nil
}
