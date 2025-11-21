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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
)

// Normal Mode tests verify that the operator works correctly for standard pipelines.
// These tests cover basic Vector Agent and Aggregator functionality.
var _ = Describe("Normal Mode", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-normal-mode")

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	Context("VectorPipeline basics", func() {
		It("should create and validate a basic pipeline with agent", func() {
			By("deploying Vector Agent")
			f.ApplyTestData("normal-mode/agent.yaml")

			// Give controller time to process Vector CR Create event and create daemonset
			// Normal mode requires slightly more time as it involves more resources
			time.Sleep(5 * time.Second)

			By("creating a VectorPipeline")
			f.ApplyTestData("normal-mode/pipeline-basic.yaml")

			By("waiting for pipeline to become valid")
			f.WaitForPipelineValid("basic-pipeline")

			By("verifying agent processes the pipeline configuration")
			Eventually(func() error {
				// Check that agent config contains the pipeline components
				return f.VerifyAgentHasPipeline("normal-agent", "basic-pipeline")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})

		It("should handle pipeline with transforms and multiple sinks", func() {
			By("creating a complex pipeline with transforms")
			f.ApplyTestData("normal-mode/pipeline-complex.yaml")

			By("waiting for pipeline to become valid")
			f.WaitForPipelineValid("complex-pipeline")

			By("verifying pipeline has expected components")
			// Pipeline should have sources, transforms, and sinks all in agent
			Eventually(func() error {
				return f.VerifyAgentHasPipeline("normal-agent", "complex-pipeline")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("VectorAggregator basics", func() {
		It("should deploy aggregator and process pipelines", func() {
			By("deploying VectorAggregator")
			f.ApplyTestData("normal-mode/aggregator.yaml")
			f.WaitForDeploymentReady("normal-aggregator-aggregator")

			By("creating a pipeline with aggregator role")
			f.ApplyTestData("normal-mode/pipeline-aggregator-role.yaml")

			By("waiting for pipeline to become valid")
			f.WaitForPipelineValid("aggregator-pipeline")

			By("verifying pipeline has aggregator role")
			role := f.GetPipelineStatus("aggregator-pipeline", "role")
			Expect(role).To(Equal("aggregator"))

			By("verifying aggregator processes the pipeline")
			Eventually(func() error {
				return f.VerifyAggregatorHasPipeline("normal-aggregator", "aggregator-pipeline")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("Multiple pipelines in normal mode", func() {
		It("should handle multiple pipelines without conflicts", func() {
			By("creating 3 pipelines in normal mode")
			for i := 1; i <= 3; i++ {
				f.ApplyTestDataWithVars("normal-mode/pipeline-template.yaml",
					map[string]string{"{{INDEX}}": fmt.Sprintf("pipeline-%d", i)})
			}

			By("waiting for all pipelines to become valid")
			f.WaitForPipelineValid("pipeline-1")
			f.WaitForPipelineValid("pipeline-2")
			f.WaitForPipelineValid("pipeline-3")

			By("verifying all pipelines are in agent configuration")
			Eventually(func() error {
				if err := f.VerifyAgentHasPipeline("normal-agent", "pipeline-1"); err != nil {
					return err
				}
				if err := f.VerifyAgentHasPipeline("normal-agent", "pipeline-2"); err != nil {
					return err
				}
				return f.VerifyAgentHasPipeline("normal-agent", "pipeline-3")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("Pipeline deletion in normal mode", func() {
		It("should clean up pipeline from agent config when deleted", func() {
			By("creating a pipeline")
			f.ApplyTestData("normal-mode/pipeline-deletable.yaml")
			f.WaitForPipelineValid("deletable-pipeline")

			By("verifying pipeline is in agent config")
			Eventually(func() error {
				return f.VerifyAgentHasPipeline("normal-agent", "deletable-pipeline")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("deleting the pipeline")
			f.DeleteResource("vectorpipeline", "deletable-pipeline")

			By("verifying pipeline is removed from agent config")
			Eventually(func() bool {
				err := f.VerifyAgentHasPipeline("normal-agent", "deletable-pipeline")
				return err != nil // Should return error when pipeline not found
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(BeTrue())
		})
	})

	Context("Kubernetes logs source with label selectors", func() {
		It("should collect logs from pods matching label selector", func() {
			By("deploying a test pod with specific labels")
			f.ApplyTestData("normal-mode/test-app-pod.yaml")
			f.WaitForPodReady("test-app")

			By("creating pipeline with kubernetes_logs source and label selector")
			f.ApplyTestData("normal-mode/pipeline-kubernetes-logs.yaml")
			f.WaitForPipelineValid("k8s-logs-pipeline")

			By("verifying agent has kubernetes_logs source")
			Eventually(func() error {
				return f.VerifyAgentHasPipeline("normal-agent", "k8s-logs-pipeline")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying pipeline role is Agent")
			role := f.GetPipelineStatus("k8s-logs-pipeline", "role")
			Expect(role).To(Equal("agent"), "kubernetes_logs pipeline should have agent role")
		})
	})

	Context("Namespace isolation", func() {
		It("should only collect logs from the pipeline's namespace", func() {
			By("creating a separate namespace")
			f.ApplyTestDataWithoutNamespaceReplacement("normal-mode/namespace-isolation-ns.yaml")

			By("deploying Vector agent in isolated namespace")
			// Note: In real scenario, the same Vector DaemonSet serves all namespaces
			// But pipelines are namespace-scoped

			By("deploying pods in both namespaces")
			f.ApplyTestData("normal-mode/namespace-isolation-pod-main.yaml")
			f.ApplyTestDataWithoutNamespaceReplacement("normal-mode/namespace-isolation-pod-isolated.yaml")
			f.WaitForPodReady("main-namespace-pod")
			f.WaitForPodReadyInNamespace("isolated-pod", "test-normal-mode-isolated")

			By("creating pipeline in isolated namespace")
			f.ApplyTestDataWithoutNamespaceReplacement("normal-mode/namespace-isolation-pipeline.yaml")
			f.WaitForPipelineValidInNamespace("isolated-pipeline", "test-normal-mode-isolated")

			By("verifying namespace isolation in configuration")
			// The agent config should have extra_namespace_label_selector set to the pipeline's namespace
			Eventually(func() error {
				return f.VerifyAgentHasPipelineInNamespace("normal-agent", "isolated-pipeline", "test-normal-mode-isolated")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("ClusterVectorPipeline", func() {
		It("should collect logs from multiple namespaces", func() {
			By("creating ClusterVectorPipeline")
			f.ApplyTestDataWithoutNamespaceReplacement("normal-mode/cluster-pipeline.yaml")
			f.WaitForClusterPipelineValid("cluster-wide-pipeline")

			By("deploying test pods in different namespaces with matching labels")
			f.ApplyTestData("normal-mode/cluster-pipeline-pod-ns1.yaml")
			f.ApplyTestDataWithoutNamespaceReplacement("normal-mode/cluster-pipeline-pod-ns2.yaml")
			f.WaitForPodReady("cluster-monitored-pod-1")
			f.WaitForPodReadyInNamespace("cluster-monitored-pod-2", "test-normal-mode-isolated")

			By("verifying agent processes the ClusterVectorPipeline")
			Eventually(func() error {
				return f.VerifyAgentHasClusterPipeline("normal-agent", "cluster-wide-pipeline")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying pipeline role is Agent")
			role := f.GetClusterPipelineStatus("cluster-wide-pipeline", "role")
			Expect(role).To(Equal("agent"), "ClusterVectorPipeline with kubernetes_logs should have agent role")
		})
	})
})
