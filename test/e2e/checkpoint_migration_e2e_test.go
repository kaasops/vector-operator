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
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
	"github.com/kaasops/vector-operator/test/utils"
)

// setCheckpointMigration toggles the config optimization together with the
// checkpoint migration operator flags and waits for the operator rollout.
func setCheckpointMigration(enable bool) {
	patch := fmt.Sprintf(`[{"op":"add","path":"/spec/template/spec/containers/0/args","value":["--enable-config-optimization","--enable-checkpoint-migration","--checkpoint-merger-image=%s"]}]`, mergerImage)
	if !enable {
		patch = `[{"op":"add","path":"/spec/template/spec/containers/0/args","value":[]}]`
	}
	cmd := exec.Command("kubectl", "-n", operatorNamespace, "patch", "deployment", "vector-operator", "--type=json", "-p", patch)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	cmd = exec.Command("kubectl", "-n", operatorNamespace, "rollout", "status", "deployment/vector-operator", "--timeout=120s")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

// Checkpoint Migration tests verify that with --enable-checkpoint-migration the
// agent config Secret name is bound to the optimization mode (so a mode switch
// rolls the DaemonSet instead of a live reload) and the checkpoint-merger init
// container consolidates vector file checkpoints before the agent starts.
var _ = Describe("Checkpoint Migration", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-checkpoint-migration")

	daemonset := "daemonset/optimized-agent-agent"
	configVolumeJsonPath := `jsonpath={.spec.template.spec.volumes[?(@.name=="config")].secret.secretName}`

	configVolumeSecret := func() string {
		cmd := exec.Command("kubectl", "-n", f.Namespace(), "get", daemonset, "-o", configVolumeJsonPath)
		out, err := utils.Run(cmd)
		if err != nil {
			return ""
		}
		return string(out)
	}

	BeforeAll(func() {
		f.Setup()
		setCheckpointMigration(true)
	})

	AfterAll(func() {
		setCheckpointMigration(false)
		f.Teardown()
		f.PrintMetrics()
	})

	It("should bind the agent config secret to the optimization mode", func() {
		By("deploying Vector Agent and two pipelines")
		f.ApplyTestData("config-optimization/agent.yaml")
		time.Sleep(5 * time.Second)
		for i := 1; i <= 2; i++ {
			f.ApplyTestDataWithVars("config-optimization/pipeline-template.yaml",
				map[string]string{"{{INDEX}}": fmt.Sprintf("cm-pipeline-%d", i)})
		}

		By("verifying the optimized config lands in the -opt secret")
		Eventually(func() error {
			return f.VerifySecretConfigContains("optimized-agent-agent-opt", `"optimizedSource-`)
		}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

		By("verifying the legacy secret holds the non-optimized config")
		Eventually(func() error {
			return f.VerifySecretConfigNotContains("optimized-agent-agent", `"optimizedSource-`)
		}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

		By("verifying the DaemonSet mounts the -opt secret and has the merger init container")
		Eventually(configVolumeSecret, config.ServiceCreateTimeout, config.DefaultPollInterval).
			Should(Equal("optimized-agent-agent-opt"))
		cmd := exec.Command("kubectl", "-n", f.Namespace(), "get", daemonset,
			"-o", `jsonpath={.spec.template.spec.initContainers[*].name}`)
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("checkpoint-merger"))
	})

	It("should roll the DaemonSet to the legacy secret on opt-out", func() {
		By("annotating the Vector CR with config-optimization=disabled")
		cmd := exec.Command("kubectl", "-n", f.Namespace(), "annotate", "vector", "optimized-agent",
			"vector-operator.kaasops.io/config-optimization=disabled", "--overwrite")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the DaemonSet to switch to the legacy secret")
		Eventually(configVolumeSecret, config.ServiceCreateTimeout, config.DefaultPollInterval).
			Should(Equal("optimized-agent-agent"))
		cmd = exec.Command("kubectl", "-n", f.Namespace(), "rollout", "status", daemonset, "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("verifying the checkpoint merger ran before vector started")
		pods, err := f.GetAgentPods("optimized-agent")
		Expect(err).NotTo(HaveOccurred())
		Expect(pods).NotTo(BeEmpty())
		cmd = exec.Command("kubectl", "-n", f.Namespace(), "logs", pods[0], "-c", "checkpoint-merger")
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(ContainSubstring("build info"))
	})

	It("should restore the optimized mode when the annotation is removed", func() {
		cmd := exec.Command("kubectl", "-n", f.Namespace(), "annotate", "vector", "optimized-agent",
			"vector-operator.kaasops.io/config-optimization-")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(configVolumeSecret, config.ServiceCreateTimeout, config.DefaultPollInterval).
			Should(Equal("optimized-agent-agent-opt"))
		cmd = exec.Command("kubectl", "-n", f.Namespace(), "rollout", "status", daemonset, "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})
})
