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

package framework

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/kaasops/vector-operator/test/e2e/framework/config"
	"github.com/kaasops/vector-operator/test/e2e/framework/errors"
	"github.com/kaasops/vector-operator/test/e2e/framework/kubectl"
	"github.com/kaasops/vector-operator/test/e2e/framework/recorder"
)

const (
	// MaxConfigSize is the maximum allowed size for base64-encoded config data (10MB)
	// This prevents DoS attacks via extremely large config payloads
	MaxConfigSize = 10 * 1024 * 1024 // 10MB
)

// Framework provides a high-level API for e2e tests
type Framework struct {
	namespace    string
	kubectl      *kubectl.Client
	isShared     bool
	metrics      *TestMetrics
	recorder     *recorder.TestRecorder
	dryRun       bool
	recordSteps  bool
	TestDataPath string // Path to test data directory (configurable via E2E_TESTDATA_PATH)
}

// TestMetrics tracks timing information for test operations
type TestMetrics struct {
	SetupTime              time.Duration
	DeploymentWaitTime     time.Duration
	PipelineValidationTime time.Duration
	CleanupTime            time.Duration
}

// Global shared framework instance
var sharedFramework *Framework

// frameworkRegistry stores framework instances for artifact collection
// Key: namespace, Value: *Framework
// DEPRECATED: This is being phased out in favor of Ginkgo report entries.
// New code should use AddReportEntry() in Setup() and retrieve from ReportAfterEach.
var frameworkRegistry sync.Map

// FrameworkContextKey is the key type for storing Framework in context
// Using a custom type prevents collisions with other context keys
type FrameworkContextKey struct{}

// frameworkReportEntryName is the name used when storing framework in Ginkgo report entries
const frameworkReportEntryName = "framework-instance"

// NewFramework creates a new isolated test framework with its own namespace
func NewFramework(namespace string) *Framework {
	// Get test data path from environment or use default
	testDataPath := os.Getenv("E2E_TESTDATA_PATH")
	if testDataPath == "" {
		testDataPath = filepath.Join("test", "e2e", "testdata")
	}

	f := &Framework{
		namespace:    namespace,
		kubectl:      kubectl.NewClient(namespace),
		isShared:     false,
		metrics:      &TestMetrics{},
		TestDataPath: testDataPath,
	}

	// Check for dry-run or recording mode
	if os.Getenv("E2E_DRY_RUN") == "true" {
		f.dryRun = true
		f.recorder = recorder.NewTestRecorder(namespace)
		f.recordSteps = true
	} else if os.Getenv("E2E_RECORD_STEPS") == "true" {
		f.recordSteps = true
		f.recorder = recorder.NewTestRecorder(namespace)
	}

	return f
}

// NewUniqueFramework creates a new framework with a unique timestamped namespace
// This prevents namespace collisions when tests run in parallel or when cleanup is slow
func NewUniqueFramework(baseName string) *Framework {
	// Use nanosecond timestamp + counter for uniqueness
	timestamp := time.Now().UnixNano()
	uniqueNS := fmt.Sprintf("%s-%d", baseName, timestamp)

	// Get test data path from environment or use default
	testDataPath := os.Getenv("E2E_TESTDATA_PATH")
	if testDataPath == "" {
		testDataPath = filepath.Join("test", "e2e", "testdata")
	}

	f := &Framework{
		namespace:    uniqueNS,
		kubectl:      kubectl.NewClient(uniqueNS),
		isShared:     false,
		metrics:      &TestMetrics{},
		TestDataPath: testDataPath,
	}

	// Check for dry-run or recording mode
	if os.Getenv("E2E_DRY_RUN") == "true" {
		f.dryRun = true
		f.recorder = recorder.NewTestRecorder(uniqueNS)
		f.recordSteps = true
	} else if os.Getenv("E2E_RECORD_STEPS") == "true" {
		f.recordSteps = true
		f.recorder = recorder.NewTestRecorder(uniqueNS)
	}

	return f
}

// Shared returns a shared framework instance that reuses the same namespace
// This is useful for parallel tests that don't interfere with each other
func Shared(namespace string) *Framework {
	if sharedFramework == nil {
		// Get test data path from environment or use default
		testDataPath := os.Getenv("E2E_TESTDATA_PATH")
		if testDataPath == "" {
			testDataPath = filepath.Join("test", "e2e", "testdata")
		}

		sharedFramework = &Framework{
			namespace:    namespace,
			kubectl:      kubectl.NewClient(namespace),
			isShared:     true,
			metrics:      &TestMetrics{},
			TestDataPath: testDataPath,
		}

		// Check for dry-run or recording mode
		if os.Getenv("E2E_DRY_RUN") == "true" {
			sharedFramework.dryRun = true
			sharedFramework.recorder = recorder.NewTestRecorder(namespace)
			sharedFramework.recordSteps = true
		} else if os.Getenv("E2E_RECORD_STEPS") == "true" {
			sharedFramework.recordSteps = true
			sharedFramework.recorder = recorder.NewTestRecorder(namespace)
		}
	}
	return sharedFramework
}

