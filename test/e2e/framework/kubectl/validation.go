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

package kubectl

import (
	"fmt"
	"regexp"
)

// ValidateNamespace validates namespace against RFC 1123 DNS Label requirements.
// A valid namespace must:
// - Be 1-63 characters long
// - Contain only lowercase alphanumeric characters or '-'
// - Start with an alphanumeric character
// - End with an alphanumeric character
// Empty namespace is allowed for cluster-scoped resources
func ValidateNamespace(namespace string) error {
	// Allow empty namespace for cluster-scoped resources
	if len(namespace) == 0 {
		return nil
	}

	if len(namespace) > 63 {
		return fmt.Errorf("namespace length must be 1-63 characters, got %d", len(namespace))
	}

	// RFC 1123 DNS Label regex: lowercase alphanumeric and hyphens only
	// Must start and end with alphanumeric
	if !regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`).MatchString(namespace) {
		return fmt.Errorf("invalid namespace format: %s (must match RFC 1123 DNS Label)", namespace)
	}

	return nil
}

// ValidateResourceName validates Kubernetes resource names against RFC 1123 DNS Subdomain requirements.
// A valid resource name must:
// - Be 1-253 characters long
// - Contain only lowercase alphanumeric characters, '-', or '.'
// - Start with an alphanumeric character
// - End with an alphanumeric character
func ValidateResourceName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("resource name cannot be empty")
	}

	if len(name) > 253 {
		return fmt.Errorf("resource name length must be 1-253 characters, got %d", len(name))
	}

	// RFC 1123 DNS Subdomain regex
	if !regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`).MatchString(name) {
		return fmt.Errorf("invalid resource name format: %s (must match RFC 1123 DNS Subdomain)", name)
	}

	return nil
}

// ValidateResourceType validates Kubernetes resource type names.
// These are typically lowercase and may contain '.' for API groups.
func ValidateResourceType(resourceType string) error {
	if len(resourceType) == 0 {
		return fmt.Errorf("resource type cannot be empty")
	}

	// Allow alphanumeric, dots for API groups (e.g., "apps.deployment")
	if !regexp.MustCompile(`^[a-z0-9]([a-z0-9\.\-]*[a-z0-9])?$`).MatchString(resourceType) {
		return fmt.Errorf("invalid resource type format: %s", resourceType)
	}

	return nil
}

// ValidateLabelSelector validates Kubernetes label selectors.
// Label selectors have specific syntax requirements for keys and values.
func ValidateLabelSelector(selector string) error {
	if selector == "" {
		// Empty selector is valid (means no filter)
		return nil
	}

	// Basic validation: check for suspicious characters that could be used for injection
	// Allow alphanumeric, dots, hyphens, underscores, slashes (for label keys), equals, commas
	if !regexp.MustCompile(`^[a-zA-Z0-9\.\_\-/=,]+$`).MatchString(selector) {
		return fmt.Errorf("invalid label selector format: %s", selector)
	}

	return nil
}

// ValidateTimeout validates timeout strings used with kubectl commands.
// Valid formats: "30s", "5m", "1h", "2m0s", "1h30m", "1h30m45s"
// Accepts both simple format (5m) and Go duration format (5m0s)
func ValidateTimeout(timeout string) error {
	if timeout == "" {
		return fmt.Errorf("timeout cannot be empty")
	}

	// Allow Go duration format: combinations of hours, minutes, seconds
	// Examples: 30s, 5m, 1h, 2m0s, 1h30m, 1h30m45s
	// Pattern: optional hours (Nh), optional minutes (Nm), optional seconds (Ns)
	if !regexp.MustCompile(`^([0-9]+h)?([0-9]+m)?([0-9]+(\.[0-9]+)?[sµμn]s?)?$`).MatchString(timeout) {
		return fmt.Errorf("invalid timeout format: %s (must be Go duration like '30s', '5m', '2m0s', or '1h30m')", timeout)
	}

	// Ensure at least one component is present
	if !regexp.MustCompile(`[0-9]`).MatchString(timeout) {
		return fmt.Errorf("invalid timeout format: %s (must contain at least one time component)", timeout)
	}

	return nil
}

// ValidateJSONPath validates JSONPath expressions used with kubectl.
// This is a basic validation to prevent obvious injection attempts.
func ValidateJSONPath(jsonPath string) error {
	if jsonPath == "" {
		return fmt.Errorf("jsonPath cannot be empty")
	}

	// Basic validation: JSONPath should not contain shell metacharacters
	// Allow alphanumeric, dots, brackets, quotes, underscores, hyphens, asterisks, colons, backslashes
	// Backslash is needed for escaping dots in keys like .data['agent\.json']
	if !regexp.MustCompile(`^[\w\.\[\]\{\}'":\*\-\s,@\?\\]+$`).MatchString(jsonPath) {
		return fmt.Errorf("invalid jsonPath format: %s", jsonPath)
	}

	return nil
}
