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

	"github.com/kaasops/vector-operator/test/utils"
)

const namespace = "vector-operator-system"

var _ = Describe("controller", Ordered, func() {
	// NOTE: Dependencies (Prometheus Operator, cert-manager) are installed once
	// in BeforeSuite via framework.InstallSharedDependencies() and shared across all tests.
	// No need for BeforeAll/AfterAll here.

	BeforeAll(func() {
		By("creating manager namespace (if not exists)")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, _ = utils.Run(cmd) // Ignore error if already exists
	})

	Context("Operator", func() {
		It("should run successfully", func() {
			var controllerPodName string

			// NOTE: Operator is deployed once in BeforeSuite (e2e_suite_test.go) via Helm
			// This test verifies that the already-deployed operator is running correctly

			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func() error {
				// Get pod name (Helm deployment uses different labels)

				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "app.kubernetes.io/name=vector-operator",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				podNames := utils.GetNonEmptyLines(string(podOutput))
				if len(podNames) != 1 {
					return fmt.Errorf("expect 1 controller pods running, but got %d", len(podNames))
				}
				controllerPodName = podNames[0]
				ExpectWithOffset(2, controllerPodName).Should(ContainSubstring("vector-operator"))

				// Validate pod status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				status, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				if string(status) != "Running" {
					return fmt.Errorf("controller pod in %s status", status)
				}
				return nil
			}
			EventuallyWithOffset(1, verifyControllerUp, time.Minute, time.Second).Should(Succeed())

		})
	})
})
