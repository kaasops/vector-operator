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

package recorder

import (
	"fmt"
	"strings"
	"time"
)

// TestRecorder records test operations for reproducibility and documentation
type TestRecorder struct {
	testName  string
	namespace string
	steps     []TestStep
	startTime time.Time
	stepOrder int
}

// TestStep represents a single operation in a test
type TestStep struct {
	Order       int
	Command     string // Exact kubectl or shell command
	Description string // Human-readable description
	Input       string // YAML or other input data
	Expected    string // Expected result
	WaitFor     string // Wait condition (e.g., "condition=available")
	Timeout     string // Timeout for the operation
}

// NewTestRecorder creates a new test recorder
func NewTestRecorder(namespace string) *TestRecorder {
	return &TestRecorder{
		namespace: namespace,
		steps:     make([]TestStep, 0),
		startTime: time.Now(),
		stepOrder: 0,
	}
}

// SetTestName sets the test name for this recording
func (r *TestRecorder) SetTestName(name string) {
	r.testName = name
}

// RecordStep records a test step
func (r *TestRecorder) RecordStep(step TestStep) {
	r.stepOrder++
	step.Order = r.stepOrder
	r.steps = append(r.steps, step)
}

// GetSteps returns all recorded steps
func (r *TestRecorder) GetSteps() []TestStep {
	return r.steps
}

