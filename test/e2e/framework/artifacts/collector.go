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

package artifacts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:golint,revive

	"github.com/kaasops/vector-operator/test/e2e/framework/kubectl"
)

// TestInfo contains information about a test execution
type TestInfo struct {
	Name           string
	Namespace      string
	Failed         bool
	FailureMessage string
	Duration       time.Duration
	StartTime      time.Time
	EndTime        time.Time
	Labels         []string

	// Test sequence tracking (for degradation analysis)
	SequenceNumber int           // Which test in the run (1, 2, 3...)
	OperatorAge    time.Duration // How long operator has been running

	// Kubernetes context
	KubectlClient *kubectl.Client
}

// Collector manages artifact collection for e2e tests
type Collector interface {
	// Initialize sets up the collector for a test run
	Initialize(runID string) error

	// CollectForTest collects artifacts for a test
	CollectForTest(ctx context.Context, testInfo TestInfo) error

	// Close finalizes the collector and writes summary
	Close() error
}

// collector implements the Collector interface
type collector struct {
	config   Config
	storage  *Storage
	metadata *MetadataBuilder
	runStart time.Time

	// Operator tracking (for degradation analysis)
	operatorStartTime time.Time

	// Statistics
	totalTests  int
	failedTests int
	testCounter int // Counter for directory naming
}

// NewCollector creates a new artifact collector
func NewCollector(config Config) (Collector, error) {
	return &collector{
		config:   config,
		runStart: time.Now(),
	}, nil
}

// Initialize sets up the collector for a test run
func (c *collector) Initialize(runID string) error {
	if !c.config.Enabled {
		return nil
	}

	// Create storage
	storage, err := NewStorage(c.config.BaseDir, runID, c.config.MaxResourceSize)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}
	c.storage = storage

	// Create metadata builder
	c.metadata = NewMetadataBuilder(storage)

	// Get operator start time for degradation tracking
	c.operatorStartTime = c.getOperatorStartTime()

	fmt.Fprintf(GinkgoWriter, "📦 Artifact collection initialized: %s\n", storage.GetRunDir())
	return nil
}

// CollectForTest collects artifacts for a specific test
func (c *collector) CollectForTest(ctx context.Context, testInfo TestInfo) error {
	if !c.config.Enabled {
		return nil
	}

	// Update statistics
	c.totalTests++
	if testInfo.Failed {
		c.failedTests++
	}

	// Skip passed tests if configured
	if !testInfo.Failed && c.config.CollectOnFailureOnly {
		return nil
	}

	// Increment counter and create short directory name
	c.testCounter++
	shortName := createShortTestName(testInfo.Name, c.testCounter)

	// Fill in tracking fields for degradation analysis
	testInfo.SequenceNumber = c.totalTests
	if !c.operatorStartTime.IsZero() {
		testInfo.OperatorAge = time.Since(c.operatorStartTime)
	}

	// Create test directory
	testDir, err := c.storage.CreateTestDir(shortName)
	if err != nil {
		return fmt.Errorf("failed to create test directory: %w", err)
	}

	fmt.Fprintf(GinkgoWriter, "📦 Collecting artifacts for test: %s\n", testInfo.Name)
	collectionStart := time.Now()

	// Collect with timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.CollectionTimeout)
	defer cancel()

	// Track collected artifacts
	inventory := ArtifactInventory{
		LogFiles:      []string{},
		ResourceFiles: []string{},
		EventFiles:    []string{},
	}

	// P0 artifacts - critical for debugging
	if err := c.collectP0Artifacts(ctx, testInfo, testDir, &inventory); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Warning: P0 artifact collection had errors: %v\n", err)
	}

	// Write test metadata
	collectionDuration := time.Since(collectionStart)
	inventory.CollectionTime = collectionDuration.String()

	meta := BuildTestMetadata(testInfo, inventory)
	if err := c.metadata.WriteTestMetadata(meta, testDir); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Warning: Failed to write test metadata: %v\n", err)
	}

	fmt.Fprintf(GinkgoWriter, "✅ Artifacts collected in %v (%d files)\n",
		collectionDuration, len(inventory.LogFiles)+len(inventory.ResourceFiles)+len(inventory.EventFiles))

	return nil
}

