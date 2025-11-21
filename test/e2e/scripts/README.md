# E2E Test Scripts

Utilities for working with e2e test results and test environment.

## Available Scripts

### generate_report.py

Generates an interactive HTML pivot grid report from e2e test results.

**Usage:**
```bash
# From project root
make test-report

# Or directly
cd test/e2e/results
python3 ../scripts/generate_report.py
```

**What it does:**
- Scans all `run-*` directories in `test/e2e/results/`
- Parses test metadata and results from each run
- Generates `test_results_report.html` with interactive pivot grid
- Shows test stability across multiple runs (flaky tests, always-failing tests, etc.)

**Requirements:**
- Python 3.6+
- Test results in `test/e2e/results/run-YYYY-MM-DD-HHMMSS/` format

**Output:**
- `test/e2e/results/test_results_report.html` - Interactive HTML report

## Adding New Scripts

When adding new test utilities:
1. Place the script in this directory
2. Update this README with usage instructions
3. Add a Makefile target if appropriate (see `make help`)
4. Ensure the script has proper error handling and help text
