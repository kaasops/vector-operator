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

package config

import (
	"os"
	"time"
)

// Default timeout values
const (
	// Resource creation timeouts
	defaultDeploymentCreateTimeout = 120 * time.Second // Increased for resource-heavy aggregator deployments
	defaultDeploymentReadyTimeout  = 120 * time.Second
	defaultNamespaceDeleteTimeout  = 120 * time.Second // Increased timeout to handle slow namespace termination

	// Pipeline validation timeouts
	defaultPipelineValidTimeout = 2 * time.Minute
	defaultConfigCheckTimeout   = 30 * time.Second

	// Service check timeouts
	defaultServiceCreateTimeout = 2 * time.Minute

	// Polling intervals
	defaultDefaultPollInterval = 2 * time.Second
	defaultFastPollInterval    = 1 * time.Second
	defaultSlowPollInterval    = 2 * time.Second // Reduced from 5s - more responsive polling

	// Test spec timeouts
	defaultDefaultTestTimeout = 5 * time.Minute
	defaultLongTestTimeout    = 10 * time.Minute
)

// Configurable timeout variables (can be overridden via environment variables)
var (
	// Resource creation timeouts
	DeploymentCreateTimeout = getEnvDuration("E2E_DEPLOYMENT_CREATE_TIMEOUT", defaultDeploymentCreateTimeout)
	DeploymentReadyTimeout  = getEnvDuration("E2E_DEPLOYMENT_READY_TIMEOUT", defaultDeploymentReadyTimeout)
	NamespaceDeleteTimeout  = getEnvDuration("E2E_NAMESPACE_DELETE_TIMEOUT", defaultNamespaceDeleteTimeout)

	// Pipeline validation timeouts
	PipelineValidTimeout = getEnvDuration("E2E_PIPELINE_VALID_TIMEOUT", defaultPipelineValidTimeout)
	ConfigCheckTimeout   = getEnvDuration("E2E_CONFIG_CHECK_TIMEOUT", defaultConfigCheckTimeout)

	// Service check timeouts
	ServiceCreateTimeout = getEnvDuration("E2E_SERVICE_CREATE_TIMEOUT", defaultServiceCreateTimeout)

	// Polling intervals
	DefaultPollInterval = getEnvDuration("E2E_DEFAULT_POLL_INTERVAL", defaultDefaultPollInterval)
	FastPollInterval    = getEnvDuration("E2E_FAST_POLL_INTERVAL", defaultFastPollInterval)
	SlowPollInterval    = getEnvDuration("E2E_SLOW_POLL_INTERVAL", defaultSlowPollInterval)

	// Test spec timeouts
	DefaultTestTimeout = getEnvDuration("E2E_DEFAULT_TEST_TIMEOUT", defaultDefaultTestTimeout)
	LongTestTimeout    = getEnvDuration("E2E_LONG_TEST_TIMEOUT", defaultLongTestTimeout)
)

// getEnvDuration reads a duration from environment variable, falling back to default if not set or invalid
func getEnvDuration(envVar string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(envVar); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			return duration
		}
		// If parsing fails, fall back to default (silently to avoid test noise)
	}
	return defaultValue
}

// GetPollInterval returns appropriate poll interval based on timeout
func GetPollInterval(timeout time.Duration) time.Duration {
	if timeout < 30*time.Second {
		return FastPollInterval
	}
	if timeout > 2*time.Minute {
		return SlowPollInterval
	}
	return DefaultPollInterval
}
