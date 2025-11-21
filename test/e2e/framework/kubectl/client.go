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
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/kaasops/vector-operator/test/utils"
)

// Client provides convenient kubectl operations
type Client struct {
	namespace string
}

// NewClient creates a new kubectl client for the given namespace
func NewClient(namespace string) *Client {
	return &Client{namespace: namespace}
}

// Apply applies YAML content to the cluster with explicit namespace override
// This ensures resources are created in the correct test namespace
func (c *Client) Apply(yamlContent string) error {
	// Validate namespace to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return fmt.Errorf("namespace validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl apply -f - -n %s", c.namespace)

	cmd := exec.Command("kubectl", "apply", "-f", "-", "-n", c.namespace)
	cmd.Stdin = strings.NewReader(yamlContent)
	output, err := utils.Run(cmd)

	// Log kubectl output for debugging (helps catch namespace mismatches)
	if len(output) > 0 {
		fmt.Printf("kubectl apply: %s\n", string(output))
	}

	return err
}

// ApplyWithoutNamespaceOverride applies YAML content without forcing namespace
// Use this when the YAML already contains the correct namespace field
func (c *Client) ApplyWithoutNamespaceOverride(yamlContent string) error {
	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl apply -f -")

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yamlContent)
	output, err := utils.Run(cmd)

	// Log kubectl output for debugging
	if len(output) > 0 {
		fmt.Printf("kubectl apply: %s\n", string(output))
	}

	return err
}

