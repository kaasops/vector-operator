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

const ccEnvDaemonSet = "ccenv-agent"

// ConfigCheck Env removes from a Vector an env var its pipeline references. The
// pipelines are unchanged, so the generated config is too; the config check must still
// run, fail, and keep the DaemonSet on the old env instead of rolling out agents that
// cannot resolve the config.
var _ = Describe("ConfigCheck Env", Label(config.LabelRegression), Ordered, func() {
	f := framework.NewUniqueFramework("test-configcheck-env")

	BeforeAll(func() {
		f.Setup()

		By("deploying the agent with LINE_PREFIX")
		f.ApplyTestData("configcheck-env/agent.yaml")
		Eventually(func() error {
			_, err := f.Kubectl().Get("daemonset", ccEnvDaemonSet)
			return err
		}, config.DeploymentCreateTimeout, config.DefaultPollInterval).Should(Succeed())

		f.ApplyTestData("configcheck-env/pipeline.yaml")
		f.WaitForPipelineValid("env-ref")

		// The Vector applies the pipeline in its own reconcile, with its own config check;
		// wait for it so the env removal below is the only change left.
		By("waiting for the agent to take the pipeline")
		Eventually(func() error {
			return f.VerifyAgentHasPipeline("ccenv", "env-ref")
		}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
		Eventually(func() (string, error) {
			return f.Kubectl().GetWithJsonPath("vector", "ccenv", ".status.configCheckResult")
		}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Equal("true"))
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	It("should fail the config check and keep the agents when a referenced env var is removed", func() {
		By("removing LINE_PREFIX from the agent")
		f.ApplyTestData("configcheck-env/agent-without-env.yaml")

		Eventually(func() (string, error) {
			return f.Kubectl().GetWithJsonPath("vector", "ccenv", ".status.configCheckResult")
		}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Equal("false"))
		Expect(f.Kubectl().GetWithJsonPath("vector", "ccenv", ".status.reason")).To(ContainSubstring("LINE_PREFIX"))

		By("checking the DaemonSet still has LINE_PREFIX")
		Expect(f.Kubectl().GetWithJsonPath("daemonset", ccEnvDaemonSet,
			".spec.template.spec.containers[0].env")).To(ContainSubstring("LINE_PREFIX"))
	})
})