// Setup performs the test environment setup
func (f *Framework) Setup() {
	// Store framework in Ginkgo report entries for artifact collection
	// This is the preferred method as it directly associates framework with the current test
	// and works correctly with parallel test execution
	AddReportEntry(frameworkReportEntryName, f)

	// DEPRECATED: Also store in global registry for backward compatibility
	// This will be removed in a future version once all code migrates to using report entries
	frameworkRegistry.Store(f.namespace, f)

	start := time.Now()
	defer func() {
		f.metrics.SetupTime = time.Since(start)
	}()

	By(fmt.Sprintf("creating test namespace: %s", f.namespace))
	err := kubectl.CreateNamespace(f.namespace)
	if err != nil {
		// Check if it's an ignorable error (AlreadyExists, NotFound)
		if errors.IsIgnorable(err) {
			GinkgoWriter.Printf("Warning: namespace creation failed (might already exist): %v\n", err)
		} else {
			// Only fail for non-ignorable errors
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// Wait for namespace to be ready before proceeding
	By(fmt.Sprintf("waiting for namespace to be ready: %s", f.namespace))
	Eventually(func() bool {
		ns, err := kubectl.GetNamespace(f.namespace)
		if err != nil {
			GinkgoWriter.Printf("Failed to get namespace status: %v\n", err)
			return false
		}
		// Check namespace is Active (not Terminating)
		return ns.Status.Phase == "Active"
	}, config.DeploymentReadyTimeout, config.DefaultPollInterval).Should(BeTrue(),
		fmt.Sprintf("namespace %s should be Active", f.namespace))
}

// Teardown performs the test environment cleanup
func (f *Framework) Teardown() {
	// Export test plan if recording is enabled
	if f.recorder != nil && f.recordSteps {
		f.ExportTestPlan()
	}

	// Don't cleanup shared namespaces immediately
	if f.isShared {
		return
	}

	start := time.Now()
	defer func() {
		f.metrics.CleanupTime = time.Since(start)
	}()

	By(fmt.Sprintf("cleaning up test namespace: %s", f.namespace))
	err := kubectl.DeleteNamespace(f.namespace, fmt.Sprintf("%ds", int(config.NamespaceDeleteTimeout.Seconds())))
	if err != nil {
		GinkgoWriter.Printf("Warning: namespace cleanup failed: %v\n", err)
	}

	// NOTE: Do NOT delete from frameworkRegistry here!
	// ReportAfterEach runs AFTER AfterAll/Teardown, and needs the framework
	// for artifact collection. The registry will be cleaned up when the process exits.
	// frameworkRegistry.Delete(f.namespace)
}

// Namespace returns the test namespace
func (f *Framework) Namespace() string {
	return f.namespace
}

// ApplyTestData loads and applies a test manifest from testdata directory
// It automatically replaces any hardcoded namespace with the framework's namespace
func (f *Framework) ApplyTestData(path string) {
	By(fmt.Sprintf("applying test data: %s", path))

	content, err := os.ReadFile(filepath.Join(f.TestDataPath, path))
	Expect(err).NotTo(HaveOccurred(), "Failed to load test data from %s", path)

	// Replace namespace in YAML if present
	yamlContent := replaceNamespace(string(content), f.namespace)

	err = f.kubectl.Apply(yamlContent)
	Expect(err).NotTo(HaveOccurred(), "Failed to apply test data %s in namespace %s", path, f.namespace)
}

// ApplyTestDataWithoutNamespaceReplacement loads and applies a test manifest WITHOUT namespace replacement
// Use this when you need to apply resources to specific namespaces
func (f *Framework) ApplyTestDataWithoutNamespaceReplacement(path string) {
	By(fmt.Sprintf("applying test data without namespace replacement: %s", path))

	content, err := os.ReadFile(filepath.Join(f.TestDataPath, path))
	Expect(err).NotTo(HaveOccurred(), "Failed to load test data from %s", path)

	// Apply without forcing namespace (YAML contains the correct namespace)
	err = f.kubectl.ApplyWithoutNamespaceOverride(string(content))
	Expect(err).NotTo(HaveOccurred(), "Failed to apply test data %s", path)
}

// DeleteTestData loads and deletes a test manifest from testdata directory
// It automatically replaces any hardcoded namespace with the framework's namespace
func (f *Framework) DeleteTestData(path string) {
	By(fmt.Sprintf("deleting test data: %s", path))

	content, err := os.ReadFile(filepath.Join(f.TestDataPath, path))
	Expect(err).NotTo(HaveOccurred(), "Failed to load test data from %s", path)

	// Replace namespace in YAML if present
	yamlContent := replaceNamespace(string(content), f.namespace)

	err = f.kubectl.DeleteFromYAML(yamlContent)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete test data %s in namespace %s", path, f.namespace)
}

// replaceNamespace replaces namespace placeholders and fields in YAML content
func replaceNamespace(yaml, namespace string) string {
	// Replace NAMESPACE placeholders throughout the content
	// This handles cases like spec.resourceNamespace: NAMESPACE
	yaml = replacePlaceholder(yaml, "NAMESPACE", namespace)

	// Also replace explicit namespace fields in metadata sections
	// This handles cases like:
	//   namespace: some-other-namespace
	lines := []string{}
	for _, line := range splitLines(yaml) {
		// Check for "  namespace:" (2 spaces + "namespace:" = 12 chars)
		if len(line) > 12 && line[:12] == "  namespace:" {
			lines = append(lines, fmt.Sprintf("  namespace: %s", namespace))
		} else {
			lines = append(lines, line)
		}
	}

	return joinLines(lines)
}

// splitLines splits string by newlines
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

// joinLines joins lines with newlines
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// ApplyYAML applies raw YAML content
func (f *Framework) ApplyYAML(yamlContent string) {
	err := f.kubectl.Apply(yamlContent)
	Expect(err).NotTo(HaveOccurred(), "Failed to apply YAML in namespace %s", f.namespace)
}

// WaitForDeploymentReady waits for a deployment to be ready
func (f *Framework) WaitForDeploymentReady(name string) {
	By(fmt.Sprintf("waiting for deployment %s to be ready", name))
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		f.metrics.DeploymentWaitTime += duration
		GinkgoWriter.Printf("⏱️  Deployment %s ready in %v\n", name, duration)
	}()

	f.kubectl.WaitForDeploymentReady(name)
}

// WaitForPipelineValid waits for a pipeline to become valid
func (f *Framework) WaitForPipelineValid(name string) {
	By(fmt.Sprintf("waiting for pipeline %s to become valid", name))
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		f.metrics.PipelineValidationTime += duration
		GinkgoWriter.Printf("⏱️  Pipeline %s validated in %v\n", name, duration)
	}()

	f.kubectl.WaitForPipelineValid(name)
}

