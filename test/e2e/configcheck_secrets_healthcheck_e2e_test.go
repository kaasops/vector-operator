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

// configCheckSecretsImage is the first Vector release whose `vector validate` resolves
// SECRET[...] before running sink healthchecks. Older releases sign the healthcheck
// with the literal placeholder, so a sink with SECRET[...] credentials fails configcheck.
const configCheckSecretsImage = "timberio/vector:0.59.0-alpine"

// ConfigCheck Secrets Healthcheck runs an aws_s3 sink whose credentials come only via
// SECRET[...] against a SigV4-checking S3 mock, with the sink healthcheck enabled. The
// configcheck pod must resolve the references to pass; a sibling pipeline backed by a
// wrong secret key must fail, which shows the healthcheck really reached the mock.
var _ = Describe("ConfigCheck Secrets Healthcheck", Label(config.LabelRegression, config.LabelSlow), Ordered, Serial, func() {
	f := framework.NewUniqueFramework("test-configcheck-secrets-healthcheck")

	BeforeAll(func() {
		f.Setup()

		By("deploying the S3 mock, the credential Secrets and the agent")
		f.ApplyTestData("configcheck-secrets-healthcheck/s3mock.yaml")
		f.WaitForDeploymentReady("s3")
		f.ApplyTestData("configcheck-secrets-healthcheck/secrets.yaml")
		f.ApplyTestDataWithVars("configcheck-secrets-healthcheck/agent.yaml",
			map[string]string{"CONFIGCHECK_IMAGE": configCheckSecretsImage})

		// Pipelines that arrive before the operator has seen this Vector can be marked
		// valid without a configcheck, so wait until the Vector has its DaemonSet.
		By("waiting for the agent DaemonSet")
		Eventually(func() error {
			_, err := f.Kubectl().Get("daemonset", "cchc-agent")
			return err
		}, config.DeploymentCreateTimeout, config.DefaultPollInterval).Should(Succeed())

		f.ApplyTestData("configcheck-secrets-healthcheck/pipelines.yaml")
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	It("should pass configcheck when the healthcheck signs with resolved SECRET[...] keys", func() {
		By("waiting for pipeline s3-valid to become valid")
		Eventually(func() (string, error) {
			return f.Kubectl().GetWithJsonPath("vectorpipeline", "s3-valid", ".status.configCheckResult")
		}, config.PipelineValidTimeout, config.SlowPollInterval).Should(Equal("true"), func() string {
			reason, _ := f.Kubectl().GetWithJsonPath("vectorpipeline", "s3-valid", ".status.reason")
			return "s3-valid reason: " + reason
		})
	})

	It("should fail configcheck when the resolved secret key is wrong", func() {
		f.WaitForPipelineInvalid("s3-wrong")
		Expect(f.GetPipelineStatus("s3-wrong", "reason")).To(ContainSubstring("Invalid credentials"),
			"the failure must come from the sink healthcheck, not from config loading")
	})
})
