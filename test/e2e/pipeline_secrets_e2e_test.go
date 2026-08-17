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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
)

// flatSecretAssetKey mirrors the (unexported) flatKey() naming rule from
// internal/config/secrets.go: "<namespace>-<pipeline>-<alias>-<key>" with every
// dash turned into an underscore. It is duplicated here (not imported) because the
// source function is unexported; keep it in lockstep with secrets.go if that naming
// scheme ever changes.
func flatSecretAssetKey(namespace, pipelineName, alias, key string) string {
	joined := namespace + "-" + pipelineName + "-" + alias + "-" + key
	return strings.ReplaceAll(joined, "-", "_")
}

// flatSecretAssetKeyClusterScoped mirrors flatSecretAssetKey for a ClusterVectorPipeline,
// where GetNamespace() is always "" and generateName("", name) == name: there is no
// namespace segment (and no leading separator) in the flat key at all.
func flatSecretAssetKeyClusterScoped(pipelineName, alias, key string) string {
	joined := pipelineName + "-" + alias + "-" + key
	return strings.ReplaceAll(joined, "-", "_")
}

// Pipeline Secrets tests verify that a (Cluster)VectorPipeline can declare a named
// secret backend (spec.secret), reference it from component options via
// SECRET[alias.key], and have the operator rewrite that reference to
// SECRET[k8s.<flat>], add a directory secret backend, and materialize the resolved
// value into an aggregated "-secret-assets" Secret mounted into the agent DaemonSet -
// without ever putting the plaintext value into the agent's config Secret. It also
// verifies Secret rotation propagates without touching the config Secret, and that
// referencing an undeclared alias fails config build with a reason naming the alias.
var _ = Describe("Pipeline Secrets", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-pipeline-secrets")

	const (
		vectorName   = "vagent"
		pipelineName = "es-pipeline"
		secretName   = "creds"
		configSecret = vectorName + "-agent"
		assetsSecret = configSecret + "-secret-assets"
		alias        = "es"
		usernameKey  = "username"
		passwordKey  = "password"
		initialUser  = "es-user-initial"
		initialPass  = "es-pass-initial"
		rotatedUser  = "es-user-rotated"
		undeclaredCR = "undeclared-secret-pipeline"

		cvpName     = "cvp-secrets-e2e"
		cvpUsername = "cvp-es-user"
		cvpPassword = "cvp-es-pass"
	)

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		// ClusterVectorPipeline is cluster-scoped: namespace teardown below does not
		// remove it, so it must be deleted explicitly to avoid leaking into other suites.
		f.DeleteClusterResource("clustervectorpipeline", cvpName)

		f.Teardown()
		f.PrintMetrics()
	})

	Context("secret reference resolution", func() {
		It("should rewrite SECRET[] refs and materialize the aggregated secret-assets Secret", func() {
			By("creating the backing Secret and the Vector agent")
			f.ApplyTestData("pipeline-secrets/secret.yaml")
			f.ApplyTestData("pipeline-secrets/agent.yaml")

			By("creating a pipeline that declares spec.secret and references SECRET[es.username/password]")
			f.ApplyTestData("pipeline-secrets/pipeline.yaml")
			f.WaitForPipelineValid(pipelineName)

			usernameFlat := flatSecretAssetKey(f.Namespace(), pipelineName, alias, usernameKey)
			passwordFlat := flatSecretAssetKey(f.Namespace(), pipelineName, alias, passwordKey)

			By("verifying the agent config Secret was rewritten to SECRET[k8s.<flat>] with a directory backend")
			Eventually(func() error {
				return f.VerifyAgentConfigContains(vectorName,
					"SECRET[k8s.",
					usernameFlat,
					passwordFlat,
					`"type":"directory"`,
					"/etc/vector/secrets",
				)
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying the plaintext secret value never appears in the agent config Secret")
			Expect(f.VerifyAgentConfigNotContains(vectorName, initialUser, initialPass)).To(Succeed())

			By("verifying the aggregated secret-assets Secret carries the resolved values under flat keys")
			var assets map[string][]byte
			Eventually(func() error {
				var err error
				assets, err = f.GetSecret(assetsSecret)
				return err
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Succeed())
			Expect(string(assets[usernameFlat])).To(Equal(initialUser))
			Expect(string(assets[passwordFlat])).To(Equal(initialPass))

			By("verifying status.relatedSecretsHash is set")
			Eventually(func() string {
				return f.GetPipelineStatus(pipelineName, "relatedSecretsHash")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(BeEmpty())
		})

		It("should propagate Secret rotation to secret-assets without touching the config Secret", func() {
			usernameFlat := flatSecretAssetKey(f.Namespace(), pipelineName, alias, usernameKey)

			By("capturing the config Secret and relatedSecretsHash before rotation")
			configBefore, err := f.GetSecret(configSecret)
			Expect(err).NotTo(HaveOccurred())
			hashBefore := f.GetPipelineStatus(pipelineName, "relatedSecretsHash")
			Expect(hashBefore).NotTo(BeEmpty())

			By("rotating the backing Secret's username")
			f.ApplyTestData("pipeline-secrets/secret-rotated.yaml")

			By("waiting for relatedSecretsHash to change")
			Eventually(func() string {
				return f.GetPipelineStatus(pipelineName, "relatedSecretsHash")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(Equal(hashBefore))

			By("waiting for the aggregated secret-assets Secret to carry the rotated value")
			Eventually(func() (string, error) {
				assets, err := f.GetSecret(assetsSecret)
				if err != nil {
					return "", err
				}
				return string(assets[usernameFlat]), nil
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Equal(rotatedUser))

			By("verifying the agent config Secret content is unchanged by the rotation")
			configAfter, err := f.GetSecret(configSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(configAfter).To(Equal(configBefore), "agent config Secret must not change on secret rotation (only secret-assets should)")
		})
	})

	Context("undeclared secret alias", func() {
		It("should fail config build with a reason naming the undeclared alias", func() {
			By("creating a pipeline that references SECRET[undeclared.key] without declaring spec.secret")
			f.ApplyTestData("pipeline-secrets/pipeline-undeclared.yaml")

			By("waiting for the pipeline to become invalid")
			f.WaitForPipelineInvalid(undeclaredCR)

			By("verifying the reason names the undeclared alias")
			reason := f.GetPipelineStatus(undeclaredCR, "reason")
			Expect(reason).To(ContainSubstring("undeclared"), fmt.Sprintf("reason should mention the undeclared alias, got: %s", reason))
		})
	})

	Context("cluster-scoped secret reference resolution", func() {
		It("should rewrite SECRET[] refs for a ClusterVectorPipeline with a namespace-less flat key", func() {
			By("creating the backing Secret for the CVP's \"es\" backend")
			f.ApplyTestData("pipeline-secrets/cvp-secret.yaml")

			By("creating a ClusterVectorPipeline that declares spec.secret with an explicit backend namespace")
			f.ApplyTestData("pipeline-secrets/cvp-pipeline.yaml")
			f.WaitForClusterPipelineValid(cvpName)

			usernameFlat := flatSecretAssetKeyClusterScoped(cvpName, alias, usernameKey)
			passwordFlat := flatSecretAssetKeyClusterScoped(cvpName, alias, passwordKey)

			By("verifying the agent config Secret was rewritten to SECRET[k8s.<flat>], with no namespace segment, plus a directory backend")
			Eventually(func() error {
				return f.VerifyAgentConfigContains(vectorName,
					fmt.Sprintf("SECRET[k8s.%s]", usernameFlat),
					fmt.Sprintf("SECRET[k8s.%s]", passwordFlat),
					`"type":"directory"`,
					"/etc/vector/secrets",
				)
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying the plaintext secret value never appears in the agent config Secret")
			Expect(f.VerifyAgentConfigNotContains(vectorName, cvpUsername, cvpPassword)).To(Succeed())

			By("verifying the aggregated secret-assets Secret carries the resolved values under the flat keys")
			var assets map[string][]byte
			Eventually(func() error {
				var err error
				assets, err = f.GetSecret(assetsSecret)
				return err
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Succeed())
			Expect(string(assets[usernameFlat])).To(Equal(cvpUsername))
			Expect(string(assets[passwordFlat])).To(Equal(cvpPassword))

			By("verifying status.relatedSecretsHash is set on the CVP")
			Eventually(func() string {
				return f.GetClusterPipelineStatus(cvpName, "relatedSecretsHash")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(BeEmpty())
		})
	})
})