// WaitForPipelineInvalid waits for a pipeline to become invalid (for negative tests)
func (f *Framework) WaitForPipelineInvalid(name string) {
	By(fmt.Sprintf("waiting for pipeline %s to become invalid", name))
	f.kubectl.WaitForPipelineInvalid(name)
}

// GetPipelineStatus retrieves a specific status field from a pipeline
func (f *Framework) GetPipelineStatus(name string, field string) string {
	result, err := f.kubectl.GetWithJsonPath("vectorpipeline", name, fmt.Sprintf(".status.%s", field))
	Expect(err).NotTo(HaveOccurred(),
		"Failed to get pipeline %s status field %s in namespace %s", name, field, f.namespace)
	return result
}

// GetServicePort retrieves the port of a service
func (f *Framework) GetServicePort(name string) string {
	result, err := f.kubectl.GetWithJsonPath("service", name, ".spec.ports[0].port")
	Expect(err).NotTo(HaveOccurred(),
		"Failed to get service %s port in namespace %s", name, f.namespace)
	return result
}

// TryGetServicePort retrieves the port of a service without failing if not found
func (f *Framework) TryGetServicePort(name string) (string, error) {
	return f.kubectl.GetWithJsonPath("service", name, ".spec.ports[0].port")
}

// CreateMultiplePipelinesFromTemplate creates N pipelines from a template by replacing a placeholder
func (f *Framework) CreateMultiplePipelinesFromTemplate(templatePath, placeholder string, count int) time.Duration {
	start := time.Now()

	content, err := os.ReadFile(filepath.Join(f.TestDataPath, templatePath))
	Expect(err).NotTo(HaveOccurred(), "Failed to load template from %s", templatePath)

	template := string(content)

	for i := 1; i <= count; i++ {
		pipelineName := fmt.Sprintf("pipeline-%03d", i)
		yaml := replaceNamespace(template, f.namespace)
		yaml = replacePlaceholder(yaml, placeholder, pipelineName)

		err = f.kubectl.Apply(yaml)
		Expect(err).NotTo(HaveOccurred(),
			"Failed to apply pipeline %s from template %s in namespace %s", pipelineName, templatePath, f.namespace)
	}

	return time.Since(start)
}

// replacePlaceholder replaces a placeholder in YAML content
func replacePlaceholder(yaml, placeholder, value string) string {
	return strings.ReplaceAll(yaml, placeholder, value)
}

// CountValidPipelines counts how many pipelines are valid in the namespace
func (f *Framework) CountValidPipelines() (int, error) {
	result, err := f.kubectl.GetWithJsonPath("vectorpipeline", "", ".items[*].status.configCheckResult")
	if err != nil {
		return 0, err
	}

	if result == "" {
		return 0, nil
	}

	validCount := 0
	for _, status := range splitFields(result) {
		if status == "true" {
			validCount++
		}
	}

	return validCount, nil
}

// CountPipelines returns the total number of pipelines in the namespace
func (f *Framework) CountPipelines() (int, error) {
	result, err := f.kubectl.GetAll("vectorpipeline", "")
	if err != nil {
		return 0, err
	}

	if result == "" {
		return 0, nil
	}

	return len(splitFields(result)), nil
}

// CountServicesContaining counts services whose name contains the given substring
func (f *Framework) CountServicesContaining(substring string) (int, error) {
	result, err := f.kubectl.GetAll("service", "")
	if err != nil {
		return 0, err
	}

	if result == "" {
		return 0, nil
	}

	count := 0
	for _, svc := range splitFields(result) {
		if svc != "" && containsSubstring(svc, substring) {
			count++
		}
	}

	return count, nil
}