// collectP0Artifacts collects P0 (critical) artifacts
func (c *collector) collectP0Artifacts(ctx context.Context, testInfo TestInfo, testDir string, inventory *ArtifactInventory) error {
	kubectl := testInfo.KubectlClient
	namespace := testInfo.Namespace

	if kubectl == nil || namespace == "" {
		return fmt.Errorf("missing kubectl client or namespace")
	}

	// 1. Pod status (JSON) - fast, critical
	if err := c.collectPodStatus(ctx, kubectl, namespace, testDir, inventory); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect pod status: %v\n", err)
	}

	// 2. Operator controller logs - critical for debugging
	if err := c.collectOperatorLogs(ctx, testDir, inventory, testInfo.StartTime); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect operator logs: %v\n", err)
	}

	// 2a. Operator health (pod describe, events) - critical for degradation diagnosis
	if err := c.collectOperatorHealth(ctx, testDir, inventory, testInfo.StartTime); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect operator health: %v\n", err)
	}

	// 3. Pipeline status - fast, shows validation state
	if err := c.collectPipelineStatus(ctx, kubectl, namespace, testDir, inventory); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect pipeline status: %v\n", err)
	}

	// 4. Namespace events - fast, shows what happened
	if err := c.collectEvents(ctx, kubectl, namespace, testDir, inventory); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect namespace events: %v\n", err)
	}

	// 5. Resource metadata (Deployment/DaemonSet/Service basic info)
	if err := c.collectResourceMetadata(ctx, kubectl, namespace, testDir, inventory); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect resource metadata: %v\n", err)
	}

	return nil
}

// collectPodStatus collects status of all pods in the namespace
func (c *collector) collectPodStatus(ctx context.Context, kubectl *kubectl.Client, namespace, testDir string, inventory *ArtifactInventory) error {
	// Get all pods
	pods, err := kubectl.GetPodsByLabel("")
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	inventory.PodCount = len(pods)

	for _, podName := range pods {
		// Get pod status as JSON
		output, err := kubectl.GetWithJsonPath("pod", podName, ".status")
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "  ⚠️  Failed to get status for pod %s: %v\n", podName, err)
			continue
		}

		filename := fmt.Sprintf("%s-status.json", podName)
		if err := c.storage.WriteFile(testDir, "pods", filename, []byte(output)); err != nil {
			fmt.Fprintf(GinkgoWriter, "  ⚠️  Failed to write pod status for %s: %v\n", podName, err)
			continue
		}

		inventory.ResourceFiles = append(inventory.ResourceFiles, "pods/"+filename)

		// Also get pod logs (last N lines)
		logs, err := kubectl.GetPodLogsTail(podName, c.config.MaxLogLines)
		if err != nil {
			// Pod might not have logs yet, that's okay
			continue
		}

		// Truncate logs if needed
		truncatedLogs := TruncateLogLines([]byte(logs), c.config.MaxLogLines)

		logFilename := fmt.Sprintf("%s.log", podName)
		if err := c.storage.WriteFile(testDir, "logs", logFilename, truncatedLogs); err != nil {
			fmt.Fprintf(GinkgoWriter, "  ⚠️  Failed to write logs for %s: %v\n", podName, err)
			continue
		}

		inventory.LogFiles = append(inventory.LogFiles, "logs/"+logFilename)
	}

	return nil
}

// collectOperatorLogs collects operator controller logs
func (c *collector) collectOperatorLogs(ctx context.Context, testDir string, inventory *ArtifactInventory, testStart time.Time) error {
	// Get operator pod name from vector-operator-system namespace
	operatorNs := "vector-operator-system"
	operatorClient := kubectl.NewClient(operatorNs)

	pods, err := operatorClient.GetPodsByLabel("app.kubernetes.io/name=vector-operator")
	if err != nil || len(pods) == 0 {
		return fmt.Errorf("failed to find operator controller pod: %w", err)
	}

	// Get logs from first controller pod (should only be one)
	podName := pods[0]

	// Add 1 minute buffer before test start to capture context
	// This helps see what was happening just before the test started
	logsSince := testStart.Add(-1 * time.Minute)

	// Use time-based log collection to get logs relevant to this test
	// This fixes the issue where long-running operator pods would only return
	// the last N lines from the entire pod lifetime, missing test-specific logs
	logs, err := operatorClient.GetPodLogsSinceTime(podName, logsSince, c.config.MaxLogLines)
	if err != nil {
		return fmt.Errorf("failed to get operator logs: %w", err)
	}

	// Truncate logs if still needed
	truncatedLogs := TruncateLogLines([]byte(logs), c.config.MaxLogLines)

	if err := c.storage.WriteFile(testDir, "logs", "operator-controller.log", truncatedLogs); err != nil {
		return fmt.Errorf("failed to write operator logs: %w", err)
	}

	inventory.LogFiles = append(inventory.LogFiles, "logs/operator-controller.log")
	return nil
}

