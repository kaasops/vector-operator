package aggregator

import (
	"context"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"github.com/stoewer/go-strcase"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"maps"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (ctrl *Controller) ensureVectorAggregatorService(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-service", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator Service")
	existing, err := ctrl.getExistingServices(ctx)
	if err != nil {
		return err
	}
	svcs, err := ctrl.createVectorAggregatorServices()
	if err != nil {
		return err
	}
	for _, svc := range svcs {
		delete(existing, svc.Name)
		if err := k8s.CreateOrUpdateResource(ctx, svc, ctrl.Client); err != nil {
			return err
		}
	}
	for _, svc := range existing {
		if err := ctrl.Client.Delete(ctx, svc); err != nil {
			return err
		}
	}
	return nil
}

func (ctrl *Controller) createVectorAggregatorServices() ([]*corev1.Service, error) {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	svcList := make([]*corev1.Service, 0)

	for group, list := range ctrl.Config.GetSourcesServicePorts() {
		ann := make(map[string]string, len(annotations))
		maps.Copy(ann, annotations)

		ports := make([]corev1.ServicePort, 0, len(list))
		for _, sp := range list {
			ports = append(ports, corev1.ServicePort{
				Name:       strcase.KebabCase(sp.SourceName),
				Protocol:   sp.Protocol,
				Port:       sp.Port,
				TargetPort: intstr.FromInt32(sp.Port),
			})
		}
		svc := &corev1.Service{
			ObjectMeta: ctrl.objectMetaVectorAggregator(labels, ann, ctrl.Namespace),
			Spec: corev1.ServiceSpec{
				Ports:    ports,
				Selector: labels,
			},
		}
		svc.ObjectMeta.Name = group.ServiceName
		svcList = append(svcList, svc)
	}

	if ctrl.Spec.Api.Enabled {
		svcList = append(svcList, &corev1.Service{
			ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
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

func (ctrl *Controller) getExistingServices(ctx context.Context) (map[string]*corev1.Service, error) {
	svcList := corev1.ServiceList{}
	opts := &client.ListOptions{
		Namespace:     ctrl.Namespace,
		LabelSelector: labels.Set(ctrl.labelsForVectorAggregator()).AsSelector(),
	}
	err := ctrl.Client.List(ctx, &svcList, opts)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]*corev1.Service, len(svcList.Items))
	for _, svc := range svcList.Items {
		existing[svc.Name] = &svc
	}
	return existing, nil
}
