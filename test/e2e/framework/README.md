# E2E Test Framework

A comprehensive testing framework for Vector Operator e2e tests, built on top of Ginkgo/Gomega.

## Overview

This framework provides a high-level API for writing maintainable and readable e2e tests. It handles common operations like namespace management, resource deployment, status checking, and cleanup, while providing custom matchers for intuitive assertions.

## Key Features

- **High-level API** - Simple methods for common operations
- **Automatic namespace management** - Creates and cleans up test namespaces
- **Shared dependencies** - Install Prometheus Operator and cert-manager once for all tests
- **Custom Gomega matchers** - Readable DSL-style assertions
- **Test metrics tracking** - Automatic timing measurements
- **YAML templating** - Dynamic test data generation
- **Centralized timeouts** - Consistent timeout configuration

## Quick Start

### Basic Test Structure

```go
package e2e

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/kaasops/vector-operator/test/e2e/framework"
    "github.com/kaasops/vector-operator/test/e2e/framework/assertions"
    "github.com/kaasops/vector-operator/test/e2e/framework/config"
)

var _ = Describe("My Feature", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
    f := framework.NewFramework("my-feature-test")

    BeforeAll(f.Setup)
    AfterAll(f.Teardown)

    Context("Basic Functionality", func() {
        It("should work correctly", func() {
            // Deploy resources
            f.ApplyTestData("normal-mode/agent.yaml")
            f.ApplyTestData("normal-mode/pipeline-basic.yaml")

            // Wait for readiness
            f.WaitForPipelineValid("basic-pipeline")

            // Assert using custom matchers
            Eventually(f.Pipeline("basic-pipeline")).Should(assertions.BeValid())
        })
    })
})
```

## Core Components

### 1. Framework Object

The main entry point for all test operations.

```go
// Create a new framework instance
f := framework.NewFramework("test-namespace-prefix")

// Setup creates namespace, initializes metrics, and registers framework
// for artifact collection via Ginkgo report entries
f.Setup()

// Teardown cleans up namespace and resources
f.Teardown()
```

**Framework Registration**

The framework uses Ginkgo's report entry system for context propagation instead of global state:

```go
// In your test:
f := framework.NewFramework("test-ns")
f.Setup()  // Automatically stores framework in Ginkgo report entries

// In ReportAfterEach (for artifact collection):
// Framework is automatically retrieved from report entries
f := framework.FromReportEntries(report.ReportEntries)
if f != nil {
    // Collect artifacts using framework's kubectl client and namespace
}
```

**Benefits:**
- ✅ No global state - eliminates race conditions in parallel tests
- ✅ Direct association between test and framework
- ✅ Works correctly with Ginkgo's parallel execution
- ✅ Backward compatible - still supports legacy registry-based matching as fallback

**Context Support (Advanced)**

For advanced use cases, the framework can be stored in Go contexts:

```go
// Store in context
ctx := f.ToContext(context.Background())

// Retrieve from context
f := framework.FromContext(ctx)
if f != nil {
    // Use framework
}
```

### 2. Resource Management

#### Apply Test Data

```go
// Load and apply YAML from test/e2e/testdata/
f.ApplyTestData("normal-mode/agent.yaml")
f.ApplyTestData("normal-mode/pipeline-basic.yaml")
```

#### Create Multiple Resources

```go
// Create 100 pipelines from template
creationTime := f.CreateMultiplePipelinesFromTemplate(
    "scalability/pipeline-template.yaml",
    "pipeline-NNNN",  // Placeholder to replace
    100,              // Count
)
fmt.Printf("Created 100 pipelines in %v\n", creationTime)
```

### 3. Wait Operations

```go
// Wait for deployment to be ready (uses config.DeploymentReadyTimeout)
f.WaitForDeploymentReady("aggregator-name")

// Wait for pipeline to become valid (uses config.PipelineValidTimeout)
f.WaitForPipelineValid("pipeline-name")
```

### 3.1. Log Polling Methods

Standardized methods for waiting on log content, eliminating boilerplate Eventually blocks:

