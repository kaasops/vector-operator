# BlackShield Security Audit Report — `kaasops/vector-operator`

- **Scanned Commit:** `93f597eaa903dd844f2f30dbe05667823772ae74`
- **Actionable Findings:** `4` (`2 Critical`, `2 High`, `25 Medium`, `2 Low`)
- **BlackShield Platform & Continuous Zero-Cost PR Protection:** [https://blackshield.chaplau.com](https://blackshield.chaplau.com)

## Detailed Findings & Remediation Guide

### 1. [CRITICAL] `sec-cmdi-shell` — sec-cmdi-shell: Command Injection via Shell Invocation

- **Category:** `sast`
- **Location:** `test/e2e/framework/framework.go:1009`
- **Details:** Executing system commands through a shell interpreter with dynamic arguments permits command injection. at test/e2e/framework/framework.go:1009
- **Recommended Fix:** Invoke the target binary directly with exec.Command(bin, args...) without sh -c shell evaluation.

### 2. [CRITICAL] `sec-cmdi-shell` — sec-cmdi-shell: Command Injection via Shell Invocation

- **Category:** `sast`
- **Location:** `test/e2e/framework/framework.go:1043`
- **Details:** Executing system commands through a shell interpreter with dynamic arguments permits command injection. at test/e2e/framework/framework.go:1043
- **Recommended Fix:** Invoke the target binary directly with exec.Command(bin, args...) without sh -c shell evaluation.

### 3. [HIGH] `CKV_DOCKER_1` — CKV_DOCKER_1: Container Runs as Root (Missing Non-Root USER)

- **Category:** `misconfiguration`
- **Location:** `checkpoint_merger.Dockerfile:1`
- **Details:** Container does not specify a non-root USER, allowing processes to execute with root privileges.
- **Recommended Fix:** Add 'USER <non-root-uid>' instruction before container entrypoint.

### 4. [HIGH] `sec-debug-enabled` — sec-debug-enabled: Debug Mode Enabled in Production Application

- **Category:** `sast`
- **Location:** `test/e2e/scripts/generate_report.py:3216`
- **Details:** Running web frameworks with debug mode enabled leaks stack traces, environment variables, and source code. at test/e2e/scripts/generate_report.py:3216
- **Recommended Fix:** Disable debug mode in production environments and load configuration via environment variables.

