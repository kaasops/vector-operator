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

// Config Optimization tests verify that with spec.agent.configOptimization.sources
// enabled the operator collapses kubernetes_logs sources with identical settings
// into a single source with namespace-based routing, and the resulting config
// passes the vector config check.
var _ = Describe("Config Optimization", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-config-optimization")

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.DeleteResource("namespace", "test-config-optimization-extra")
		for i := 1; i <= 18; i++ {
			f.DeleteResource("namespace", fmt.Sprintf("test-config-opt-hier-%02d", i))
		}
		f.Teardown()
		f.PrintMetrics()
	})

	Context("Sources optimization", func() {
		It("should collapse kubernetes_logs sources of multiple pipelines into one", func() {
			By("deploying Vector Agent with sources optimization enabled")
			f.ApplyTestData("config-optimization/agent.yaml")

			// Give controller time to process Vector CR Create event and create daemonset
			time.Sleep(5 * time.Second)

			By("creating 3 pipelines with identical kubernetes_logs sources")
			for i := 1; i <= 3; i++ {
				f.ApplyTestDataWithVars("config-optimization/pipeline-template.yaml",
					map[string]string{"{{INDEX}}": fmt.Sprintf("opt-pipeline-%d", i)})
			}

			By("waiting for all pipelines to become valid")
			f.WaitForPipelineValid("opt-pipeline-1")
			f.WaitForPipelineValid("opt-pipeline-2")
			f.WaitForPipelineValid("opt-pipeline-3")

			By("verifying sources are collapsed into an optimized source with a router")
			Eventually(func() error {
				return f.VerifyAgentConfigContains("optimized-agent",
					`"optimizedSource-`,
					`"optimizedRouter-`,
					`kubernetes.io/metadata.name in (`,
				)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying the original per-pipeline sources are gone from the config")
			Eventually(func() error {
				return f.VerifyAgentConfigNotContains("optimized-agent",
					fmt.Sprintf(`"kubernetes.io/metadata.name=%s"`, f.Namespace()),
				)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying pipeline sinks are still present and routed")
			Eventually(func() error {
				return f.VerifyAgentHasPipeline("optimized-agent", "opt-pipeline-1")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})

		It("should collapse sources across namespaces into one selector", func() {
			By("creating a pipeline in an extra namespace")
			f.ApplyTestDataWithoutNamespaceReplacement("config-optimization/extra-ns.yaml")
			f.ApplyTestDataWithoutNamespaceReplacement("config-optimization/pipeline-extra-ns.yaml")
			f.WaitForPipelineValidInNamespace("extra-ns-pipeline", "test-config-optimization-extra")

			By("verifying the optimized source covers both namespaces")
			Eventually(func() error {
				return f.VerifyAgentConfigContains("optimized-agent",
					"test-config-optimization-extra",
					`kubernetes.io/metadata.name in (`,
				)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})

		It("should use hierarchical routing for large groups", func() {
			By("creating pipelines in 18 namespaces (over the flat routing threshold)")
			for i := 1; i <= 18; i++ {
				f.ApplyTestDataWithVarsWithoutNamespaceReplacement("config-optimization/pipeline-hier-template.yaml",
					map[string]string{"{{NS}}": fmt.Sprintf("test-config-opt-hier-%02d", i)})
			}
			for i := 1; i <= 18; i++ {
				f.WaitForPipelineValidInNamespace("hier-pipeline", fmt.Sprintf("test-config-opt-hier-%02d", i))
			}

			By("verifying the bucketed two-level routing is generated and accepted by the agent")
			Eventually(func() error {
				return f.VerifyAgentConfigContains("optimized-agent",
					`"optimizedBucketer-`,
					`mod(parse_int!`,
					`-l1`,
				)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying the agent pod runs with the new config")
			pods, err := f.GetAgentPods("optimized-agent")
			Expect(err).NotTo(HaveOccurred())
			Expect(pods).NotTo(BeEmpty())
			f.WaitForPodReady(pods[0])
		})

		It("should keep sources with different settings standalone", func() {
			By("creating a pipeline with an extra_label_selector")
			f.ApplyTestData("config-optimization/pipeline-with-selector.yaml")
			f.WaitForPipelineValid("selector-pipeline")

			By("verifying the source with distinct settings is not collapsed")
			Eventually(func() error {
				return f.VerifyAgentConfigContains("optimized-agent",
					fmt.Sprintf("%s-selector-pipeline-logs", f.Namespace()),
					`app=selector-test`,
				)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})
	})
})
