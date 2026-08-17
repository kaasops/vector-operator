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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
)

const (
	// secretRuntimeTimeout bounds the path from "resource applied" to "value visible in
	// the agent's stdout": pipeline validation + configcheck, DaemonSet rollout, Vector
	// startup and kubernetes_logs discovery.
	secretRuntimeTimeout = 5 * time.Minute
	// secretRotationTimeout additionally covers kubelet's mounted-Secret sync period
	// (up to ~60-90s on top of its sync interval) and Vector's --watch-config reload.
	secretRotationTimeout = 6 * time.Minute
	secretRuntimePoll     = 5 * time.Second
)

// podFieldOrEmpty reads a single jsonpath field off a pod, returning "" when the pod
// is gone or the field is unset - callers use it inside Eventually blocks where both
// are just "not there yet".
func podFieldOrEmpty(f *framework.Framework, podName, jsonPath string) string {
	value, err := f.Kubectl().GetWithJsonPath("pod", podName, jsonPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

// agentPodOnNode returns the vectorName agent pod scheduled on node. The DaemonSet
// runs one agent per node and only the one co-located with the log producer tails its
// container log, so the assertions must read that pod's stdout specifically.
func agentPodOnNode(f *framework.Framework, vectorName, node string) (string, error) {
	pods, err := f.GetAgentPods(vectorName)
	if err != nil {
		return "", err
	}
	for _, pod := range pods {
		if podFieldOrEmpty(f, pod, ".spec.nodeName") == node {
			return pod, nil
		}
	}
	return "", fmt.Errorf("no %s agent pod found on node %q (agent pods: %v)", vectorName, node, pods)
}

// Pipeline Secrets Runtime exercises the data plane the "Pipeline Secrets" spec cannot
// see: that the running Vector process actually retrieves the declared secret through
// the mounted directory backend, and that rotating the backing Secret reaches that
// already-running process. The probe pipeline stamps every event with
// SECRET[probe.token] and prints it to the agent's stdout, making the resolved value
// observable end-to-end. Rotation must arrive as a live reload - no config bytes
// change, no pod rollout - which is asserted explicitly.
var _ = Describe("Pipeline Secrets Runtime", Label(config.LabelRegression, config.LabelSlow), Ordered, func() {
	f := framework.NewUniqueFramework("test-pipeline-secrets-runtime")

	const (
		vectorName   = "rtprobe"
		pipelineName = "runtime-probe-pipeline"
		producerPod  = "log-producer"
		configSecret = vectorName + "-agent"
		assetsSecret = configSecret + "-secret-assets"
		alias        = "probe"
		tokenKey     = "token"
		initialToken = "runtime-probe-v1"
		rotatedToken = "runtime-probe-v2"

		restartCountPath = ".status.containerStatuses[*].restartCount"
	)

	var (
		producerNode string
		agentPod     string
		agentPodUID  string
		agentRestart string
		configRV     string
	)

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	Context("secret retrieval by the running Vector", func() {
		It("should stamp events with the value the agent read from the mounted secret backend", func() {
			By("creating the backing Secret, the log producer and the Vector agent")
			f.ApplyTestData("pipeline-secrets-runtime/secret.yaml")
			f.ApplyTestData("pipeline-secrets-runtime/producer.yaml")
			f.ApplyTestData("pipeline-secrets-runtime/agent.yaml")
			f.WaitForPodReady(producerPod)

			producerNode = podFieldOrEmpty(f, producerPod, ".spec.nodeName")
			Expect(producerNode).NotTo(BeEmpty(), "log producer must be scheduled before the agent can tail it")

			By("creating the probe pipeline that references SECRET[probe.token] from a remap transform")
			f.ApplyTestData("pipeline-secrets-runtime/pipeline.yaml")
			f.WaitForPipelineValid(pipelineName)

			tokenFlat := flatSecretAssetKey(f.Namespace(), pipelineName, alias, tokenKey)

			By("verifying the operator rewrote the reference and materialized the resolved value")
			Eventually(func() error {
				return f.VerifyAgentConfigContains(vectorName, fmt.Sprintf("SECRET[k8s.%s]", tokenFlat))
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Succeed())
			Eventually(func() (string, error) {
				assets, err := f.GetSecret(assetsSecret)
				if err != nil {
					return "", err
				}
				return string(assets[tokenFlat]), nil
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Equal(initialToken))

			By("waiting for the resolved value to appear in the agent's stdout")
			// The pod is resolved inside the loop: adding the secret-assets volume changes
			// the pod template, so the agent pod is replaced while this is converging.
			stamped := fmt.Sprintf(`"probe_token":"%s"`, initialToken)
			Eventually(func() (string, error) {
				pod, err := agentPodOnNode(f, vectorName, producerNode)
				if err != nil {
					return "", err
				}
				return f.GetPodLogs(pod)
			}, secretRuntimeTimeout, secretRuntimePoll).Should(ContainSubstring(stamped),
				"the running Vector must resolve SECRET[] through the mounted directory backend")
		})
	})

	Context("rotation reaching the running Vector", func() {
		It("should pick up a rotated Secret by live reload, without a config change or a pod restart", func() {
			By("capturing the agent pod identity and the config Secret resourceVersion before rotation")
			var err error
			agentPod, err = agentPodOnNode(f, vectorName, producerNode)
			Expect(err).NotTo(HaveOccurred())
			agentPodUID = podFieldOrEmpty(f, agentPod, ".metadata.uid")
			Expect(agentPodUID).NotTo(BeEmpty())
			agentRestart = podFieldOrEmpty(f, agentPod, restartCountPath)

			configRV, err = f.Kubectl().GetWithJsonPath("secret", configSecret, ".metadata.resourceVersion")
			Expect(err).NotTo(HaveOccurred())
			configRV = strings.TrimSpace(configRV)
			Expect(configRV).NotTo(BeEmpty())

			By("rotating the backing Secret")
			f.ApplyTestData("pipeline-secrets-runtime/secret-rotated.yaml")

			By("waiting for the rotated value to appear in the same agent pod's stdout")
			stamped := fmt.Sprintf(`"probe_token":"%s"`, rotatedToken)
			Eventually(func() (string, error) {
				return f.GetPodLogs(agentPod)
			}, secretRotationTimeout, secretRuntimePoll).Should(ContainSubstring(stamped),
				"rotation must reach the running Vector via the mounted secret-assets Secret and --watch-config")

			By("verifying the agent config Secret was not rewritten by the rotation")
			configRVAfter, err := f.Kubectl().GetWithJsonPath("secret", configSecret, ".metadata.resourceVersion")
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(configRVAfter)).To(Equal(configRV),
				"secret rotation must not touch the agent config Secret")

			By("verifying the agent pod was reloaded in place, not rolled")
			Expect(podFieldOrEmpty(f, agentPod, ".metadata.uid")).To(Equal(agentPodUID),
				"the agent pod must survive the rotation (a new UID means a rollout, not a live reload)")
			Expect(podFieldOrEmpty(f, agentPod, restartCountPath)).To(Equal(agentRestart),
				"the agent container must not restart on rotation")
		})
	})
})