```go
// Wait for substring to appear in pod logs
err := f.WaitForLogsContaining("pod-name", "expected text", 2*time.Minute)
Expect(err).NotTo(HaveOccurred())

// Wait for regex pattern to match in pod logs
err := f.WaitForLogsMatching("pod-name", `\d+ requests processed`, 1*time.Minute)
Expect(err).NotTo(HaveOccurred())

// Verify substring does NOT appear in logs (negative assertion)
err := f.AssertNoLogsContaining("pod-name", "ERROR", 30*time.Second)
Expect(err).NotTo(HaveOccurred())

// Get logs with options
logs, err := f.GetPodLogsWithOptions("pod-name", framework.LogOptions{
    TailLines: 100,
})
Expect(err).NotTo(HaveOccurred())
```

**Before (verbose):**
```go
var logs string
Eventually(func() bool {
    l, err := f.GetPodLogs("pod-name")
    if err != nil {
        return false
    }
    logs = l
    return strings.Contains(logs, "expected text")
}, 2*time.Minute, 1*time.Second).Should(BeTrue())
```

**After (concise):**
```go
err := f.WaitForLogsContaining("pod-name", "expected text", 2*time.Minute)
Expect(err).NotTo(HaveOccurred())
```

### 4. Status Queries

```go
// Get pipeline status field
role := f.GetPipelineStatus("my-pipeline", "role")

// Count valid pipelines
validCount, err := f.CountValidPipelines()

// Count services with label
serviceCount := f.CountServicesWithLabel("app.kubernetes.io/component=Aggregator")
```

### 5. Custom Matchers

The framework provides custom Gomega matchers for readable assertions:

#### Pipeline Matchers

```go
// Check if pipeline is valid
Eventually(f.Pipeline("test-pipeline")).Should(assertions.BeValid())
Eventually(f.Pipeline("test-pipeline")).Should(assertions.BeInvalid())

// Check role
Expect(f.Pipeline("test-pipeline")).To(assertions.HaveRole("agent"))
Expect(f.Pipeline("test-pipeline")).To(assertions.HaveRole("aggregator"))

// Check error message contains substring
Expect(f.Pipeline("invalid-pipeline")).To(assertions.HaveErrorContaining("validation"))
```

#### Service Matchers

```go
// Check if service exists
Eventually(f.Service("my-service")).Should(assertions.Exist())

// Check service port
Expect(f.Service("my-service")).To(assertions.HavePort("9090"))
```

## Shared Dependencies

Shared dependencies (Prometheus Operator, cert-manager) are installed once in `BeforeSuite` and shared across all tests.

### Installation

Handled automatically in `test/e2e/e2e_suite_test.go`:

```go
var _ = BeforeSuite(func() {
    // ... operator deployment

    // Install shared dependencies once
    framework.InstallSharedDependencies()
})
```

### Benefits

- **Faster test execution** - ~3 minutes saved per test run
- **More stable** - Avoid repeated install/uninstall cycles
- **Cleaner logs** - No AlreadyExists errors

### Usage in Tests

Tests automatically use shared dependencies:

```go
// No need to install/uninstall in individual tests
var _ = Describe("My Test", func() {
    f := framework.NewFramework("test-ns")

    BeforeAll(f.Setup)    // Just creates namespace
    AfterAll(f.Teardown)  // Just cleans up namespace

    // Dependencies are already available
})
```

## Test Labels

Ginkgo v2 provides a powerful label system for categorizing and filtering tests. Labels are simply strings that can be attached to test specs.

### Standard Labels (defined in `config/constants.go`)

```go
Label(config.LabelSmoke)      // Quick smoke tests
Label(config.LabelFast)       // Fast tests (<2 min)
Label(config.LabelSlow)       // Slow tests (>5 min)
Label(config.LabelStress)     // Stress/load tests
Label(config.LabelRegression) // Regression tests
```

### Priority Labels

```go
Label(config.LabelP0)     // P0: Critical, must always pass
Label(config.LabelP1)     // P1: High priority
Label(config.LabelP2)     // P2: Medium priority

// Example usage:
var _ = Describe("Source Type Constraints [P0-Security]",
    Label(config.LabelConstraint, config.LabelP0, config.LabelSecurity, config.LabelFast), func() {
    // ...
})
```

### Category Labels

```go
Label(config.LabelSecurity)    // Security-related tests
Label(config.LabelConstraint)  // Constraint validation tests
```

### Combined Labels

```go
// Multiple labels for fine-grained filtering
Label(config.LabelSmoke, config.LabelFast)                     // Quick smoke test
Label(config.LabelP0, config.LabelSecurity, config.LabelFast)  // Critical security test
Label(config.LabelStress, config.LabelSlow)                    // Long-running load test
```

### Filtering Tests

