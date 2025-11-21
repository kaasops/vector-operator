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

package kubectl

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework/config"
	"github.com/kaasops/vector-operator/test/utils"
)

// WaitForDeploymentReady waits for a deployment to be created and ready
func (c *Client) WaitForDeploymentReady(name string) {
	// First wait for deployment to exist (with reduced timeout)
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "deployment", name, "-n", c.namespace)
		_, err := utils.Run(cmd)
		return err
	}, config.DeploymentCreateTimeout, config.DefaultPollInterval).Should(Succeed(),
		"Deployment %s should be created in namespace %s", name, c.namespace)

	// Then wait for it to be available
	err := c.Wait("deployment", name, "condition=available", config.DeploymentReadyTimeout.String())
	Expect(err).NotTo(HaveOccurred(),
		"Deployment %s should become ready in namespace %s", name, c.namespace)
}

// WaitForPipelineValid waits for a VectorPipeline to become valid
func (c *Client) WaitForPipelineValid(name string) {
	Eventually(func() error {
		result, err := c.GetWithJsonPath("vectorpipeline", name, ".status.configCheckResult")
		if err != nil {
			return err
		}
		if result != "true" {
			return fmt.Errorf("pipeline not valid yet: %s", result)
		}
		return nil
	}, config.PipelineValidTimeout, config.SlowPollInterval).Should(Succeed(),
		"Pipeline %s should become valid in namespace %s", name, c.namespace)
}

// WaitForPipelineInvalid waits for a VectorPipeline to become invalid (for negative tests)
func (c *Client) WaitForPipelineInvalid(name string) {
	Eventually(func() error {
		result, err := c.GetWithJsonPath("vectorpipeline", name, ".status.configCheckResult")
		if err != nil {
			return err
		}
		if result != "false" {
			return fmt.Errorf("expected pipeline to be invalid, got: %s", result)
		}
		return nil
	}, config.PipelineValidTimeout, config.SlowPollInterval).Should(Succeed(),
		"Pipeline %s should become invalid in namespace %s", name, c.namespace)
}

// WaitForServiceExists waits for a service to be created
func (c *Client) WaitForServiceExists(name string) {
	Eventually(func() error {
		_, err := c.Get("service", name)
		return err
	}, config.ServiceCreateTimeout, config.SlowPollInterval).Should(Succeed(),
		"Service %s should be created in namespace %s", name, c.namespace)
}

// WaitForServiceCount waits for a specific number of services matching filter
func (c *Client) WaitForServiceCount(labelSelector string, expectedCount int, timeout time.Duration) {
	Eventually(func() (int, error) {
		result, err := c.GetAll("service", labelSelector)
		if err != nil {
			return 0, err
		}
		if result == "" {
			return 0, nil
		}

		services := 0
		for _, svc := range splitFields(result) {
			if svc != "" {
				services++
			}
		}
		return services, nil
	}, timeout, config.SlowPollInterval).Should(Equal(expectedCount),
		"Expected %d services with label %s in namespace %s", expectedCount, labelSelector, c.namespace)
}

// splitFields splits space-separated fields and filters empty strings
func splitFields(s string) []string {
	return strings.Fields(s)
}
