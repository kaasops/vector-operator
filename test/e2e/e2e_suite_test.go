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
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/artifacts"
	"github.com/kaasops/vector-operator/test/utils"
)

const (
	operatorNamespace = "vector-operator-system"
	operatorImage     = "example.com/vector-operator:v0.0.1"
)

// artifactCollector manages artifact collection for failed tests
var artifactCollector artifacts.Collector

// readinessTestNamespace is created during controller readiness check
// and cleaned up in AfterSuite to avoid interfering with actual tests
var readinessTestNamespace string

// SynchronizedBeforeSuite ensures setup runs only once across all parallel processes
var _ = SynchronizedBeforeSuite(func() []byte {
	// This function runs ONLY on process #1
	By("building and loading operator image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", operatorImage))
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	cmd = exec.Command("kind", "load", "docker-image", operatorImage, "--name", "kind")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	By("deploying operator via Helm")
	cmd = exec.Command("make", "deploy-helm-e2e",
		fmt.Sprintf("IMG=%s", operatorImage),
		fmt.Sprintf("NAMESPACE=%s", operatorNamespace),
	)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	By("verifying operator is ready")
	// Wait a bit for controllers to start watching CRs
	cmd = exec.Command("kubectl", "wait", "--for=condition=ready",
		"--timeout=60s",
		"pod", "-l", "app.kubernetes.io/name=vector-operator",
		"-n", operatorNamespace,
	)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	// Install shared dependencies once for all tests
	framework.InstallSharedDependencies()

	By("verifying controllers are ready to process resources")
	// Pod being Ready doesn't guarantee controllers are initialized (leader election, cache sync, etc.)
	// Create a test VectorAggregator and verify the controller creates its deployment
	// This ensures the VectorAggregator controller is fully operational before tests start
	verifyControllersReady()

	// Initialize artifact collector
	By("initializing artifact collector")
	config := artifacts.LoadConfigFromEnv()
	collector, err := artifacts.NewCollector(config)
	Expect(err).NotTo(HaveOccurred())

	runID := fmt.Sprintf("%d", time.Now().Unix())
	err = collector.Initialize(runID)
	Expect(err).NotTo(HaveOccurred())

	artifactCollector = collector

	return nil
}, func(data []byte) {
	// This function runs on ALL processes after process #1 completes

	// Initialize artifact collector on all processes (skip if already initialized on process #1)
	if artifactCollector == nil {
		config := artifacts.LoadConfigFromEnv()
		collector, err := artifacts.NewCollector(config)
		if err == nil {
			runID := fmt.Sprintf("%d", time.Now().Unix())
			_ = collector.Initialize(runID)
			artifactCollector = collector
		}
	}
})

