# CI/CD Documentation

## GitHub Actions Workflows

### E2E Tests Workflow

The E2E tests workflow automatically runs end-to-end tests for every pull request and push to the main branch.

**Workflow File:** `.github/workflows/e2e-tests.yaml`

#### Triggers

- **Push to main/master**: Runs on every push to the main or master branch
- **Pull Requests**: Runs on PRs targeting main/master
- **Manual**: Can be triggered manually via GitHub Actions UI (workflow_dispatch)

#### Workflow Steps

1. **Checkout code**: Clones the repository
2. **Set up Go**: Installs Go using version from `go.mod`
3. **Install dependencies**: Installs kubebuilder for CRD generation
4. **Create kind cluster**: Creates a single-node Kubernetes cluster using `scripts/kind-config-ci.yaml`
5. **Verify cluster**: Checks cluster health and connectivity
6. **Build image**: Builds operator Docker image
7. **Load image**: Loads image into the kind cluster
8. **Run E2E tests**: Executes `make test-e2e` with JUnit reporting
9. **Upload test results**: Saves test results as artifacts (retained for 7 days)
10. **Publish test results**: Publishes JUnit results as GitHub check

#### Configuration

**Kind Cluster (CI):** `scripts/kind-config-ci.yaml`
- Single control-plane node
- Control-plane allows scheduling workloads for faster execution
- Port mappings for ingress (80, 443)

#### Test Reports

Test results are available in multiple formats:

1. **JUnit XML**: `test/e2e/results/run-*/reports/junit-report.xml`
   - Machine-readable format
   - Used by GitHub Actions to display test results

2. **JSON Report**: `test/e2e/results/run-*/reports/report.json`
   - Detailed test execution data
   - Suitable for programmatic analysis

3. **Plain text log**: `test/e2e/results/run-*/reports/test-output.log`
   - Human-readable test output
   - Contains full test execution logs

4. **HTML Report**: Generated via `make test-report`
   - Interactive visualization
   - Requires Python 3

#### Artifacts

**Test Results** (7 days retention):
- JUnit XML report
- JSON report
- Plain text test output
- Failure artifacts (pod logs, events, resource states)
- Available for all workflow runs

#### Viewing Results

1. **GitHub UI**:
   - Go to Actions tab → E2E Tests workflow
   - Click on a specific run to view results

2. **PR Checks**:
   - Test results appear as a check on PRs
   - Click "Details" to view full report

#### Running E2E Tests Locally

```bash
# Run e2e tests with full reporting
make test-e2e

# Run with fail-fast (stop on first failure)
make test-e2e E2E_FAIL_FAST=true

# Run with label filter
make test-e2e E2E_LABEL_FILTER="smoke"

# Run with description
make test-e2e E2E_RUN_DESCRIPTION="Testing new feature"

# Generate HTML report from results
make test-report
```

#### Troubleshooting

**Tests fail in CI but pass locally:**
- Check timing issues (CI may be slower)
- Verify kind-config-ci.yaml configuration
- Check resource limits in CI environment

**Cluster creation timeout:**
- Increase `wait` timeout in workflow
- Check Docker daemon health in CI
- Verify kind version compatibility

**Image loading fails:**
- Ensure Docker build succeeds
- Check image names match between build and load steps
- Verify kind cluster name is correct

**Tests timeout:**
- Default timeout is 30 minutes
- Adjust `timeout-minutes` in workflow if needed
- Check for hanging pods or resources

#### Manual Trigger

To manually trigger the E2E tests workflow:

1. Go to Actions tab in GitHub
2. Select "E2E Tests" workflow
3. Click "Run workflow" button
4. Select branch and click "Run workflow"

#### Performance

**Typical execution time:**
- Cluster creation: ~1-2 minutes
- Image build: ~2-3 minutes
- Image load: ~30 seconds
- E2E tests: ~10-15 minutes
- **Total: ~15-20 minutes**

### Lint Workflow

**Workflow File:** `.github/workflows/lint.yaml`

#### Jobs

1. **golangci-lint**: Runs golangci-lint with project configuration
2. **go fmt**: Checks code formatting
3. **go vet**: Runs Go static analysis

#### Configuration

Linter configuration is defined in `.golangci.yml`:

```yaml
linters:
  enable:
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - errcheck
    - gofmt
    - goimports

linters-settings:
  goimports:
    local-prefixes: github.com/kaasops/vector-operator
```

#### Running Locally

```bash
# Run linter
make lint

# Run linter with auto-fix
make lint-fix
```