// Get retrieves a resource by name and type
func (c *Client) Get(resourceType, name string) ([]byte, error) {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return nil, fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateResourceType(resourceType); err != nil {
		return nil, fmt.Errorf("resource type validation failed: %w", err)
	}
	if err := ValidateResourceName(name); err != nil {
		return nil, fmt.Errorf("resource name validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl get %s %s -n %s", resourceType, name, c.namespace)

	cmd := exec.Command("kubectl", "get", resourceType, name, "-n", c.namespace)
	return utils.Run(cmd)
}

// GetWithJsonPath retrieves a specific field from a resource
// If name is empty, retrieves from all resources of the given type
func (c *Client) GetWithJsonPath(resourceType, name, jsonPath string) (string, error) {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return "", fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateResourceType(resourceType); err != nil {
		return "", fmt.Errorf("resource type validation failed: %w", err)
	}
	if name != "" {
		if err := ValidateResourceName(name); err != nil {
			return "", fmt.Errorf("resource name validation failed: %w", err)
		}
	}
	if err := ValidateJSONPath(jsonPath); err != nil {
		return "", fmt.Errorf("jsonPath validation failed: %w", err)
	}

	// Build command args based on whether name is specified
	args := []string{"get", resourceType}

	// Only include name if it's not empty (empty name means get all resources)
	if name != "" {
		args = append(args, name)
	}

	args = append(args, "-n", c.namespace, "-o", fmt.Sprintf("jsonpath={%s}", jsonPath))

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl %s", strings.Join(args, " "))

	cmd := exec.Command("kubectl", args...)
	output, err := utils.Run(cmd)
	return string(output), err
}

// GetAll retrieves all resources of a type with optional label selector
func (c *Client) GetAll(resourceType string, labelSelector string) (string, error) {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return "", fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateResourceType(resourceType); err != nil {
		return "", fmt.Errorf("resource type validation failed: %w", err)
	}
	if labelSelector != "" {
		if err := ValidateLabelSelector(labelSelector); err != nil {
			return "", fmt.Errorf("label selector validation failed: %w", err)
		}
	}

	args := []string{"get", resourceType, "-n", c.namespace}
	if labelSelector != "" {
		args = append(args, "-l", labelSelector)
	}
	args = append(args, "-o", "jsonpath={.items[*].metadata.name}")

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl %s", strings.Join(args, " "))

	cmd := exec.Command("kubectl", args...)
	output, err := utils.Run(cmd)
	return string(output), err
}

// Wait waits for a resource condition
func (c *Client) Wait(resourceType, name, condition string, timeout string) error {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateResourceType(resourceType); err != nil {
		return fmt.Errorf("resource type validation failed: %w", err)
	}
	if err := ValidateResourceName(name); err != nil {
		return fmt.Errorf("resource name validation failed: %w", err)
	}
	if err := ValidateTimeout(timeout); err != nil {
		return fmt.Errorf("timeout validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl wait --for=%s --timeout=%s %s/%s -n %s", condition, timeout, resourceType, name, c.namespace)

	cmd := exec.Command("kubectl", "wait",
		fmt.Sprintf("--for=%s", condition),
		fmt.Sprintf("--timeout=%s", timeout),
		fmt.Sprintf("%s/%s", resourceType, name),
		"-n", c.namespace)
	_, err := utils.Run(cmd)
	return err
}

// Delete deletes a resource
func (c *Client) Delete(resourceType, name string) error {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateResourceType(resourceType); err != nil {
		return fmt.Errorf("resource type validation failed: %w", err)
	}
	if err := ValidateResourceName(name); err != nil {
		return fmt.Errorf("resource name validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl delete %s %s -n %s", resourceType, name, c.namespace)

	cmd := exec.Command("kubectl", "delete", resourceType, name, "-n", c.namespace)
	_, err := utils.Run(cmd)
	return err
}

// CreateNamespace creates a namespace
func CreateNamespace(name string) error {
	// Validate namespace to prevent command injection
	if err := ValidateNamespace(name); err != nil {
		return fmt.Errorf("namespace validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl create ns %s", name)

	cmd := exec.Command("kubectl", "create", "ns", name)
	_, err := utils.Run(cmd)
	return err
}

// GetNamespace retrieves namespace information
func GetNamespace(name string) (*corev1.Namespace, error) {
	// Validate namespace to prevent command injection
	if err := ValidateNamespace(name); err != nil {
		return nil, fmt.Errorf("namespace validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl get ns %s -o json", name)

	cmd := exec.Command("kubectl", "get", "ns", name, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var ns corev1.Namespace
	if err := json.Unmarshal(output, &ns); err != nil {
		return nil, fmt.Errorf("failed to parse namespace JSON: %w", err)
	}

	return &ns, nil
}

// DeleteNamespace deletes a namespace with retry and force delete fallback
// Handles CRD resources with finalizers to prevent stuck namespaces
func DeleteNamespace(name string, timeout string) error {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(name); err != nil {
		return fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateTimeout(timeout); err != nil {
		return fmt.Errorf("timeout validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl delete ns %s --timeout=%s", name, timeout)

	// Parse timeout duration for wait logic
	timeoutDuration, err := parseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout format: %w", err)
	}

	// First try: normal delete
	cmd := exec.Command("kubectl", "delete", "ns", name, fmt.Sprintf("--timeout=%s", timeout))
	_, err = utils.Run(cmd)
	if err == nil {
		// Wait for namespace to actually disappear
		return waitForNamespaceDeletion(name, timeoutDuration)
	}

	// If normal delete fails or times out, force cleanup CRD resources first
	fmt.Printf("⚠️  Namespace %s deletion failed, attempting force cleanup\n", name)

	// Clean up operator CRD resources that might have finalizers
	crdTypes := []string{
		"vectorpipeline",
		"vectoraggregator",
		"vector",
		"clustervectorpipeline",
		"clustervectoraggregator",
	}

	for _, crdType := range crdTypes {
		// Get all resources of this type
		cmd := exec.Command("kubectl", "get", crdType, "-n", name, "-o", "name")
		output, err := cmd.Output()
		if err != nil {
			continue // Resource type doesn't exist or no resources, skip
		}

		resources := strings.Fields(string(output))
		for _, resource := range resources {
			// Remove finalizers
			patchCmd := exec.Command("kubectl", "patch", resource, "-n", name,
				"-p", `{"metadata":{"finalizers":[]}}`,
				"--type=merge")
			_ = patchCmd.Run() // Ignore errors

			// Force delete
			deleteCmd := exec.Command("kubectl", "delete", resource, "-n", name,
				"--grace-period=0", "--force")
			_ = deleteCmd.Run() // Ignore errors
		}
	}

	// Remove namespace finalizers
	_ = exec.Command("kubectl", "patch", "ns", name,
		"-p", `{"metadata":{"finalizers":[]}}`,
		"--type=merge").Run()

	// Then force delete namespace with shorter timeout
	log.Printf("KUBECTL_CMD: kubectl delete ns %s --grace-period=0 --force --timeout=10s", name)
	cmd = exec.Command("kubectl", "delete", "ns", name,
		"--grace-period=0", "--force", "--timeout=10s")
	_, _ = utils.Run(cmd)

	// Wait for namespace to actually disappear, even after force delete
	waitErr := waitForNamespaceDeletion(name, 30*time.Second)
	if waitErr != nil {
		fmt.Printf("⚠️  Namespace %s still exists after cleanup, continuing anyway\n", name)
		return nil // Don't fail the test - namespace will be cleaned up eventually
	}

	return nil
}

// waitForNamespaceDeletion waits for a namespace to be fully deleted
func waitForNamespaceDeletion(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		// Try to get the namespace
		cmd := exec.Command("kubectl", "get", "ns", name)
		err := cmd.Run()
		if err != nil {
			// Namespace not found - deletion successful
			log.Printf("KUBECTL_CMD: namespace %s successfully deleted", name)
			return nil
		}

		// Namespace still exists, wait and retry
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("namespace %s still exists after %v", name, timeout)
}

// parseDuration parses timeout strings like "30s", "5m", "1h"
func parseDuration(timeout string) (time.Duration, error) {
	// Extract numeric part and unit
	if len(timeout) < 2 {
		return 0, fmt.Errorf("invalid timeout: %s", timeout)
	}

	unit := timeout[len(timeout)-1:]
	valueStr := timeout[:len(timeout)-1]

	var value int
	_, err := fmt.Sscanf(valueStr, "%d", &value)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout value: %s", timeout)
	}

	switch unit {
	case "s":
		return time.Duration(value) * time.Second, nil
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid timeout unit: %s (must be s, m, or h)", unit)
	}
}

// GetPodLogs retrieves logs from a pod
func (c *Client) GetPodLogs(podName string) (string, error) {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return "", fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateResourceName(podName); err != nil {
		return "", fmt.Errorf("pod name validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl logs %s -n %s", podName, c.namespace)

	cmd := exec.Command("kubectl", "logs", podName, "-n", c.namespace)
	output, err := utils.Run(cmd)
	return string(output), err
}

// GetPodLogsSince retrieves logs from a pod since a specific time
func (c *Client) GetPodLogsSince(podName string, since string) (string, error) {
	cmd := exec.Command("kubectl", "logs", podName, "-n", c.namespace, "--since", since)
	output, err := utils.Run(cmd)
	return string(output), err
}

// GetPodLogsTail retrieves the last N lines of logs from a pod
func (c *Client) GetPodLogsTail(podName string, lines int) (string, error) {
	cmd := exec.Command("kubectl", "logs", podName, "-n", c.namespace, "--tail", fmt.Sprintf("%d", lines))
	output, err := utils.Run(cmd)
	return string(output), err
}

// GetPodLogsSinceTime retrieves logs from a pod since a specific time with line limit
// Uses --since-time for temporal filtering and --tail as a safety limit
func (c *Client) GetPodLogsSinceTime(podName string, since time.Time, tailLines int) (string, error) {
	// Format time as RFC3339 for Kubernetes
	sinceTime := since.Format(time.RFC3339)

	// Use both --since-time and --tail:
	// --since-time filters logs by timestamp
	// --tail provides safety limit if too many logs match
	cmd := exec.Command("kubectl", "logs", podName, "-n", c.namespace,
		"--since-time", sinceTime,
		"--tail", fmt.Sprintf("%d", tailLines))
	output, err := utils.Run(cmd)
	return string(output), err
}

// GetPodsByLabel retrieves pod names matching a label selector
func (c *Client) GetPodsByLabel(labelSelector string) ([]string, error) {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return nil, fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateLabelSelector(labelSelector); err != nil {
		return nil, fmt.Errorf("label selector validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl get pods -n %s -l %s -o jsonpath={.items[*].metadata.name}", c.namespace, labelSelector)

	cmd := exec.Command("kubectl", "get", "pods", "-n", c.namespace, "-l", labelSelector, "-o", "jsonpath={.items[*].metadata.name}")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}

	podNames := strings.Fields(string(output))
	return podNames, nil
}

// WaitForPodReady waits for a pod to become ready
func (c *Client) WaitForPodReady(podName string, timeout string) error {
	// Validate parameters to prevent command injection
	if err := ValidateNamespace(c.namespace); err != nil {
		return fmt.Errorf("namespace validation failed: %w", err)
	}
	if err := ValidateResourceName(podName); err != nil {
		return fmt.Errorf("pod name validation failed: %w", err)
	}
	if err := ValidateTimeout(timeout); err != nil {
		return fmt.Errorf("timeout validation failed: %w", err)
	}

	// Log command for audit and reproducibility
	log.Printf("KUBECTL_CMD: kubectl wait --for=condition=Ready --timeout=%s pod/%s -n %s", timeout, podName, c.namespace)

	cmd := exec.Command("kubectl", "wait",
		"--for=condition=Ready",
		fmt.Sprintf("--timeout=%s", timeout),
		fmt.Sprintf("pod/%s", podName),
		"-n", c.namespace)
	_, err := utils.Run(cmd)
	return err
}
