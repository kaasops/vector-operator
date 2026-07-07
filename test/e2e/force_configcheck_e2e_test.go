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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
)

const (
	forceConfigCheckAgent           = "force-cc-agent"
	forceConfigCheckPipeline        = "force-cc-pipeline"
	forceConfigCheckClusterPipeline = "force-cc-cvp"
)

// Force ConfigCheck tests verify that the force-configcheck annotation
// triggers configcheck even when the pipeline spec has not changed.
var _ = Describe("Force ConfigCheck Annotation", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-force-configcheck")

	BeforeAll(func() {
		f.Setup()

		By("deploying Vector Agent")
		f.ApplyTestData("force-configcheck/agent.yaml")
		time.Sleep(5 * time.Second)
	})

	AfterAll(func() {
		f.DeleteClusterResource("clustervectorpipeline", forceConfigCheckClusterPipeline)
		f.Teardown()
		f.PrintMetrics()
	})

	Context("pipeline without annotation", func() {
		It("should validate pipeline normally", func() {
			By("creating a VectorPipeline without force-configcheck annotation")
			f.ApplyTestData("force-configcheck/pipeline.yaml")

			By("waiting for pipeline to become valid")
			f.WaitForPipelineValid(forceConfigCheckPipeline)

			By("verifying agent processes the pipeline")
			Eventually(func() error {
				return f.VerifyAgentHasPipeline(forceConfigCheckAgent, forceConfigCheckPipeline)
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		})
	})

	Context("adding force-configcheck annotation", func() {
		It("should re-run configcheck when annotation is added", func() {
			By("recording current pipeline hash")
			hashBefore := f.GetPipelineStatus(forceConfigCheckPipeline, "LastAppliedPipelineHash")
			Expect(hashBefore).NotTo(BeEmpty(), "Pipeline should have a hash after initial validation")

			By("applying pipeline with force-configcheck annotation set to v1")
			f.ApplyTestData("force-configcheck/pipeline-with-annotation.yaml")

			By("waiting for pipeline to become valid again")
			f.WaitForPipelineValid(forceConfigCheckPipeline)

			By("verifying pipeline hash changed due to annotation")
			Eventually(func() string {
				return f.GetPipelineStatus(forceConfigCheckPipeline, "LastAppliedPipelineHash")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(Equal(hashBefore),
				"Hash should change after adding force-configcheck annotation")
		})
	})

	Context("same annotation value", func() {
		It("should not re-run configcheck for the same annotation value", func() {
			By("recording current pipeline hash")
			hashBefore := f.GetPipelineStatus(forceConfigCheckPipeline, "LastAppliedPipelineHash")

			By("re-applying pipeline with same annotation value v1")
			f.ApplyTestData("force-configcheck/pipeline-with-annotation.yaml")

			By("waiting briefly for any reconciliation")
			time.Sleep(5 * time.Second)

			By("verifying hash has not changed (no configcheck re-run)")
			hashAfter := f.GetPipelineStatus(forceConfigCheckPipeline, "LastAppliedPipelineHash")
			Expect(hashAfter).To(Equal(hashBefore),
				"Hash should not change when annotation value is the same")
		})
	})

	Context("changed annotation value", func() {
		It("should re-run configcheck when annotation value changes", func() {
			By("recording current pipeline hash")
			hashBefore := f.GetPipelineStatus(forceConfigCheckPipeline, "LastAppliedPipelineHash")

			By("applying pipeline with changed annotation value v2")
			f.ApplyTestData("force-configcheck/pipeline-with-annotation-v2.yaml")

			By("waiting for pipeline to become valid again")
			f.WaitForPipelineValid(forceConfigCheckPipeline)

			By("verifying pipeline hash changed due to new annotation value")
			Eventually(func() string {
				return f.GetPipelineStatus(forceConfigCheckPipeline, "LastAppliedPipelineHash")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(Equal(hashBefore),
				"Hash should change after changing force-configcheck annotation value")
		})
	})

	Context("ClusterVectorPipeline without annotation", func() {
		It("should validate CVP normally", func() {
			By("creating a ClusterVectorPipeline without force-configcheck annotation")
			f.ApplyTestDataWithoutNamespaceReplacement("force-configcheck/cluster-pipeline.yaml")

			By("waiting for CVP to become valid")
			f.WaitForClusterPipelineValid(forceConfigCheckClusterPipeline)
		})
	})

	Context("ClusterVectorPipeline with annotation", func() {
		It("should re-run configcheck when annotation is added to CVP", func() {
			By("recording current CVP hash")
			hashBefore := f.GetClusterPipelineStatus(forceConfigCheckClusterPipeline, "LastAppliedPipelineHash")
			Expect(hashBefore).NotTo(BeEmpty(), "CVP should have a hash after initial validation")

			By("applying CVP with force-configcheck annotation set to v1")
			f.ApplyTestDataWithoutNamespaceReplacement("force-configcheck/cluster-pipeline-with-annotation.yaml")

			By("waiting for CVP to become valid again")
			f.WaitForClusterPipelineValid(forceConfigCheckClusterPipeline)

			By("verifying CVP hash changed due to annotation")
			Eventually(func() string {
				return f.GetClusterPipelineStatus(forceConfigCheckClusterPipeline, "LastAppliedPipelineHash")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(Equal(hashBefore),
				"CVP hash should change after adding force-configcheck annotation")
		})
	})

	Context("ClusterVectorPipeline changed annotation value", func() {
		It("should re-run configcheck when CVP annotation value changes", func() {
			By("recording current CVP hash")
			hashBefore := f.GetClusterPipelineStatus(forceConfigCheckClusterPipeline, "LastAppliedPipelineHash")

			By("applying CVP with changed annotation value v2")
			f.ApplyTestDataWithoutNamespaceReplacement("force-configcheck/cluster-pipeline-with-annotation-v2.yaml")

			By("waiting for CVP to become valid again")
			f.WaitForClusterPipelineValid(forceConfigCheckClusterPipeline)

			By("verifying CVP hash changed due to new annotation value")
			Eventually(func() string {
				return f.GetClusterPipelineStatus(forceConfigCheckClusterPipeline, "LastAppliedPipelineHash")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(Equal(hashBefore),
				"CVP hash should change after changing force-configcheck annotation value")
		})
	})
})