// containsSubstring checks if a string contains a substring
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ExpectServiceExists verifies that a service exists
func (f *Framework) ExpectServiceExists(name string) {
	By(fmt.Sprintf("verifying service %s exists", name))
	_, err := f.kubectl.Get("service", name)
	Expect(err).NotTo(HaveOccurred(),
		"Expected service %s to exist in namespace %s", name, f.namespace)
}

// CountServicesWithLabel counts services matching a label selector
func (f *Framework) CountServicesWithLabel(labelSelector string) int {
	result, err := f.kubectl.GetAll("service", labelSelector)
	Expect(err).NotTo(HaveOccurred(),
		"Failed to get services with label %s in namespace %s", labelSelector, f.namespace)

	if result == "" {
		return 0
	}

	count := 0
	for _, svc := range splitFields(result) {
		if svc != "" {
			count++
		}
	}
	return count
}

// WaitForServiceCount waits for a specific number of services
func (f *Framework) WaitForServiceCount(labelSelector string, expectedCount int, timeout time.Duration) {
	By(fmt.Sprintf("waiting for %d services with label %s", expectedCount, labelSelector))
	f.kubectl.WaitForServiceCount(labelSelector, expectedCount, timeout)
}

// PrintMetrics prints timing metrics for the test
func (f *Framework) PrintMetrics() {
	GinkgoWriter.Println("\n📊 Test Metrics:")
	GinkgoWriter.Printf("  Setup: %v\n", f.metrics.SetupTime)
	GinkgoWriter.Printf("  Deployment Wait: %v\n", f.metrics.DeploymentWaitTime)
	GinkgoWriter.Printf("  Pipeline Validation: %v\n", f.metrics.PipelineValidationTime)
	GinkgoWriter.Printf("  Cleanup: %v\n", f.metrics.CleanupTime)
	GinkgoWriter.Printf("  Total: %v\n", f.metrics.SetupTime+f.metrics.DeploymentWaitTime+f.metrics.PipelineValidationTime+f.metrics.CleanupTime)
}

// splitFields splits space-separated fields
func splitFields(s string) []string {
	return strings.Fields(s)
}

// GetPodLogs retrieves logs from a pod
func (f *Framework) GetPodLogs(podName string) (string, error) {
	return f.kubectl.GetPodLogs(podName)
}

// GetPodLogsTail retrieves the last N lines of logs from a pod
func (f *Framework) GetPodLogsTail(podName string, lines int) (string, error) {
	return f.kubectl.GetPodLogsTail(podName, lines)
}

// GetPodsByLabel retrieves pod names matching a label selector
func (f *Framework) GetPodsByLabel(labelSelector string) ([]string, error) {
	return f.kubectl.GetPodsByLabel(labelSelector)
}

// WaitForPodReady waits for a pod to become ready
func (f *Framework) WaitForPodReady(podName string) {
	By(fmt.Sprintf("waiting for pod %s to be ready", podName))
	err := f.kubectl.WaitForPodReady(podName, "2m")
	Expect(err).NotTo(HaveOccurred(), "Pod %s did not become ready in namespace %s", podName, f.namespace)
}

// GetAggregatorPods retrieves aggregator pod names for a given aggregator
func (f *Framework) GetAggregatorPods(aggregatorName string) ([]string, error) {
	// Aggregator pods use instance label to identify which aggregator they belong to
	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s,app.kubernetes.io/component=Aggregator", aggregatorName)
	return f.kubectl.GetPodsByLabel(labelSelector)
}

// GetAgentPods retrieves agent pod names
func (f *Framework) GetAgentPods(vectorName string) ([]string, error) {
	// Agent pods use instance label and component=Agent
	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s,app.kubernetes.io/component=Agent", vectorName)
	return f.kubectl.GetPodsByLabel(labelSelector)
}

// GetPipelineAnnotation retrieves a specific annotation from a pipeline
func (f *Framework) GetPipelineAnnotation(name string, annotationKey string) string {
	jsonPath := fmt.Sprintf(".metadata.annotations['%s']", annotationKey)
	result, err := f.kubectl.GetWithJsonPath("vectorpipeline", name, jsonPath)
	if err != nil {
		// Annotation might not exist, which is expected in some cases
		return ""
	}
	return result
}

// VerifyAgentHasPipeline verifies that the agent Secret contains the specified pipeline
func (f *Framework) VerifyAgentHasPipeline(vectorName, pipelineName string) error {
	return f.VerifyAgentHasPipelineInNamespace(vectorName, pipelineName, f.namespace)
}

