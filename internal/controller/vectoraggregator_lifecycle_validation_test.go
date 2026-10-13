/*
Copyright 2024.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	observabilityv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

var _ = Describe("VectorAggregator lifecycle validation", func() {
	ctx := context.Background()

	aggregator := func(name string, grace *int64, lifecycle *corev1.Lifecycle) *observabilityv1alpha1.VectorAggregator {
		return &observabilityv1alpha1.VectorAggregator{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: observabilityv1alpha1.VectorAggregatorSpec{
				VectorAggregatorCommon: observabilityv1alpha1.VectorAggregatorCommon{
					TerminationGracePeriodSeconds: grace,
					Lifecycle:                     lifecycle,
				},
			},
		}
	}

	preStopSleep := func(seconds int64) *corev1.Lifecycle {
		return &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{Sleep: &corev1.SleepAction{Seconds: seconds}},
		}
	}

	It("rejects a preStop sleep longer than the default grace period", func() {
		err := k8sClient.Create(ctx, aggregator("sleep-over-default", nil, preStopSleep(45)))
		Expect(errors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("lifecycle.preStop.sleep.seconds must not exceed terminationGracePeriodSeconds"))
	})

	It("rejects a preStop sleep longer than the configured grace period", func() {
		err := k8sClient.Create(ctx, aggregator("sleep-over-grace", ptr.To(int64(20)), preStopSleep(25)))
		Expect(errors.IsInvalid(err)).To(BeTrue(), "got %v", err)
	})

	It("accepts a preStop sleep within the grace period", func() {
		resource := aggregator("sleep-within-grace", ptr.To(int64(60)), preStopSleep(45))
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
	})

	It("rejects stopSignal", func() {
		lifecycle := &corev1.Lifecycle{StopSignal: ptr.To(corev1.SIGINT)}
		err := k8sClient.Create(ctx, aggregator("stop-signal", nil, lifecycle))
		Expect(errors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("lifecycle.stopSignal is not supported"))
	})
})