Run specific test categories:

```bash
# Run only smoke tests
ginkgo --label-filter=smoke ./test/e2e/

# Run fast tests
ginkgo --label-filter=fast ./test/e2e/

# Exclude slow tests
ginkgo --label-filter="!slow" ./test/e2e/

# Run critical security tests
ginkgo --label-filter="p0 && security" ./test/e2e/

# Run smoke tests but exclude slow ones
ginkgo --label-filter="smoke && !slow" ./test/e2e/

# Run either constraint or security tests
ginkgo --label-filter="constraint || security" ./test/e2e/
```

### Best Practices

1. **Use descriptive labels**: Labels should clearly indicate what they categorize
2. **Combine standard + custom labels**: Mix project-standard labels with feature-specific ones
3. **Document critical labels**: If using priority labels (P0, P1), document their meaning
4. **Keep labels in test names**: Add labels to Describe text for better readability (e.g., `[P0-Security]`)

### Available Labels

List all labels in the test suite:
```bash
ginkgo labels ./test/e2e/
```

## Test Metrics

The framework automatically tracks test operation timing:

```go
// Metrics are collected automatically
f.Setup()                        // Tracks setup time
f.WaitForDeploymentReady(...)   // Tracks deployment wait time
f.WaitForPipelineValid(...)     // Tracks pipeline validation time
f.Teardown()                     // Tracks cleanup time

// Metrics are printed after each test
// Example output:
// 📊 Test Metrics:
//     Setup: 60.777ms
//     Deployment Wait: 4.299s
//     Pipeline Validation: 5.098s
//     Cleanup: 11.034s
//     Total: 20.472s
```

## Environment Variables

The framework supports several environment variables for customization:

### E2E_TESTDATA_PATH

Customize the location of test data files. Defaults to `test/e2e/testdata`.

```bash
# Use custom test data directory
E2E_TESTDATA_PATH=/path/to/testdata make test-e2e

# Run tests with test data in a different location
E2E_TESTDATA_PATH=/tmp/my-testdata ginkgo test/e2e/
```

**Use cases:**
- Testing with different data sets
- CI/CD pipelines with mounted test data
- Temporary test data generation
- Isolated test environments

### E2E_DRY_RUN

Run tests in dry-run mode to generate test plans without executing them.

```bash
E2E_DRY_RUN=true make test-e2e
```

### E2E_RECORD_STEPS

Record test steps for debugging and reproducibility.

```bash
E2E_RECORD_STEPS=true make test-e2e
```

## Timeouts Configuration

Centralized timeout configuration in `config/timeouts.go`:

```go
const (
    DeploymentCreateTimeout = 90 * time.Second   // Wait for deployment to be created
    DeploymentReadyTimeout  = 120 * time.Second  // Wait for deployment to be ready
    PipelineValidTimeout    = 2 * time.Minute    // Wait for pipeline validation
    ServiceCreateTimeout    = 2 * time.Minute    // Wait for service creation
    DefaultPollInterval     = 2 * time.Second    // Default polling interval
    SlowPollInterval        = 5 * time.Second    // Slower polling for heavy ops
)
```

## Advanced Examples

### Example 1: Basic Pipeline Test

```go
It("should create and validate a basic pipeline with agent", func() {
    // Deploy resources
    f.ApplyTestData("normal-mode/agent.yaml")
    f.ApplyTestData("normal-mode/pipeline-basic.yaml")

    // Wait for readiness
    f.WaitForPipelineValid("basic-pipeline")

    // Verify pipeline configuration
    Eventually(f.Pipeline("basic-pipeline")).Should(assertions.BeValid())
    Expect(f.Pipeline("basic-pipeline")).To(assertions.HaveRole("agent"))

    // Verify agent processes the pipeline
    Eventually(func() error {
        return f.VerifyAgentHasPipeline("normal-agent", "basic-pipeline")
    }, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())
})
```

### Example 2: Aggregator Test

```go
It("should deploy aggregator and process pipelines", func() {
    // Deploy aggregator
    f.ApplyTestData("normal-mode/aggregator.yaml")
    f.WaitForDeploymentReady("my-aggregator-aggregator")

    // Create pipeline with aggregator role
    f.ApplyTestData("normal-mode/pipeline-aggregator-role.yaml")
    f.WaitForPipelineValid("aggregator-pipeline")

    // Verify role
    Expect(f.Pipeline("aggregator-pipeline")).To(assertions.HaveRole("aggregator"))
})
```