// ReportAfterEach collects artifacts for failed tests
var _ = ReportAfterEach(func(report SpecReport) {
	// Skip if collector not initialized or artifacts disabled
	if artifactCollector == nil {
		return
	}

	// Try to get framework from report entries first (preferred method)
	// This is more reliable and works correctly with parallel tests
	f := framework.FromReportEntries(report.ReportEntries)

	// Fallback to registry-based matching for backward compatibility
	// This path will be deprecated once all tests use the new approach
	if f == nil {
		// Find framework for this test by matching namespace in ContainerHierarchyTexts
		var matchedFrameworks []*framework.Framework
		var matchScores []int

		containerTexts := report.ContainerHierarchyTexts
		fullTestPath := strings.Join(containerTexts, " ") + " " + report.LeafNodeText
		fullTestPathLower := strings.ToLower(fullTestPath)

		// Try to find framework by matching namespace patterns
		// Priority: exact namespace match > timestamp suffix match > pattern match
		framework.GetFrameworkRegistry().Range(func(key, value interface{}) bool {
			namespace := key.(string)
			fw := value.(*framework.Framework)

			score := 0

			// Remove timestamp suffix for pattern matching
			// e.g., "test-dataflow-1763129228782243000" -> "test-dataflow"
			baseNamespace := namespace
			if idx := strings.LastIndex(namespace, "-"); idx > 0 {
				possibleBase := namespace[:idx]
				// Check if suffix is a timestamp (all digits)
				suffix := namespace[idx+1:]
				isTimestamp := true
				for _, c := range suffix {
					if c < '0' || c > '9' {
						isTimestamp = false
						break
					}
				}
				if isTimestamp && len(suffix) > 10 { // timestamps are long
					baseNamespace = possibleBase
				}
			}

			// Extract pattern from namespace: "test-normal-mode" -> "normal mode" (with space)
			// This matches Ginkgo's Describe("Normal Mode") to namespace test-normal-mode
			namespacePattern := strings.TrimPrefix(baseNamespace, "test-")
			namespacePattern = strings.ReplaceAll(namespacePattern, "-", " ") // "normal-mode" -> "normal mode"

			// Scoring: higher score = better match
			// Exact namespace match in text - highest priority (1000 points)
			if strings.Contains(fullTestPathLower, strings.ToLower(namespace)) {
				score += 1000
			}

			// Base namespace match (without timestamp) - very high priority (500 points)
			if baseNamespace != namespace && strings.Contains(fullTestPathLower, strings.ToLower(baseNamespace)) {
				score += 500
			}

			// Pattern match: "test-normal-mode" matches "Normal Mode" (50 points)
			if strings.Contains(fullTestPathLower, strings.ToLower(namespacePattern)) {
				score += 50
			}

			// Check if namespace words appear in test path (5 points per word, reduced to avoid false positives)
			namespaceWords := strings.Fields(namespacePattern)
			for _, word := range namespaceWords {
				// Only count meaningful words (skip common words)
				if len(word) > 3 && strings.Contains(fullTestPathLower, strings.ToLower(word)) {
					score += 5
				}
			}

			if score > 0 {
				matchedFrameworks = append(matchedFrameworks, fw)
				matchScores = append(matchScores, score)
			}

			return true // Continue searching all frameworks
		})

		// Select framework with highest match score
		if len(matchedFrameworks) > 0 {
			bestIdx := 0
			bestScore := matchScores[0]
			for i := 1; i < len(matchScores); i++ {
				if matchScores[i] > bestScore {
					bestScore = matchScores[i]
					bestIdx = i
				}
			}
			f = matchedFrameworks[bestIdx]

			if report.Failed() {
				fmt.Fprintf(GinkgoWriter, "🔍 Framework matched via registry (fallback) with score %d: namespace=%s\n",
					bestScore, f.Namespace())
			}
		}
	} else {
		// Successfully retrieved from report entries (preferred path)
		if report.Failed() {
			fmt.Fprintf(GinkgoWriter, "✓ Framework retrieved from report entries: namespace=%s\n",
				f.Namespace())
		}
	}

	if f == nil {
		// No framework found - can't collect artifacts
		if report.Failed() {
			fmt.Fprintf(GinkgoWriter, "⚠️  Cannot collect artifacts: no framework found for test\n")
		}
		return
	}

	// Log artifact collection for failed tests only
	if report.Failed() {
		fmt.Fprintf(GinkgoWriter, "📦 Collecting artifacts for FAILED test: %s (namespace: %s)\n",
			report.LeafNodeText, f.Namespace())
	}

	// Build test info
	testInfo := artifacts.TestInfo{
		Name:           report.FullText(),
		Namespace:      f.Namespace(),
		Failed:         report.Failed(),
		FailureMessage: report.FailureMessage(),
		Duration:       report.RunTime,
		StartTime:      report.StartTime,
		EndTime:        report.EndTime,
		Labels:         report.Labels(),
		KubectlClient:  f.Kubectl(),
	}

	// Collect artifacts with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := artifactCollector.CollectForTest(ctx, testInfo); err != nil {
		fmt.Fprintf(GinkgoWriter, "⚠️  Failed to collect artifacts: %v\n", err)
	} else {
		if report.Failed() {
			fmt.Fprintf(GinkgoWriter, "✓ Artifacts collected successfully\n")
		}
	}
})

