package aggregator

import (
	"context"
	"fmt"
	"github.com/kaasops/vector-operator/internal/common"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
	"github.com/stoewer/go-strcase"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"maps"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
)

func (ctrl *Controller) ensureVectorAggregatorService(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("vector-aggregator-service", ctrl.VectorAggregator.Name)
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
		if svc.Annotations[common.AnnotationK8sEventsPort] != "" {
			ctrl.EventsCollector.RegisterSubscriber(
				svc.Name,
				svc.Namespace,
				svc.Annotations[common.AnnotationK8sEventsPort],
				svc.Annotations[common.AnnotationK8sEventsNamespace],
			)
		}
	}
	for _, svc := range existing {
		if svc.Annotations[common.AnnotationK8sEventsPort] != "" {
			ctrl.EventsCollector.UnregisterSubscriber(svc.Name, svc.Namespace)
		}
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
			if sp.IsKubernetesEvents {
				ann[common.AnnotationK8sEventsNamespace] = sp.Namespace
				ann[common.AnnotationK8sEventsPort] = strconv.Itoa(int(sp.Port))
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
		name := group.ServiceName
		if name == "" {
			name = strcase.KebabCase(fmt.Sprintf("%s-%s", svc.ObjectMeta.Name, group.PipelineName))
		}
		svc.ObjectMeta.Name = name
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

func (ctrl *Controller) getExistingServices(ctx context.Context) (map[string]*corev1.Service, error) {
	svcList := corev1.ServiceList{}
	opts := &client.ListOptions{
		Namespace:     ctrl.VectorAggregator.Namespace,
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
