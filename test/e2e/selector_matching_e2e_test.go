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
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
	"github.com/kaasops/vector-operator/test/utils"
)

// Resource names used in selector matching tests
const (
	selectorTestAgent            = "test-agent"
	labeledPipelineName          = "labeled-pipeline"
	unlabeledPipelineName        = "unlabeled-pipeline"
	relabeledPipelineName        = "relabeled-pipeline"
	matchingAggregatorName       = "matching-aggregator"
	nonMatchingAggregatorName    = "non-matching-aggregator"
	noSelectorAggregatorName     = "no-selector-aggregator"
	matchingAggregatorDeployment = matchingAggregatorName + "-aggregator"
	nonMatchingAggregatorDeploy  = nonMatchingAggregatorName + "-aggregator"
	noSelectorAggregatorDeploy   = noSelectorAggregatorName + "-aggregator"
)

// Selector Matching tests verify that ClusterVectorPipeline is validated only against
// ClusterVectorAggregator instances whose selector matches the pipeline's labels.
// This is a regression test for issue #201 / PR #208.
//
// The bug was: configcheck validated CVP against ALL CVA instances instead of only
// those whose selector matches the CVP's labels. This caused validation failures
// when a CVP used features/config only available on specific aggregators.
var _ = Describe("Selector Matching", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-selector-matching")

	BeforeAll(func() {
		f.Setup()

		By("deploying Vector Agent")
		f.ApplyTestData("selector-matching/agent.yaml")

		// Give controller time to process Vector CR and create DaemonSet
		time.Sleep(5 * time.Second)
	})

	AfterAll(func() {
		// Clean up cluster-scoped resources
		By("cleaning up ClusterVectorPipelines")
		f.DeleteClusterResource("clustervectorpipeline", labeledPipelineName)
		f.DeleteClusterResource("clustervectorpipeline", unlabeledPipelineName)
		f.DeleteClusterResource("clustervectorpipeline", relabeledPipelineName)

		By("cleaning up ClusterVectorAggregators")
		f.DeleteClusterResource("clustervectoraggregator", matchingAggregatorName)
		f.DeleteClusterResource("clustervectoraggregator", nonMatchingAggregatorName)
		f.DeleteClusterResource("clustervectoraggregator", noSelectorAggregatorName)

		f.Teardown()
		f.PrintMetrics()
	})

	Context("CVP with labels matching CVA selector", func() {
		It("should validate CVP only against matching CVA", func() {
			By("deploying ClusterVectorAggregator with matching selector (team: platform)")
			f.ApplyTestData("selector-matching/cva-matching.yaml")
			f.WaitForDeploymentReady(matchingAggregatorDeployment)

			By("deploying ClusterVectorAggregator with non-matching selector (team: backend)")
			f.ApplyTestData("selector-matching/cva-non-matching.yaml")
			f.WaitForDeploymentReady(nonMatchingAggregatorDeploy)

			By("creating ClusterVectorPipeline with label team: platform")
			f.ApplyTestDataWithoutNamespaceReplacement("selector-matching/cvp-with-labels.yaml")

			By("waiting for CVP to become valid")
			// The pipeline should be valid because it matches "matching-aggregator"
			// Before the fix in PR #208, the pipeline would be validated against ALL aggregators,
			// potentially causing validation failures against non-matching aggregators
			f.WaitForClusterPipelineValid(labeledPipelineName)

			By("verifying CVP role is agent (kubernetes_logs source)")
			role := f.GetClusterPipelineStatus(labeledPipelineName, "role")
			Expect(role).To(Equal("agent"), "Pipeline with kubernetes_logs source should have agent role")

			By("verifying CVP is processed by agent")
			Eventually(func() error {
				return f.VerifyAgentHasClusterPipeline(selectorTestAgent, labeledPipelineName)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed(),
				"Pipeline should be in agent's config")
		})
	})

	Context("CVP relabeled to another CVA selector", func() {
		It("should move the pipeline to the newly matching CVA", func() {
			By("creating aggregator ClusterVectorPipeline with label team: platform")
			f.ApplyTestDataWithoutNamespaceReplacement("selector-matching/cvp-aggregator.yaml")
			f.WaitForClusterPipelineValid(relabeledPipelineName)
			Eventually(func() error {
				return f.VerifyAggregatorHasClusterPipeline(matchingAggregatorName, relabeledPipelineName)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("changing only the label to team: backend")
			// kubectl label keeps metadata.generation and the annotations (kubectl apply
			// would rewrite last-applied-configuration), so only the labels change.
			cmd := exec.Command("kubectl", "label", "clustervectorpipeline", relabeledPipelineName, "team=backend", "--overwrite")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the pipeline moved to the backend aggregator")
			Eventually(func() error {
				return f.VerifyAggregatorHasClusterPipeline(nonMatchingAggregatorName, relabeledPipelineName)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
			Eventually(func() error {
				return f.VerifyAggregatorHasClusterPipeline(matchingAggregatorName, relabeledPipelineName)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).ShouldNot(Succeed())
		})
	})

	Context("CVP without labels with CVA without selector", func() {
		It("should validate CVP against CVA without selector", func() {
			By("deploying ClusterVectorAggregator without selector")
			f.ApplyTestData("selector-matching/cva-no-selector.yaml")
			f.WaitForDeploymentReady(noSelectorAggregatorDeploy)

			By("creating ClusterVectorPipeline without labels")
			f.ApplyTestDataWithoutNamespaceReplacement("selector-matching/cvp-no-labels.yaml")

			By("waiting for CVP to become valid")
			// Pipeline without labels should match aggregator without selector
			f.WaitForClusterPipelineValid(unlabeledPipelineName)

			By("verifying CVP role is agent (kubernetes_logs source)")
			role := f.GetClusterPipelineStatus(unlabeledPipelineName, "role")
			Expect(role).To(Equal("agent"), "Pipeline with kubernetes_logs source should have agent role")

			By("verifying CVP is processed by agent")
			Eventually(func() error {
				return f.VerifyAgentHasClusterPipeline(selectorTestAgent, unlabeledPipelineName)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed(),
				"Pipeline should be in agent's config")
		})
	})
})