// VerifyAgentHasPipelineInNamespace verifies that an agent Secret contains the specified pipeline from a specific namespace
func (f *Framework) VerifyAgentHasPipelineInNamespace(vectorName, pipelineName, namespace string) error {
	// Get the agent's vector config from the Secret
	// The config is stored in a Secret with name pattern: {vectorName}-agent
	secretName := fmt.Sprintf("%s-agent", vectorName)

	// Get base64-encoded config from Secret
	encodedConfig, err := f.kubectl.GetWithJsonPath("secret", secretName, ".data['agent\\.json']")
	if err != nil {
		return fmt.Errorf("failed to get agent secret %s: %w", secretName, err)
	}

	if encodedConfig == "" {
		return fmt.Errorf("agent secret %s has no agent.json data", secretName)
	}

	// Check size before decoding to prevent DoS via large payloads
	maxEncodedSize := MaxConfigSize * 4 / 3
	if len(encodedConfig) > maxEncodedSize {
		return fmt.Errorf("config too large: %d bytes (max %d bytes)", len(encodedConfig), maxEncodedSize)
	}

	// Decode base64
	configBytes, err := base64.StdEncoding.DecodeString(encodedConfig)
	if err != nil {
		return fmt.Errorf("failed to decode base64 config from secret %s: %w", secretName, err)
	}
	config := string(configBytes)

	if config == "" {
		return fmt.Errorf("agent config is empty after decoding")
	}

	// Check if the pipeline name appears in the config
	// In normal mode, pipeline components are prefixed with namespace-pipelinename-
	expectedPrefix := fmt.Sprintf("%s-%s-", namespace, pipelineName)
	if !strings.Contains(config, expectedPrefix) {
		return fmt.Errorf("pipeline %s not found in agent config (expected prefix: %s)", pipelineName, expectedPrefix)
	}

	return nil
}

// VerifyAgentHasClusterPipeline verifies that an agent Secret contains the specified ClusterVectorPipeline
func (f *Framework) VerifyAgentHasClusterPipeline(vectorName, pipelineName string) error {
	// Get the agent's vector config from the Secret
	secretName := fmt.Sprintf("%s-agent", vectorName)

	// Get base64-encoded config from Secret
	encodedConfig, err := f.kubectl.GetWithJsonPath("secret", secretName, ".data['agent\\.json']")
	if err != nil {
		return fmt.Errorf("failed to get agent secret %s: %w", secretName, err)
	}

	if encodedConfig == "" {
		return fmt.Errorf("agent secret %s has no agent.json data", secretName)
	}

	// Check size before decoding to prevent DoS via large payloads
	maxEncodedSize := MaxConfigSize * 4 / 3
	if len(encodedConfig) > maxEncodedSize {
		return fmt.Errorf("config too large: %d bytes (max %d bytes)", len(encodedConfig), maxEncodedSize)
	}

	// Decode base64
	configBytes, err := base64.StdEncoding.DecodeString(encodedConfig)
	if err != nil {
		return fmt.Errorf("failed to decode base64 config from secret %s: %w", secretName, err)
	}
	config := string(configBytes)

	if config == "" {
		return fmt.Errorf("agent config is empty after decoding")
	}

	// Check if the cluster pipeline name appears in the config
	// ClusterVectorPipeline components are prefixed with only pipelinename- (no namespace prefix)
	expectedPrefix := fmt.Sprintf("%s-", pipelineName)
	if !strings.Contains(config, expectedPrefix) {
		return fmt.Errorf("cluster pipeline %s not found in agent config (expected prefix: %s)", pipelineName, expectedPrefix)
	}

	return nil
}

// VerifyAggregatorHasPipeline verifies that an aggregator Secret contains the specified pipeline
func (f *Framework) VerifyAggregatorHasPipeline(aggregatorName, pipelineName string) error {
	// Get the aggregator's vector config from the Secret
	// The config is stored in a Secret with name pattern: {aggregatorName}-aggregator
	secretName := fmt.Sprintf("%s-aggregator", aggregatorName)

	// Get base64-encoded config from Secret
	encodedConfig, err := f.kubectl.GetWithJsonPath("secret", secretName, ".data['config\\.json']")
	if err != nil {
		return fmt.Errorf("failed to get aggregator secret %s: %w", secretName, err)
	}

	if encodedConfig == "" {
		return fmt.Errorf("aggregator secret %s has no config.json data", secretName)
	}

	// Check size before decoding to prevent DoS via large payloads
	maxEncodedSize := MaxConfigSize * 4 / 3
	if len(encodedConfig) > maxEncodedSize {
		return fmt.Errorf("config too large: %d bytes (max %d bytes)", len(encodedConfig), maxEncodedSize)
	}

	// Decode base64
	configBytes, err := base64.StdEncoding.DecodeString(encodedConfig)
	if err != nil {
		return fmt.Errorf("failed to decode base64 config from secret %s: %w", secretName, err)
	}
	config := string(configBytes)

	if config == "" {
		return fmt.Errorf("aggregator %s config is empty after decoding", aggregatorName)
	}

	// Check if the pipeline name appears in the config
	expectedPrefix := fmt.Sprintf("%s-%s-", f.namespace, pipelineName)
	if !strings.Contains(config, expectedPrefix) {
		return fmt.Errorf("pipeline %s not found in aggregator %s config (expected prefix: %s)",
			pipelineName, aggregatorName, expectedPrefix)
	}

	return nil
}