// collectOperatorHealth collects operator pod describe and events for degradation diagnosis
func (c *collector) collectOperatorHealth(ctx context.Context, testDir string, inventory *ArtifactInventory, testStart time.Time) error {
	const operatorNs = "vector-operator-system"
	operatorClient := kubectl.NewClient(operatorNs)

	// Get operator pod
	pods, err := operatorClient.GetPodsByLabel("app.kubernetes.io/name=vector-operator")
	if err != nil || len(pods) == 0 {
		return fmt.Errorf("failed to find operator pod: %w", err)
	}
	podName := pods[0]

	// 1. Get pod describe (shows conditions, events, restarts, QoS, resource requests/limits)
	describeCmd := exec.Command("kubectl", "describe", "pod", podName, "-n", operatorNs)
	describeOutput, err := describeCmd.CombinedOutput()
	if err == nil {
		if err := c.storage.WriteFile(testDir, "operator", "pod-describe.txt", describeOutput); err != nil {
			return fmt.Errorf("failed to write operator pod describe: %w", err)
		}
		inventory.ResourceFiles = append(inventory.ResourceFiles, "operator/pod-describe.txt")
	}

	// 2. Get cluster-wide events related to operator (evictions, OOMKills, etc.)
	// Use time window from 2 minutes before test start to catch context
	sinceTime := testStart.Add(-2 * time.Minute).UTC().Format(time.RFC3339)

	// Get all Warning events in operator namespace
	eventsCmd := exec.Command("kubectl", "get", "events", "-n", operatorNs,
		"--field-selector", "type=Warning",
		"--since-time", sinceTime)
	eventsOutput, err := eventsCmd.CombinedOutput()
	if err == nil && len(eventsOutput) > 0 {
		if err := c.storage.WriteFile(testDir, "operator", "warning-events.txt", eventsOutput); err != nil {
			return fmt.Errorf("failed to write operator warning events: %w", err)
		}
		inventory.EventFiles = append(inventory.EventFiles, "operator/warning-events.txt")
	}

	// 3. Get deployment describe (shows replica status, conditions)
	deployDescribeCmd := exec.Command("kubectl", "describe", "deployment", "vector-operator-controller-manager", "-n", operatorNs)
	deployDescribeOutput, err := deployDescribeCmd.CombinedOutput()
	if err == nil {
		if err := c.storage.WriteFile(testDir, "operator", "deployment-describe.txt", deployDescribeOutput); err != nil {
			return fmt.Errorf("failed to write deployment describe: %w", err)
		}
		inventory.ResourceFiles = append(inventory.ResourceFiles, "operator/deployment-describe.txt")
	}

	// 4. Collect pprof profiles (goroutine, heap) for memory/goroutine leak diagnosis
	if err := c.collectPprofProfiles(ctx, testDir, inventory, podName, operatorNs); err != nil {
		// Non-fatal: pprof may not be enabled in production
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect pprof profiles (may not be enabled): %v\n", err)
	}

	return nil
}

// collectPipelineStatus collects VectorPipeline CR status
func (c *collector) collectPipelineStatus(ctx context.Context, kubectl *kubectl.Client, namespace, testDir string, inventory *ArtifactInventory) error {
	// Get all VectorPipeline CRs
	pipelinesOutput, err := kubectl.GetAll("vectorpipeline", "")
	if err != nil {
		return fmt.Errorf("failed to list pipelines: %w", err)
	}

	if pipelinesOutput == "" {
		// No pipelines, that's okay
		return nil
	}

	pipelines := strings.Fields(pipelinesOutput)
	for _, pipelineName := range pipelines {
		// Get pipeline status
		status, err := kubectl.GetWithJsonPath("vectorpipeline", pipelineName, ".status")
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "  ⚠️  Failed to get status for pipeline %s: %v\n", pipelineName, err)
			continue
		}

		filename := fmt.Sprintf("vectorpipeline-%s-status.json", pipelineName)
		if err := c.storage.WriteFile(testDir, "resources", filename, []byte(status)); err != nil {
			fmt.Fprintf(GinkgoWriter, "  ⚠️  Failed to write pipeline status for %s: %v\n", pipelineName, err)
			continue
		}

		inventory.ResourceFiles = append(inventory.ResourceFiles, "resources/"+filename)
	}

	return nil
}

