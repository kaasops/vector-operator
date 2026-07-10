package aggregator

import (
	"context"
	"maps"

	"github.com/stoewer/go-strcase"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
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
		if err := ctrl.Delete(ctx, svc); err != nil {
			return err
		}
	}
	return nil
}

func (ctrl *Controller) createVectorAggregatorServices() ([]*corev1.Service, error) {
	labels := ctrl.labelsForVectorAggregator()
	matchLabels := ctrl.matchLabelsForVectorAggregator()
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
				Selector: matchLabels,
			},
		}
		svc.Name = group.ServiceName
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

	// In persistent mode the aggregator is a StatefulSet, which needs a headless
	// governing service for stable per replica DNS. It carries the aggregator
	// labels so the reconcile loop deletes it again if persistence is turned off.
	if ctrl.persistenceEnabled() {
		headless := &corev1.Service{
			ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
			Spec: corev1.ServiceSpec{
				ClusterIP:                corev1.ClusterIPNone,
				Selector:                 matchLabels,
				PublishNotReadyAddresses: true,
			},
		}
		headless.Name = ctrl.getHeadlessServiceName()
		svcList = append(svcList, headless)
	}

	return svcList, nil
}

func (ctrl *Controller) getExistingServices(ctx context.Context) (map[string]*corev1.Service, error) {
	svcList := corev1.ServiceList{}
	opts := &client.ListOptions{
		Namespace:     ctrl.Namespace,
		LabelSelector: labels.Set(ctrl.matchLabelsForVectorAggregator()).AsSelector(),
	}
	err := ctrl.List(ctx, &svcList, opts)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]*corev1.Service, len(svcList.Items))
	for _, svc := range svcList.Items {
		existing[svc.Name] = &svc
	}
	return existing, nil
}