// SynchronizedAfterSuite ensures cleanup runs only once across all parallel processes
var _ = SynchronizedAfterSuite(func() {
	// This function runs on ALL processes
	// Nothing needed here
}, func() {
	// This function runs ONLY on process #1 after all others finish

	// Close artifact collector
	if artifactCollector != nil {
		if err := artifactCollector.Close(); err != nil {
			fmt.Fprintf(GinkgoWriter, "⚠️  Failed to close artifact collector: %v\n", err)
		}
	}

	// Clean up readiness test namespace if it was created
	// We defer this cleanup until after all tests to avoid controller overload
	// during test execution (namespace deletion triggers cascading reconciliations)
	if readinessTestNamespace != "" {
		By("cleaning up controller readiness test namespace")
		cmd := exec.Command("kubectl", "delete", "namespace", readinessTestNamespace, "--timeout=30s")
		if _, err := utils.Run(cmd); err != nil {
			fmt.Fprintf(GinkgoWriter, "⚠️  Failed to delete readiness test namespace %s: %v\n", readinessTestNamespace, err)
		}
	}

	// Uninstall shared dependencies
	framework.UninstallSharedDependencies()

	By("undeploying operator via Helm")
	cmd := exec.Command("make", "undeploy-helm-e2e",
		fmt.Sprintf("NAMESPACE=%s", operatorNamespace),
	)
	_, _ = utils.Run(cmd)
})

// verifyControllersReady ensures controllers are fully initialized by creating a test resource
// and verifying the controller processes it. This prevents flaky tests caused by controllers
// still initializing (leader election, cache sync, informers) when pod becomes Ready.
func verifyControllersReady() {
	const (
		testNamespace    = "controller-readiness-test"
		testAggregator   = "readiness-test-aggregator"
		readinessTimeout = 60 * time.Second
		pollInterval     = 2 * time.Second
	)

	// Store namespace name for cleanup in AfterSuite
	// We don't clean up immediately to avoid controller overload during test execution
	// (namespace deletion triggers cascading reconciliations that can interfere with tests)
	readinessTestNamespace = testNamespace

	// Create temporary namespace for readiness test (idempotent)
	// First delete if exists and wait for full deletion
	deleteCmd := exec.Command("kubectl", "delete", "namespace", testNamespace, "--ignore-not-found=true", "--wait=true", "--timeout=30s")
	_ = deleteCmd.Run() // Ignore errors, namespace might not exist

	// Now create the namespace
	cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
	if _, err := utils.Run(cmd); err != nil {
		Fail(fmt.Sprintf("Failed to create readiness test namespace: %v", err))
	}

	// Create a minimal VectorAggregator CR
	aggregatorYAML := fmt.Sprintf(`apiVersion: observability.kaasops.io/v1alpha1
kind: VectorAggregator
metadata:
  name: %s
  namespace: %s
spec:
  selector: {}
  replicas: 1
  image: timberio/vector:0.40.0-alpine
`, testAggregator, testNamespace)

	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(aggregatorYAML)
	if _, err := utils.Run(cmd); err != nil {
		Fail(fmt.Sprintf("Failed to create test VectorAggregator: %v", err))
	}

	// Wait for controller to create the deployment
	deploymentName := testAggregator + "-aggregator"
	startTime := time.Now()

	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "deployment", deploymentName,
			"-n", testNamespace, "-o", "name")
		output, err := utils.Run(cmd)
		if err != nil {
			return fmt.Errorf("deployment not found: %w", err)
		}
		if !strings.Contains(string(output), "deployment") {
			return fmt.Errorf("deployment not found")
		}
		return nil
	}, readinessTimeout, pollInterval).Should(Succeed(),
		"VectorAggregator controller should create deployment %s in namespace %s within %v. "+
			"This indicates controller is not ready. Pod may be Ready but controllers are still initializing "+
			"(leader election, cache sync, webhook registration, informers startup).",
		deploymentName, testNamespace, readinessTimeout)

	elapsed := time.Since(startTime)
	fmt.Fprintf(GinkgoWriter, "✓ Controllers ready in %.2fs (deployment %s created)\n",
		elapsed.Seconds(), deploymentName)
}

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting vector-operator suite\n")
	RunSpecs(t, "e2e suite")
}