// ApplyTestDataWithVars loads and applies a test manifest with variable substitution
func (f *Framework) ApplyTestDataWithVars(path string, vars map[string]string) {
	By(fmt.Sprintf("applying test data with vars: %s", path))

	content, err := os.ReadFile(filepath.Join(f.TestDataPath, path))
	Expect(err).NotTo(HaveOccurred(), "Failed to load test data from %s", path)

	// Replace namespace in YAML
	yamlContent := replaceNamespace(string(content), f.namespace)

	// Replace variables
	for placeholder, value := range vars {
		yamlContent = strings.ReplaceAll(yamlContent, placeholder, value)
	}

	err = f.kubectl.Apply(yamlContent)
	Expect(err).NotTo(HaveOccurred(), "Failed to apply test data %s in namespace %s", path, f.namespace)
}

// DeleteResource deletes a Kubernetes resource
func (f *Framework) DeleteResource(kind, name string) {
	By(fmt.Sprintf("deleting %s %s", kind, name))
	err := f.kubectl.Delete(kind, name)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete %s %s in namespace %s", kind, name, f.namespace)
}

// WaitForPodReadyInNamespace waits for a pod to become ready in a specific namespace
func (f *Framework) WaitForPodReadyInNamespace(podName, namespace string) {
	By(fmt.Sprintf("waiting for pod %s to be ready in namespace %s", podName, namespace))
	client := kubectl.NewClient(namespace)
	err := client.WaitForPodReady(podName, "2m")
	Expect(err).NotTo(HaveOccurred(), "Pod %s did not become ready in namespace %s", podName, namespace)
}

// WaitForPipelineValidInNamespace waits for a pipeline to become valid in a specific namespace
func (f *Framework) WaitForPipelineValidInNamespace(name, namespace string) {
	By(fmt.Sprintf("waiting for pipeline %s to become valid in namespace %s", name, namespace))
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		GinkgoWriter.Printf("⏱️  Pipeline %s validated in %v (namespace: %s)\n", name, duration, namespace)
	}()

	client := kubectl.NewClient(namespace)
	client.WaitForPipelineValid(name)
}

// GetPipelineAnnotationInNamespace retrieves a specific annotation from a pipeline in a specific namespace
func (f *Framework) GetPipelineAnnotationInNamespace(name, namespace, annotationKey string) string {
	jsonPath := fmt.Sprintf(".metadata.annotations['%s']", annotationKey)
	client := kubectl.NewClient(namespace)
	result, err := client.GetWithJsonPath("vectorpipeline", name, jsonPath)
	if err != nil {
		// Annotation might not exist, which is expected in some cases
		return ""
	}
	return result
}

// WaitForClusterPipelineValid waits for a ClusterVectorPipeline to become valid
func (f *Framework) WaitForClusterPipelineValid(name string) {
	By(fmt.Sprintf("waiting for ClusterVectorPipeline %s to become valid", name))
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		GinkgoWriter.Printf("⏱️  ClusterVectorPipeline %s validated in %v\n", name, duration)
	}()

	// ClusterVectorPipeline is cluster-scoped, so we use a client without namespace
	client := kubectl.NewClient("")
	Eventually(func() string {
		result, _ := client.GetWithJsonPath("clustervectorpipeline", name, ".status.configCheckResult")
		return result
	}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Equal("true"),
		"ClusterVectorPipeline %s did not become valid", name)
}

// GetClusterPipelineAnnotation retrieves a specific annotation from a ClusterVectorPipeline
func (f *Framework) GetClusterPipelineAnnotation(name, annotationKey string) string {
	jsonPath := fmt.Sprintf(".metadata.annotations['%s']", annotationKey)
	client := kubectl.NewClient("")
	result, err := client.GetWithJsonPath("clustervectorpipeline", name, jsonPath)
	if err != nil {
		// Annotation might not exist, which is expected in some cases
		return ""
	}
	return result
}

// GetClusterPipelineStatus retrieves a specific status field from a ClusterVectorPipeline
func (f *Framework) GetClusterPipelineStatus(name, field string) string {
	client := kubectl.NewClient("")
	result, err := client.GetWithJsonPath("clustervectorpipeline", name, fmt.Sprintf(".status.%s", field))
	Expect(err).NotTo(HaveOccurred(),
		"Failed to get ClusterVectorPipeline %s status field %s", name, field)
	return result
}

// Kubectl returns the kubectl client
func (f *Framework) Kubectl() *kubectl.Client {
	return f.kubectl
}

// GetRegisteredFramework retrieves a framework by namespace
// Used by artifact collector to access kubectl client and namespace
func GetRegisteredFramework(namespace string) (*Framework, bool) {
	value, ok := frameworkRegistry.Load(namespace)
	if !ok {
		return nil, false
	}
	return value.(*Framework), true
}

// GetFrameworkRegistry returns the framework registry for iteration
// Used by ReportAfterEach to find frameworks when namespace is not known
func GetFrameworkRegistry() *sync.Map {
	return &frameworkRegistry
}