### Example 3: Scalability Test

```go
It("should handle 100 pipelines successfully", func() {
    const pipelineCount = 100

    // Deploy aggregator
    f.ApplyTestData("scalability/aggregator.yaml")
    f.WaitForDeploymentReady("scale-aggregator-aggregator")

    // Create 100 pipelines from template
    creationTime := f.CreateMultiplePipelinesFromTemplate(
        "scalability/pipeline-template.yaml",
        "pipeline-NNNN",
        pipelineCount,
    )
    GinkgoWriter.Printf("✨ Created %d pipelines in %v\n", pipelineCount, creationTime)

    // Wait for all to become valid (with progress logging)
    Eventually(func() (int, error) {
        validCount, err := f.CountValidPipelines()
        if validCount > 0 {
            GinkgoWriter.Printf("📊 Validation progress: %d/%d pipelines valid (%.0f%%)\n",
                validCount, pipelineCount, float64(validCount)/float64(pipelineCount)*100)
        }
        return validCount, nil
    }, 7*time.Minute, 10*time.Second).Should(Equal(pipelineCount))
})
```

## Best Practices

### 1. Use Descriptive Test Names

```go
// Good
It("should create and validate a basic pipeline with agent", func() { ... })

// Bad
It("test1", func() { ... })
```

### 2. Use Eventually for Async Operations

```go
// Good - waits for condition to be met
Eventually(f.Pipeline("test-pipeline")).Should(assertions.BeValid())

// Bad - may fail if not ready immediately
Expect(f.Pipeline("test-pipeline")).To(assertions.BeValid())
```

### 3. Use Appropriate Labels

```go
// Mark fast smoke tests
var _ = Describe("Quick Validation", Label(config.LabelSmoke, config.LabelFast), ...)

// Mark slow stress tests
var _ = Describe("Load Test", Label(config.LabelStress, config.LabelSlow), ...)
```

### 4. Leverage Test Metrics

```go
// Metrics are automatically tracked and displayed
BeforeAll(f.Setup)    // Tracks setup time
AfterAll(f.Teardown)  // Tracks cleanup time + displays all metrics
```

### 5. Use Custom Matchers

```go
// Good - readable and clear intent
Expect(f.Pipeline("test")).To(assertions.BeValid())
Expect(f.Pipeline("test")).To(assertions.HaveRole("agent"))

// Bad - verbose and less clear
status := f.GetPipelineStatus("test", "configCheckResult")
Expect(status).To(Equal("true"))
role := f.GetPipelineStatus("test", "role")
Expect(role).To(Equal("agent"))
```

## Directory Structure

```
test/e2e/framework/
├── README.md              # This file
├── framework.go           # Main framework implementation
├── lifecycle.go           # Shared dependencies management
├── resources.go           # Resource utilities
├── config/
│   ├── constants.go       # Test labels and constants
│   └── timeouts.go        # Timeout configuration
├── kubectl/
│   ├── client.go          # Kubectl wrapper
│   ├── wait.go            # Wait utilities
│   └── validation.go      # Validation helpers
├── assertions/
│   └── matchers.go        # Custom Gomega matchers
├── artifacts/
│   ├── collector.go       # Artifact collection
│   ├── storage.go         # Artifact storage
│   └── config.go          # Artifact configuration
├── errors/
│   └── errors.go          # Custom error types
└── recorder/
    └── recorder.go        # Step recorder
```

## Contributing

When adding new features to the framework:

1. Keep the API simple and intuitive
2. Add appropriate error handling
3. Track timing metrics for new operations
4. Add custom matchers for common assertions
5. Update this README with examples

## Troubleshooting

### AlreadyExists Errors

If you see `AlreadyExists` errors for Prometheus Operator or cert-manager:
- Ensure you're not installing dependencies in `BeforeAll`
- Dependencies are automatically installed in `BeforeSuite` via `framework.InstallSharedDependencies()`

### Timeout Errors

If tests timeout:
- Check `config/timeouts.go` and adjust as needed
- Use `SlowPollInterval` for expensive operations
- Consider increasing go test timeout: `-timeout=15m`

### Namespace Not Found

If you see namespace errors:
- Ensure `BeforeAll(f.Setup)` is called
- Verify namespace name matches test data YAML files

## References

- [Ginkgo Documentation](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)
- [Vector Operator E2E Tests](../README.md)
