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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
)

// Persistence tests verify that an aggregator with persistence enabled is
// rendered as a StatefulSet backed by a per replica persistent volume claim
// and a headless governing service, rather than as a Deployment.
var _ = Describe("Aggregator persistence", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-persistence")

	const (
		vectorAggregator = "persistent-aggregator"
		aggregator       = "persistent-aggregator-aggregator"
		headlessSvc      = "persistent-aggregator-aggregator-headless"
		dataPVC          = "data-persistent-aggregator-aggregator-0"
	)

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	Context("StatefulSet workload", func() {
		It("should render a StatefulSet with a persistent volume and headless service", func() {
			By("deploying a persistence enabled VectorAggregator")
			f.ApplyTestData("persistence/aggregator-persistent.yaml")

			By("waiting for the StatefulSet to become ready")
			Eventually(func() (string, error) {
				return f.Kubectl().GetWithJsonPath("statefulset", aggregator, ".status.readyReplicas")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Equal("1"), "StatefulSet should have one ready replica")

			By("verifying no Deployment was created for the aggregator")
			_, err := f.Kubectl().Get("deployment", aggregator)
			Expect(err).To(HaveOccurred(), "aggregator should not be a Deployment in persistent mode")

			By("verifying the headless governing service exists")
			clusterIP, err := f.Kubectl().GetWithJsonPath("service", headlessSvc, ".spec.clusterIP")
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterIP).To(Equal("None"), "governing service should be headless")

			By("verifying the per replica PVC is created and bound")
			Eventually(func() (string, error) {
				return f.Kubectl().GetWithJsonPath("pvc", dataPVC, ".status.phase")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Equal("Bound"), "the data PVC should be bound")

			By("verifying the StatefulSet references the headless service")
			svcName, err := f.Kubectl().GetWithJsonPath("statefulset", aggregator, ".spec.serviceName")
			Expect(err).NotTo(HaveOccurred())
			Expect(svcName).To(Equal(headlessSvc))
		})
	})

	Context("Pipeline named headless", func() {
		It("should keep the headless service and report the skipped pipeline service", func() {
			By("creating a pipeline whose service would take the name of the headless service")
			f.ApplyTestData("persistence/pipeline-headless.yaml")
			f.WaitForPipelineValid("headless")

			By("waiting for the aggregator status to name the skipped service")
			Eventually(func() (string, error) {
				return f.Kubectl().GetWithJsonPath("vectoraggregator", vectorAggregator, ".status.reason")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(HavePrefix("Service " + headlessSvc + " is skipped: "))

			By("verifying the config check result stays true")
			result, err := f.Kubectl().GetWithJsonPath("vectoraggregator", vectorAggregator, ".status.configCheckResult")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("true"))

			By("verifying the headless service is still headless and has no pipeline ports")
			clusterIP, err := f.Kubectl().GetWithJsonPath("service", headlessSvc, ".spec.clusterIP")
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterIP).To(Equal("None"), "governing service should stay headless")
			ports, err := f.Kubectl().GetWithJsonPath("service", headlessSvc, ".spec.ports")
			Expect(err).NotTo(HaveOccurred())
			Expect(ports).To(BeEmpty(), "the pipeline service should not be written over the headless service")
		})
	})
})