// GetSecret retrieves a Secret by name in the framework's namespace
func (f *Framework) GetSecret(name string) (map[string][]byte, error) {
	cmd := fmt.Sprintf("kubectl get secret %s -n %s -o json", name, f.namespace)
	output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w, output: %s", name, err, string(output))
	}

	var secret struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(output, &secret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret: %w", err)
	}

	// Decode base64 data
	decodedData := make(map[string][]byte)
	maxEncodedSize := MaxConfigSize * 4 / 3
	for k, v := range secret.Data {
		// Check size before decoding to prevent DoS via large payloads
		if len(v) > maxEncodedSize {
			return nil, fmt.Errorf("secret data for key %s too large: %d bytes (max %d bytes)", k, len(v), maxEncodedSize)
		}

		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("failed to decode secret data for key %s: %w", k, err)
		}
		decodedData[k] = decoded
	}

	return decodedData, nil
}

// GetDeployment retrieves a Deployment by name in the framework's namespace
func (f *Framework) GetDeployment(name string) (*DeploymentInfo, error) {
	cmd := fmt.Sprintf("kubectl get deployment %s -n %s -o json", name, f.namespace)
	output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s: %w, output: %s", name, err, string(output))
	}

	var deployment struct {
		Spec struct {
			Template struct {
				Spec struct {
					InitContainers []struct {
						Name string `json:"name"`
					} `json:"initContainers"`
					Containers []struct {
						Name string `json:"name"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(output, &deployment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployment: %w", err)
	}

	info := &DeploymentInfo{
		InitContainers: make([]string, 0),
		Containers:     make([]string, 0),
	}

	for _, c := range deployment.Spec.Template.Spec.InitContainers {
		info.InitContainers = append(info.InitContainers, c.Name)
	}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		info.Containers = append(info.Containers, c.Name)
	}

	return info, nil
}

// DeploymentInfo contains simplified deployment information
type DeploymentInfo struct {
	InitContainers []string
	Containers     []string
}

// RecordStep records a test step for reproducibility
func (f *Framework) RecordStep(step recorder.TestStep) {
	if f.recorder != nil {
		f.recorder.RecordStep(step)
	}
}

// SetTestName sets the current test name in the recorder
func (f *Framework) SetTestName(name string) {
	if f.recorder != nil {
		f.recorder.SetTestName(name)
	}
}

// ExportTestPlan exports the recorded test plan to files
func (f *Framework) ExportTestPlan() {
	if f.recorder == nil {
		return
	}

	// Get current test spec info
	spec := CurrentSpecReport()
	testName := buildTestName(spec)

	if testName == "" {
		testName = "unknown-test"
	}

	f.recorder.SetTestName(testName)

	// In dry-run mode, print to stdout
	if f.dryRun {
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Printf("Test Plan: %s\n", testName)
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println(f.recorder.ExportAsShellScript())
		return
	}

	// Otherwise, save to artifact directory if it exists
	artifactDir := os.Getenv("ARTIFACT_DIR")
	if artifactDir == "" {
		artifactDir = "test/e2e/results/test-plans"
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create artifact directory: %v\n", err)
		return
	}

	// Sanitize test name for filename
	safeTestName := strings.ReplaceAll(testName, " ", "-")
	safeTestName = strings.ReplaceAll(safeTestName, "/", "-")

	// Save as shell script
	scriptPath := filepath.Join(artifactDir, fmt.Sprintf("%s.sh", safeTestName))
	scriptContent := f.recorder.ExportAsShellScript()
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		fmt.Printf("Warning: failed to write test plan script: %v\n", err)
	} else {
		fmt.Printf("✓ Test plan saved to: %s\n", scriptPath)
	}

	// Save as markdown
	mdPath := filepath.Join(artifactDir, fmt.Sprintf("%s.md", safeTestName))
	mdContent := f.recorder.ExportAsMarkdown()
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		fmt.Printf("Warning: failed to write test plan markdown: %v\n", err)
	} else {
		fmt.Printf("✓ Test plan documentation saved to: %s\n", mdPath)
	}
}

// buildTestName constructs a test name from the spec report
func buildTestName(spec types.SpecReport) string {
	hierarchy := spec.ContainerHierarchyTexts
	leaf := spec.LeafNodeText

	if len(hierarchy) > 0 {
		return strings.Join(append(hierarchy, leaf), " ")
	}
	return leaf
}

// ToContext stores the framework in the given context
// This allows framework to be passed through context chains if needed
func (f *Framework) ToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, FrameworkContextKey{}, f)
}

// FromContext retrieves a framework from the given context
// Returns nil if no framework is stored in the context
func FromContext(ctx context.Context) *Framework {
	if f, ok := ctx.Value(FrameworkContextKey{}).(*Framework); ok {
		return f
	}
	return nil
}

// FromReportEntries retrieves a framework from Ginkgo report entries
// This is the preferred way to access framework in ReportAfterEach
// Returns nil if no framework entry is found
func FromReportEntries(entries []types.ReportEntry) *Framework {
	for _, entry := range entries {
		if entry.Name == frameworkReportEntryName {
			// GetRawValue() returns the underlying interface{} value
			if f, ok := entry.Value.GetRawValue().(*Framework); ok {
				return f
			}
		}
	}
	return nil
}

// LogOptions contains options for retrieving pod logs
type LogOptions struct {
	// Container name to get logs from (empty for default container)
	Container string
	// TailLines limits the number of lines from the end of the logs
	TailLines int
	// SinceSeconds returns logs newer than a relative duration (in seconds)
	SinceSeconds int
}

// WaitForLogsContaining waits for a substring to appear in pod logs
// Returns nil if found, error if timeout occurs
func (f *Framework) WaitForLogsContaining(podName, substring string, timeout time.Duration) error {
	fmt.Fprintf(GinkgoWriter, "⏳ Waiting for logs in pod %s to contain: %s\n", podName, substring)

	var lastLogs string
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		logs, err := f.GetPodLogs(podName)
		if err != nil {
			// Not a critical error, pod might not exist yet or be starting
			return false, nil
		}
		lastLogs = logs
		return strings.Contains(logs, substring), nil
	})

	if err != nil {
		elapsed := time.Since(startTime)
		// Truncate logs if too long
		truncatedLogs := lastLogs
		if len(lastLogs) > 500 {
			truncatedLogs = lastLogs[len(lastLogs)-500:] + "\n... (truncated)"
		}
		return fmt.Errorf("timeout waiting for logs to contain '%s' in pod %s after %v. Last logs:\n%s",
			substring, podName, elapsed, truncatedLogs)
	}

	elapsed := time.Since(startTime)
	fmt.Fprintf(GinkgoWriter, "✓ Found expected substring in pod %s logs (took %v)\n", podName, elapsed)
	return nil
}

// WaitForLogsMatching waits for a regex pattern to match in pod logs
// Returns nil if match found, error if timeout occurs or pattern is invalid
func (f *Framework) WaitForLogsMatching(podName, pattern string, timeout time.Duration) error {
	fmt.Fprintf(GinkgoWriter, "⏳ Waiting for logs in pod %s to match pattern: %s\n", podName, pattern)

	// Compile regex pattern
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
	}

	var lastLogs string
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		logs, err := f.GetPodLogs(podName)
		if err != nil {
			// Not a critical error, pod might not exist yet or be starting
			return false, nil
		}
		lastLogs = logs
		return re.MatchString(logs), nil
	})

	if err != nil {
		elapsed := time.Since(startTime)
		// Truncate logs if too long
		truncatedLogs := lastLogs
		if len(lastLogs) > 500 {
			truncatedLogs = lastLogs[len(lastLogs)-500:] + "\n... (truncated)"
		}
		return fmt.Errorf("timeout waiting for logs to match pattern '%s' in pod %s after %v. Last logs:\n%s",
			pattern, podName, elapsed, truncatedLogs)
	}

	elapsed := time.Since(startTime)
	fmt.Fprintf(GinkgoWriter, "✓ Found pattern match in pod %s logs (took %v)\n", podName, elapsed)
	return nil
}

// AssertNoLogsContaining verifies that a substring does NOT appear in pod logs
// Returns nil if substring is absent for the entire check duration, error otherwise
func (f *Framework) AssertNoLogsContaining(podName, substring string, checkDuration time.Duration) error {
	fmt.Fprintf(GinkgoWriter, "⏳ Verifying logs in pod %s do NOT contain: %s (checking for %v)\n",
		podName, substring, checkDuration)

	var foundLogs string
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), checkDuration)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, time.Second, checkDuration, true, func(ctx context.Context) (bool, error) {
		logs, err := f.GetPodLogs(podName)
		if err != nil {
			// Pod might not exist yet, which is acceptable for negative checks
			return false, nil
		}

		if strings.Contains(logs, substring) {
			foundLogs = logs
			// Found the substring - this is a failure for negative assertion
			return true, nil
		}

		// Continue checking
		return false, nil
	})

	// For Consistently-style checks, we want to ensure the substring was NEVER found
	if wait.Interrupted(err) {
		// Timeout means we successfully verified absence for the entire duration
		elapsed := time.Since(startTime)
		fmt.Fprintf(GinkgoWriter, "✓ Verified substring absent in pod %s logs for %v\n", podName, elapsed)
		return nil
	}

	if foundLogs != "" {
		// We found the substring - this is an error
		truncatedLogs := foundLogs
		if len(foundLogs) > 500 {
			truncatedLogs = foundLogs[len(foundLogs)-500:] + "\n... (truncated)"
		}
		return fmt.Errorf("found unexpected substring '%s' in pod %s logs. Last logs:\n%s",
			substring, podName, truncatedLogs)
	}

	// Other error occurred
	if err != nil {
		return fmt.Errorf("error while checking logs for pod %s: %w", podName, err)
	}

	return nil
}

// GetPodLogsWithOptions retrieves logs from a pod with custom options
func (f *Framework) GetPodLogsWithOptions(podName string, opts LogOptions) (string, error) {
	if opts.Container != "" || opts.TailLines > 0 || opts.SinceSeconds > 0 {
		// Use kubectl client methods if options are specified
		if opts.TailLines > 0 {
			return f.kubectl.GetPodLogsTail(podName, opts.TailLines)
		}
		// For other options, we'd need to add more kubectl methods
		// For now, fall back to basic GetPodLogs
		return f.kubectl.GetPodLogs(podName)
	}

	return f.kubectl.GetPodLogs(podName)
}
