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

package errors

import (
	"strings"
)

// Centralized error classification for e2e tests
// Provides consistent error handling across kubectl operations

// IsAlreadyExists checks if error indicates resource already exists
func IsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "AlreadyExists") ||
		strings.Contains(errStr, "already exists")
}

// IsNotFound checks if error indicates resource not found
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "NotFound") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "(NotFound)")
}

// IsConflict checks if error indicates resource conflict
func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "Conflict") ||
		strings.Contains(errStr, "the object has been modified")
}

// IsTimeout checks if error indicates timeout
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "timed out") ||
		strings.Contains(errStr, "context deadline exceeded")
}

// IsConnectionError checks if error indicates connection/network issue
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dial tcp")
}

// IsTransient checks if error is likely transient and retriable
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	return IsTimeout(err) ||
		IsConnectionError(err) ||
		IsConflict(err) ||
		strings.Contains(err.Error(), "Internal error") ||
		strings.Contains(err.Error(), "TooManyRequests") ||
		strings.Contains(err.Error(), "ServerTimeout")
}

// IsIgnorable checks if error can be safely ignored in test setup/teardown
func IsIgnorable(err error) bool {
	if err == nil {
		return true
	}
	// AlreadyExists and NotFound are often acceptable in test lifecycle
	return IsAlreadyExists(err) || IsNotFound(err)
}
