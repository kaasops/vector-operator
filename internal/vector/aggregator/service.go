package aggregator

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/stoewer/go-strcase"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

func (ctrl *Controller) ensureVectorAggregatorService(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-service", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator Service")
	ctrl.skippedServices = nil
	existing, err := ctrl.getExistingServices(ctx)
	if err != nil {
		return err
	}
	svcs, err := ctrl.createVectorAggregatorServices()
	if err != nil {
		return err
	}
	for _, svc := range svcs {
		current := existing[svc.Name]
		delete(existing, svc.Name)
		if err := ctrl.createOrUpdateService(ctx, svc, current); err != nil {
			// A bad field and, by default, a ValidatingAdmissionPolicy denial come back as Invalid: skip the Service and report it.
			if !api_errors.IsInvalid(err) {
				return err
			}
			ctrl.skipService(svc.Name, rejectionMessage(err))
		}
	}
	for _, svc := range existing {
		if err := ctrl.Delete(ctx, svc); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(ctrl.skippedServices)) {
		log.Error(errors.New(ctrl.skippedServices[name]), "Service is skipped", "service", name)
	}
	return nil
}

// createOrUpdateService writes svc. Under the headless name it deletes current (our Service there, or nil) and creates svc
// when persistence was turned on over a pipeline Service, or off over the governing Service.
func (ctrl *Controller) createOrUpdateService(ctx context.Context, svc, current *corev1.Service) error {
	if current != nil && svc.Name == ctrl.getHeadlessServiceName() &&
		(isHeadless(svc) && !isHeadless(current) || !isHeadless(svc) && isGoverningService(current)) {
		// Persistence was turned on or off, and a cluster IP cannot be added or removed in place; the UID precondition keeps a stale cache from deleting a newer Service.
		if err := ctrl.Delete(ctx, current, client.Preconditions{UID: &current.UID}); err != nil && !api_errors.IsNotFound(err) {
			return err
		}
		return ctrl.Create(ctx, svc)
	}
	return k8s.CreateOrUpdateResource(ctx, svc, ctrl.Client)
}

func isHeadless(svc *corev1.Service) bool {
	return svc.Spec.ClusterIP == corev1.ClusterIPNone
}

// isGoverningService reports whether svc looks like the headless Service the operator creates for the StatefulSet.
func isGoverningService(svc *corev1.Service) bool {
	return isHeadless(svc) && len(svc.Spec.Ports) == 0 && svc.Spec.PublishNotReadyAddresses
}

// skipService records a Service that this round does not write, for the status.
func (ctrl *Controller) skipService(name, reason string) {
	if ctrl.skippedServices == nil {
		ctrl.skippedServices = map[string]string{}
	}
	ctrl.skippedServices[name] = reason
}

// rejectionMessage returns the message of the API server error without the wrapping.
func rejectionMessage(err error) string {
	var status api_errors.APIStatus
	if errors.As(err, &status) {
		return status.Status().Message
	}
	return err.Error()
}

// HasSkippedServices reports whether the last Service step skipped a Service.
func (ctrl *Controller) HasSkippedServices() bool {
	return len(ctrl.skippedServices) > 0
}

// skippedServicesReason returns the status reason for the skipped Services, or nil if none was skipped.
func (ctrl *Controller) skippedServicesReason() *string {
	if len(ctrl.skippedServices) == 0 {
		return nil
	}
	problems := make([]string, 0, len(ctrl.skippedServices))
	for _, name := range slices.Sorted(maps.Keys(ctrl.skippedServices)) {
		problems = append(problems, fmt.Sprintf("Service %s is skipped: %s", name, ctrl.skippedServices[name]))
	}
	reason := strings.Join(problems, "; ")
	return &reason
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
		// With persistence the headless Service below has this name, so a pipeline Service must not take it.
		if ctrl.persistenceEnabled() && group.ServiceName == ctrl.getHeadlessServiceName() {
			ctrl.skipService(group.ServiceName, "with persistence enabled, this name is used by the headless Service of the StatefulSet")
			continue
		}
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