// ExportAsShellScript exports the recorded steps as an executable shell script
func (r *TestRecorder) ExportAsShellScript() string {
	var sb strings.Builder

	// Script header
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("# E2E Test Playbook\n")
	sb.WriteString(fmt.Sprintf("# Test: %s\n", r.testName))
	sb.WriteString(fmt.Sprintf("# Namespace: %s\n", r.namespace))
	sb.WriteString(fmt.Sprintf("# Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	// Shell settings for safety
	sb.WriteString("set -e  # Exit on error\n")
	sb.WriteString("set -u  # Exit on undefined variable\n")
	sb.WriteString("set -o pipefail  # Catch errors in pipes\n\n")

	// Variables
	sb.WriteString(fmt.Sprintf("NAMESPACE='%s'\n", r.namespace))
	sb.WriteString("KUBECTL='kubectl'\n")
	sb.WriteString("TMPDIR=$(mktemp -d)\n")
	sb.WriteString("trap 'rm -rf $TMPDIR' EXIT\n\n")

	// Helper functions
	sb.WriteString(r.generateHelperFunctions())

	// Main steps
	sb.WriteString("# Test Steps\n")
	sb.WriteString("echo '═══════════════════════════════════════════════════════════'\n")
	sb.WriteString(fmt.Sprintf("echo 'Test: %s'\n", r.testName))
	sb.WriteString("echo '═══════════════════════════════════════════════════════════'\n\n")

	for _, step := range r.steps {
		sb.WriteString(fmt.Sprintf("# Step %d: %s\n", step.Order, step.Description))
		sb.WriteString("echo '───────────────────────────────────────────────────────────'\n")
		sb.WriteString(fmt.Sprintf("log_info 'Step %d: %s'\n", step.Order, step.Description))
		sb.WriteString("echo '───────────────────────────────────────────────────────────'\n")

		// If there's input data, save it to a temporary file
		if step.Input != "" {
			tmpFile := fmt.Sprintf("$TMPDIR/step-%d.yaml", step.Order)
			sb.WriteString(fmt.Sprintf("cat <<'EOF' > %s\n", tmpFile))
			sb.WriteString(step.Input)
			sb.WriteString("\nEOF\n")

			// Modify command to use the temp file
			if strings.Contains(step.Command, "kubectl apply -f -") {
				modifiedCmd := strings.Replace(step.Command, "kubectl apply -f -", fmt.Sprintf("kubectl apply -f %s", tmpFile), 1)
				sb.WriteString(modifiedCmd + "\n")
			} else {
				sb.WriteString(step.Command + "\n")
			}
		} else {
			sb.WriteString(step.Command + "\n")
		}

		// Add expected result as comment
		if step.Expected != "" {
			sb.WriteString(fmt.Sprintf("# Expected: %s\n", step.Expected))
		}

		// Add wait condition if specified
		if step.WaitFor != "" {
			sb.WriteString(fmt.Sprintf("# Wait for: %s (timeout: %s)\n", step.WaitFor, step.Timeout))
		}

		sb.WriteString("\n")
	}

	// Success message
	sb.WriteString("echo '═══════════════════════════════════════════════════════════'\n")
	sb.WriteString("log_success 'Test completed successfully!'\n")
	sb.WriteString("echo '═══════════════════════════════════════════════════════════'\n")

	return sb.String()
}

// ExportAsMarkdown exports the recorded steps as Markdown documentation
func (r *TestRecorder) ExportAsMarkdown() string {
	var sb strings.Builder

	// Document header
	sb.WriteString(fmt.Sprintf("# Test Plan: %s\n\n", r.testName))
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Namespace**: `%s`\n\n", r.namespace))

	// Prerequisites
	sb.WriteString("## Prerequisites\n\n")
	sb.WriteString("- Kubernetes cluster with Vector Operator installed\n")
	sb.WriteString("- kubectl configured with cluster access\n")
	sb.WriteString("- Appropriate RBAC permissions\n\n")

	// Test steps
	sb.WriteString("## Test Steps\n\n")

	for _, step := range r.steps {
		sb.WriteString(fmt.Sprintf("### Step %d: %s\n\n", step.Order, step.Description))

		// Command
		sb.WriteString("**Command**:\n")
		sb.WriteString("```bash\n")
		sb.WriteString(step.Command + "\n")
		sb.WriteString("```\n\n")

		// Input YAML if present
		if step.Input != "" {
			sb.WriteString("**Input YAML**:\n")
			sb.WriteString("```yaml\n")
			sb.WriteString(step.Input + "\n")
			sb.WriteString("```\n\n")
		}

		// Wait condition if present
		if step.WaitFor != "" {
			sb.WriteString(fmt.Sprintf("**Wait Condition**: `%s`\n\n", step.WaitFor))
			if step.Timeout != "" {
				sb.WriteString(fmt.Sprintf("**Timeout**: %s\n\n", step.Timeout))
			}
		}

		// Expected result
		if step.Expected != "" {
			sb.WriteString(fmt.Sprintf("**Expected Result**: %s\n\n", step.Expected))
		}

		sb.WriteString("---\n\n")
	}

	return sb.String()
}

// generateHelperFunctions generates helper shell functions for the script
func (r *TestRecorder) generateHelperFunctions() string {
	return `# Helper Functions
log_info() {
    echo "[INFO] $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_error() {
    echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') - $1" >&2
}

log_success() {
    echo "[SUCCESS] $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

check_deployment() {
    local name=$1
    local namespace=${2:-$NAMESPACE}
    log_info "Checking deployment $name in namespace $namespace..."
    kubectl get deployment "$name" -n "$namespace" &>/dev/null || {
        log_error "Deployment $name not found!"
        return 1
    }
    log_info "Deployment $name exists"
}

check_service() {
    local name=$1
    local namespace=${2:-$NAMESPACE}
    log_info "Checking service $name in namespace $namespace..."
    kubectl get service "$name" -n "$namespace" &>/dev/null || {
        log_error "Service $name not found!"
        return 1
    }
    log_info "Service $name exists"
}

wait_for_pods() {
    local label=$1
    local namespace=${2:-$NAMESPACE}
    local timeout=${3:-120s}
    log_info "Waiting for pods with label $label in namespace $namespace..."
    kubectl wait --for=condition=Ready pods -l "$label" -n "$namespace" --timeout="$timeout" || {
        log_error "Pods with label $label did not become ready within $timeout"
        return 1
    }
    log_info "Pods are ready"
}

`
}

// Clear clears all recorded steps
func (r *TestRecorder) Clear() {
	r.steps = make([]TestStep, 0)
	r.stepOrder = 0
}
