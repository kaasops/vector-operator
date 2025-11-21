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
	"os"
	"strconv"
	"time"
)

// Default configuration values
const (
	defaultBaseDir           = "test/e2e/artifacts"
	defaultMaxLogLines       = 500
	defaultMaxResourceSize   = 10 * 1024 * 1024  // 10MB
	defaultMaxTotalSize      = 100 * 1024 * 1024 // 100MB per test
	defaultCollectionTimeout = 30 * time.Second
	defaultEnabled           = true
	defaultOnFailureOnly     = true
	defaultMinimalOnly       = false
)

// Config defines artifact collection behavior
type Config struct {
	// Collection control
	Enabled              bool // Master switch for artifact collection
	CollectOnFailureOnly bool // Collect artifacts only for failed tests
	CollectMinimalOnly   bool // Collect only P0 artifacts (fast path)

	// Storage paths
	BaseDir string // Base directory for artifact storage

	// Size limits (prevent artifact bloat)
	MaxLogLines     int   // Maximum log lines per pod
	MaxResourceSize int64 // Maximum size for single resource (bytes)
	MaxTotalSize    int64 // Maximum total size per test (bytes)

	// Timeouts
	CollectionTimeout time.Duration // Maximum time to collect artifacts

	// Filters
	NamespacePatterns []string // Namespace patterns to collect from
	PodLabelSelectors []string // Pod label selectors for filtering
}

// LoadConfigFromEnv loads configuration from environment variables
// Following Phase 1 pattern: ENV-based config with sensible defaults
func LoadConfigFromEnv() Config {
	return Config{
		Enabled:              getEnvBool("E2E_ARTIFACTS_ENABLED", defaultEnabled),
		CollectOnFailureOnly: getEnvBool("E2E_ARTIFACTS_ON_FAILURE_ONLY", defaultOnFailureOnly),
		CollectMinimalOnly:   getEnvBool("E2E_ARTIFACTS_MINIMAL_ONLY", defaultMinimalOnly),

		BaseDir: getEnvString("E2E_ARTIFACTS_DIR", defaultBaseDir),

		MaxLogLines:     getEnvInt("E2E_ARTIFACTS_MAX_LOG_LINES", defaultMaxLogLines),
		MaxResourceSize: getEnvInt64("E2E_ARTIFACTS_MAX_RESOURCE_SIZE", defaultMaxResourceSize),
		MaxTotalSize:    getEnvInt64("E2E_ARTIFACTS_MAX_TOTAL_SIZE", defaultMaxTotalSize),

		CollectionTimeout: getEnvDuration("E2E_ARTIFACTS_TIMEOUT", defaultCollectionTimeout),

		NamespacePatterns: []string{"test-*"},
		PodLabelSelectors: []string{},
	}
}

// Helper functions for ENV parsing

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	result, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return result
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return result
}

func getEnvInt64(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return result
}

func getEnvString(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	result, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return result
}