// collectEvents collects Kubernetes events from the namespace
func (c *collector) collectEvents(ctx context.Context, kubectl *kubectl.Client, namespace, testDir string, inventory *ArtifactInventory) error {
	// Get events - use kubectl.Client to run kubectl get events
	// Since we don't have a GetEvents method, we'll use a simple approach
	eventsOutput, err := kubectl.Get("events", "")
	if err != nil {
		// Events might not exist, that's okay
		return nil
	}

	if err := c.storage.WriteFile(testDir, "events", "namespace-events.txt", eventsOutput); err != nil {
		return fmt.Errorf("failed to write namespace events: %w", err)
	}

	inventory.EventFiles = append(inventory.EventFiles, "events/namespace-events.txt")
	return nil
}

// collectResourceMetadata collects basic metadata about Deployments, DaemonSets, Services
func (c *collector) collectResourceMetadata(ctx context.Context, kubectl *kubectl.Client, namespace, testDir string, inventory *ArtifactInventory) error {
	resourceTypes := []string{"deployment", "daemonset", "service"}

	for _, resourceType := range resourceTypes {
		resources, err := kubectl.GetAll(resourceType, "")
		if err != nil {
			continue // Resource type might not exist
		}

		if resources == "" {
			continue
		}

		resourceNames := strings.Fields(resources)
		for _, resourceName := range resourceNames {
			// Get resource metadata (name, labels, status)
			output, err := kubectl.Get(resourceType, resourceName)
			if err != nil {
				continue
			}

			filename := fmt.Sprintf("%s-%s.yaml", resourceType, resourceName)
			if err := c.storage.WriteFile(testDir, "resources", filename, output); err != nil {
				fmt.Fprintf(GinkgoWriter, "  ⚠️  Failed to write %s/%s: %v\n", resourceType, resourceName, err)
				continue
			}

			inventory.ResourceFiles = append(inventory.ResourceFiles, "resources/"+filename)
		}
	}

	return nil
}

// Close finalizes the collector and writes run summary
func (c *collector) Close() error {
	if !c.config.Enabled || c.storage == nil {
		return nil
	}

	runEnd := time.Now()
	runMeta := RunMetadata{
		RunID:        c.storage.GetRunID(),
		StartTime:    c.runStart,
		EndTime:      runEnd,
		TotalTests:   c.totalTests,
		FailedTests:  c.failedTests,
		PassedTests:  c.totalTests - c.failedTests,
		ArtifactsDir: c.storage.GetRunDir(),
		Environment: map[string]string{
			"E2E_ARTIFACTS_ENABLED":         fmt.Sprintf("%t", c.config.Enabled),
			"E2E_ARTIFACTS_ON_FAILURE_ONLY": fmt.Sprintf("%t", c.config.CollectOnFailureOnly),
			"E2E_ARTIFACTS_MAX_LOG_LINES":   fmt.Sprintf("%d", c.config.MaxLogLines),
			"E2E_ARTIFACTS_COLLECTION_TIME": runEnd.Sub(c.runStart).String(),
		},
		GitCommit:   os.Getenv("E2E_GIT_COMMIT"),
		GitBranch:   os.Getenv("E2E_GIT_BRANCH"),
		GitDirty:    os.Getenv("E2E_GIT_DIRTY"),
		Description: os.Getenv("E2E_RUN_DESCRIPTION"),
	}

	if err := c.metadata.WriteRunMetadata(runMeta); err != nil {
		return fmt.Errorf("failed to write run metadata: %w", err)
	}

	fmt.Fprintf(GinkgoWriter, "\n📦 Artifact Collection Summary:\n")
	fmt.Fprintf(GinkgoWriter, "   Location: %s\n", c.storage.GetRunDir())
	fmt.Fprintf(GinkgoWriter, "   Total tests: %d\n", c.totalTests)
	fmt.Fprintf(GinkgoWriter, "   Failed tests with artifacts: %d\n", c.failedTests)
	fmt.Fprintf(GinkgoWriter, "   Duration: %v\n\n", runEnd.Sub(c.runStart))

	return nil
}

