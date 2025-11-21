# E2E Test Artifact Collection

Automatic collection of debugging artifacts when e2e tests fail.

## Features

- **Automatic**: Collects artifacts only on test failures (configurable)
- **Safe**: Never fails tests due to collection errors
- **Fast**: < 1s overhead for passing tests, < 30s for failing tests
- **Comprehensive**: Logs, pod status, events, resources

## Configuration (ENV Variables)

All configuration is ENV-based with sensible defaults:

### Collection Control
- `E2E_ARTIFACTS_ENABLED` (default: `true`) - Master switch
- `E2E_ARTIFACTS_ON_FAILURE_ONLY` (default: `true`) - Only collect on failures
- `E2E_ARTIFACTS_MINIMAL_ONLY` (default: `false`) - P0 artifacts only

### Storage
- `E2E_ARTIFACTS_DIR` (default: `test/e2e/artifacts`) - Base directory

### Size Limits
- `E2E_ARTIFACTS_MAX_LOG_LINES` (default: `500`) - Max log lines per pod
- `E2E_ARTIFACTS_MAX_RESOURCE_SIZE` (default: `10485760`) - Max 10MB per file
- `E2E_ARTIFACTS_MAX_TOTAL_SIZE` (default: `104857600`) - Max 100MB per test

### Timeouts
- `E2E_ARTIFACTS_TIMEOUT` (default: `30s`) - Max collection time

## Usage

### Running Tests with Artifacts

```bash
# Default behavior (enabled, on-failure-only)
make test-e2e

# Disable artifacts
E2E_ARTIFACTS_ENABLED=false make test-e2e

# Collect for all tests (even passing)
E2E_ARTIFACTS_ON_FAILURE_ONLY=false make test-e2e

# Increase log lines
E2E_ARTIFACTS_MAX_LOG_LINES=1000 make test-e2e
```

### Artifact Location

Artifacts are stored in:
```
test/e2e/artifacts/
└── run-{timestamp}/
    ├── metadata.json
    └── {test-name}/
        ├── metadata.json
        ├── logs/
        │   ├── operator-controller.log
        │   └── pod-{name}.log
        ├── pods/
        │   └── {pod-name}-status.json
        ├── resources/
        │   ├── vectorpipeline-{name}-status.json
        │   └── deployment-{name}.yaml
        └── events/
            └── namespace-events.txt
```

### Unified Test Results

When you run `make test-e2e`, all results are automatically saved in a unified structure with reports and artifacts correlated by timestamp:

```bash
# Run tests - results automatically saved with timestamp
make test-e2e

# Results structure:
test/e2e/results/run-{timestamp}/
├── reports/
│   ├── junit-report.xml    # JUnit XML for CI integration
│   ├── report.json          # Ginkgo JSON report
│   └── test-output.log      # Full test output logs
└── artifacts/               # Debug artifacts (only for failed tests)
    ├── metadata.json        # Run-level metadata
    └── {test-name}/         # Per-test artifacts
        ├── metadata.json
        ├── logs/
        ├── pods/
        ├── resources/
        └── events/
```

**Benefits**:
- Single runID correlates all reports and artifacts
- Easy to navigate - everything in one directory
- CI/CD friendly - upload one directory
- Helpful output with quick analysis commands

### CI Integration (GitHub Actions)

```yaml
- name: Run E2E Tests
  run: make test-e2e

- name: Upload Test Results
  if: always()  # Upload even if tests fail
  uses: actions/upload-artifact@v4
  with:
    name: e2e-results-${{ github.run_number }}
    path: test/e2e/results/
    retention-days: 30
```

## Collected Artifacts (P0 - MVP)

### Critical for Debugging
1. **Pod Status JSON** - Conditions, restarts, phase
2. **Operator Controller Logs** - Time-filtered logs (test duration + 1min buffer)
3. **VectorPipeline CR Status** - Validation results
4. **Namespace Events** - What happened in test namespace
5. **Resource Metadata** - Deployments, DaemonSets, Services

### Future (Phase 2)
- Full pod logs (all containers)
- Full pod descriptions
- Vector agent/aggregator logs
- ConfigCheck pod logs
- Timeline reconstruction

## Architecture

- **Thread-safe**: Uses `sync.Map` for parallel test support
- **Graceful degradation**: Collection errors don't fail tests
- **Size limits**: Prevents CI artifact bloat
- **Atomic writes**: Temp file + rename for reliability

## Performance

- **Passing tests**: < 1s overhead (if `ON_FAILURE_ONLY=true`)
- **Failing tests**: < 30s collection time
- **Storage**: < 100MB per test, < 500MB per run

## Troubleshooting

### No artifacts collected
1. Check `E2E_ARTIFACTS_ENABLED=true`
2. Verify test is using `framework.NewFramework()` or `framework.Shared()`
3. Check GinkgoWriter output for warning messages

### Artifacts too large
1. Reduce `E2E_ARTIFACTS_MAX_LOG_LINES` (default: 500)
2. Enable `E2E_ARTIFACTS_MINIMAL_ONLY=true`
3. Check individual file sizes with `E2E_ARTIFACTS_MAX_RESOURCE_SIZE`

### Collection timeout
1. Increase `E2E_ARTIFACTS_TIMEOUT` (default: 30s)
2. Check kubectl connectivity
3. Review namespace resource count

## Important Bug Fixes

### Time-based Log Collection (Fixed)
**Problem**: Previously, operator logs were collected using `kubectl logs --tail 500`, which retrieved the last 500 lines from the entire pod lifetime. In long-running test suites (e.g., full e2e runs lasting 15+ minutes), the operator pod could generate thousands of log lines, causing the last 500 lines to exclude logs from earlier failing tests.

**Example**: A test failing at 18:05-18:07 would collect operator logs from 16:02-16:03 (the pod's startup logs), completely missing the relevant reconciliation attempts.

**Solution**: Implemented time-based log collection using `kubectl logs --since-time` with the test's start time (+ 1 minute buffer). This ensures operator logs are collected only for the relevant time period, regardless of how long the pod has been running.

**Impact**:
- Fixes flaky test debugging where operator logs were missing
- Enables reliable root cause analysis for race conditions
- Reduces confusion when logs don't match test timeline

## Development

See architect design document for Phase 2+ enhancements.