// createShortTestName creates a short, numbered directory name from full test name
// Input:  "Artifact Verification should intentionally fail to test artifact collection"
// Output: "01-artifact-verification"
func createShortTestName(fullName string, counter int) string {
	// Split by spaces to get the first part (Describe block name)
	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return fmt.Sprintf("%02d-unknown", counter)
	}

	// Stop at "should" or "[" - these mark the end of test suite name
	var suiteParts []string
	for _, word := range parts {
		lower := strings.ToLower(word)
		// Stop at common separators
		if lower == "should" || strings.HasPrefix(word, "[") {
			break
		}
		// Clean up and add word
		clean := strings.Trim(word, "()[]{}")
		if clean != "" {
			suiteParts = append(suiteParts, clean)
		}
		// Limit to first 3-4 words
		if len(suiteParts) >= 4 {
			break
		}
	}

	// Fallback if nothing found
	if len(suiteParts) == 0 {
		suiteParts = parts[:1]
	}

	// Join and lowercase
	mainPart := strings.ToLower(strings.Join(suiteParts, "-"))

	// Remove any remaining special characters
	replacer := strings.NewReplacer(
		"(", "", ")", "",
		"[", "", "]", "",
		"{", "", "}", "",
	)
	mainPart = replacer.Replace(mainPart)

	// Limit length to reasonable size
	const maxLen = 40
	if len(mainPart) > maxLen {
		mainPart = mainPart[:maxLen]
	}

	// Add counter prefix for uniqueness and ordering
	return fmt.Sprintf("%02d-%s", counter, mainPart)
}

// getOperatorStartTime retrieves the operator pod's start time for degradation tracking
func (c *collector) getOperatorStartTime() time.Time {
	const operatorNs = "vector-operator-system"
	operatorClient := kubectl.NewClient(operatorNs)

	// Get operator pods
	pods, err := operatorClient.GetPodsByLabel("app.kubernetes.io/name=vector-operator")
	if err != nil || len(pods) == 0 {
		// If we can't get operator pod, return zero time
		return time.Time{}
	}

	// Get pod start time
	startTimeStr, err := operatorClient.GetWithJsonPath("pod", pods[0], ".status.startTime")
	if err != nil {
		return time.Time{}
	}

	// Parse RFC3339 timestamp
	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(startTimeStr))
	if err != nil {
		return time.Time{}
	}

	return startTime
}

// collectPprofProfiles collects pprof profiles from the operator pod for leak diagnosis
// Uses kubectl port-forward since distroless image doesn't have wget/curl
func (c *collector) collectPprofProfiles(ctx context.Context, testDir string, inventory *ArtifactInventory, podName, namespace string) error {
	const pprofPort = "6060"
	const localPort = "16060" // Use high port to avoid conflicts

	// Start port-forward in background
	portForwardCmd := exec.Command("kubectl", "port-forward",
		fmt.Sprintf("pod/%s", podName),
		"-n", namespace,
		fmt.Sprintf("%s:%s", localPort, pprofPort))

	if err := portForwardCmd.Start(); err != nil {
		return fmt.Errorf("failed to start port-forward: %w", err)
	}
	defer func() {
		if portForwardCmd.Process != nil {
			_ = portForwardCmd.Process.Kill()
		}
	}()

	// Wait a bit for port-forward to establish
	time.Sleep(2 * time.Second)

	// Collect goroutine profile (text format for readability)
	goroutineCmd := exec.Command("curl", "-s",
		fmt.Sprintf("http://localhost:%s/debug/pprof/goroutine?debug=1", localPort))
	goroutineOutput, err := goroutineCmd.CombinedOutput()
	if err == nil && len(goroutineOutput) > 0 {
		if err := c.storage.WriteFile(testDir, "operator", "pprof-goroutine.txt", goroutineOutput); err != nil {
			return fmt.Errorf("failed to write goroutine profile: %w", err)
		}
		inventory.ResourceFiles = append(inventory.ResourceFiles, "operator/pprof-goroutine.txt")
	} else {
		return fmt.Errorf("failed to collect goroutine profile: %w", err)
	}

	// Collect heap profile (text format for readability)
	heapCmd := exec.Command("curl", "-s",
		fmt.Sprintf("http://localhost:%s/debug/pprof/heap?debug=1", localPort))
	heapOutput, err := heapCmd.CombinedOutput()
	if err == nil && len(heapOutput) > 0 {
		if err := c.storage.WriteFile(testDir, "operator", "pprof-heap.txt", heapOutput); err != nil {
			return fmt.Errorf("failed to write heap profile: %w", err)
		}
		inventory.ResourceFiles = append(inventory.ResourceFiles, "operator/pprof-heap.txt")
	} else {
		return fmt.Errorf("failed to collect heap profile: %w", err)
	}

	return nil
}
