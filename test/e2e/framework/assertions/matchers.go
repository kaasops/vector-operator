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

package assertions

import (
	"fmt"
	"strings"

	"github.com/onsi/gomega/types"

	"github.com/kaasops/vector-operator/test/e2e/framework/kubectl"
)

// PipelineResource represents a pipeline for matching
type PipelineResource struct {
	namespace string
	name      string
	kubectl   *kubectl.Client
}

// NewPipelineResource creates a new pipeline resource wrapper
func NewPipelineResource(namespace, name string) *PipelineResource {
	return &PipelineResource{
		namespace: namespace,
		name:      name,
		kubectl:   kubectl.NewClient(namespace),
	}
}

// resourceType returns the correct resource type based on namespace
// Empty namespace = cluster-scoped (ClusterVectorPipeline)
// Non-empty namespace = namespaced (VectorPipeline)
func (p *PipelineResource) resourceType() string {
	if p.namespace == "" {
		return "clustervectorpipeline"
	}
	return "vectorpipeline"
}

// BeValid matcher for pipeline validity
type beValidMatcher struct{}

func (m *beValidMatcher) Match(actual interface{}) (success bool, err error) {
	pipeline, ok := actual.(*PipelineResource)
	if !ok {
		return false, fmt.Errorf("BeValid matcher expects a *PipelineResource")
	}

	result, err := pipeline.kubectl.GetWithJsonPath(pipeline.resourceType(), pipeline.name, ".status.configCheckResult")
	if err != nil {
		return false, err
	}

	return result == "true", nil
}

func (m *beValidMatcher) FailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s to be valid", pipeline.namespace, pipeline.name)
}

func (m *beValidMatcher) NegatedFailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s not to be valid", pipeline.namespace, pipeline.name)
}

// BeValid returns a matcher that checks if a pipeline is valid
func BeValid() types.GomegaMatcher {
	return &beValidMatcher{}
}

// HaveSplitModeEnabled matcher
type haveSplitModeEnabledMatcher struct{}

func (m *haveSplitModeEnabledMatcher) Match(actual interface{}) (success bool, err error) {
	pipeline, ok := actual.(*PipelineResource)
	if !ok {
		return false, fmt.Errorf("HaveSplitModeEnabled matcher expects a *PipelineResource")
	}

	result, err := pipeline.kubectl.GetWithJsonPath(pipeline.resourceType(), pipeline.name, ".status.splitMode.enabled")
	if err != nil {
		return false, err
	}

	return result == "true", nil
}

func (m *haveSplitModeEnabledMatcher) FailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s to have split mode enabled", pipeline.namespace, pipeline.name)
}

func (m *haveSplitModeEnabledMatcher) NegatedFailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s not to have split mode enabled", pipeline.namespace, pipeline.name)
}

// HaveSplitModeEnabled returns a matcher that checks if split mode is enabled
func HaveSplitModeEnabled() types.GomegaMatcher {
	return &haveSplitModeEnabledMatcher{}
}

// HaveRole matcher
type haveRoleMatcher struct {
	expectedRole string
}

func (m *haveRoleMatcher) Match(actual interface{}) (success bool, err error) {
	pipeline, ok := actual.(*PipelineResource)
	if !ok {
		return false, fmt.Errorf("HaveRole matcher expects a *PipelineResource")
	}

	result, err := pipeline.kubectl.GetWithJsonPath(pipeline.resourceType(), pipeline.name, ".status.role")
	if err != nil {
		return false, err
	}

	return result == m.expectedRole, nil
}

func (m *haveRoleMatcher) FailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s to have role %s", pipeline.namespace, pipeline.name, m.expectedRole)
}

func (m *haveRoleMatcher) NegatedFailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s not to have role %s", pipeline.namespace, pipeline.name, m.expectedRole)
}

// HaveRole returns a matcher that checks the pipeline role
func HaveRole(role string) types.GomegaMatcher {
	return &haveRoleMatcher{expectedRole: role}
}

// ServiceResource represents a service for matching
type ServiceResource struct {
	namespace string
	name      string
	kubectl   *kubectl.Client
}

// NewServiceResource creates a new service resource wrapper
func NewServiceResource(namespace, name string) *ServiceResource {
	return &ServiceResource{
		namespace: namespace,
		name:      name,
		kubectl:   kubectl.NewClient(namespace),
	}
}

// Exist matcher for service existence
type existMatcher struct{}

func (m *existMatcher) Match(actual interface{}) (success bool, err error) {
	service, ok := actual.(*ServiceResource)
	if !ok {
		return false, fmt.Errorf("Exist matcher expects a *ServiceResource")
	}

	_, err = service.kubectl.Get("service", service.name)
	return err == nil, nil
}

func (m *existMatcher) FailureMessage(actual interface{}) string {
	service := actual.(*ServiceResource)
	return fmt.Sprintf("Expected service %s/%s to exist", service.namespace, service.name)
}

func (m *existMatcher) NegatedFailureMessage(actual interface{}) string {
	service := actual.(*ServiceResource)
	return fmt.Sprintf("Expected service %s/%s not to exist", service.namespace, service.name)
}

// Exist returns a matcher that checks if a service exists
func Exist() types.GomegaMatcher {
	return &existMatcher{}
}

// HavePort matcher
type havePortMatcher struct {
	expectedPort string
}

func (m *havePortMatcher) Match(actual interface{}) (success bool, err error) {
	service, ok := actual.(*ServiceResource)
	if !ok {
		return false, fmt.Errorf("HavePort matcher expects a *ServiceResource")
	}

	port, err := service.kubectl.GetWithJsonPath("service", service.name, ".spec.ports[0].port")
	if err != nil {
		return false, err
	}

	return port == m.expectedPort, nil
}

func (m *havePortMatcher) FailureMessage(actual interface{}) string {
	service := actual.(*ServiceResource)
	return fmt.Sprintf("Expected service %s/%s to have port %s", service.namespace, service.name, m.expectedPort)
}

func (m *havePortMatcher) NegatedFailureMessage(actual interface{}) string {
	service := actual.(*ServiceResource)
	return fmt.Sprintf("Expected service %s/%s not to have port %s", service.namespace, service.name, m.expectedPort)
}

// HavePort returns a matcher that checks the service port
func HavePort(port string) types.GomegaMatcher {
	return &havePortMatcher{expectedPort: port}
}

// BeInvalid matcher for pipeline invalidity
type beInvalidMatcher struct{}

func (m *beInvalidMatcher) Match(actual interface{}) (success bool, err error) {
	pipeline, ok := actual.(*PipelineResource)
	if !ok {
		return false, fmt.Errorf("BeInvalid matcher expects a *PipelineResource")
	}

	result, err := pipeline.kubectl.GetWithJsonPath(pipeline.resourceType(), pipeline.name, ".status.configCheckResult")
	if err != nil {
		return false, err
	}

	return result == "false", nil
}

func (m *beInvalidMatcher) FailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s to be invalid", pipeline.namespace, pipeline.name)
}

func (m *beInvalidMatcher) NegatedFailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s not to be invalid", pipeline.namespace, pipeline.name)
}

// BeInvalid returns a matcher that checks if a pipeline is invalid
func BeInvalid() types.GomegaMatcher {
	return &beInvalidMatcher{}
}

// HaveErrorContaining matcher for error messages
type haveErrorContainingMatcher struct {
	expectedSubstring string
}

func (m *haveErrorContainingMatcher) Match(actual interface{}) (success bool, err error) {
	pipeline, ok := actual.(*PipelineResource)
	if !ok {
		return false, fmt.Errorf("HaveErrorContaining matcher expects a *PipelineResource")
	}

	reason, err := pipeline.kubectl.GetWithJsonPath(pipeline.resourceType(), pipeline.name, ".status.reason")
	if err != nil {
		return false, err
	}

	// Simple substring check (case-insensitive)
	lowerReason := strings.ToLower(reason)
	lowerExpected := strings.ToLower(m.expectedSubstring)

	return strings.Contains(lowerReason, lowerExpected), nil
}

func (m *haveErrorContainingMatcher) FailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s to have error containing '%s'",
		pipeline.namespace, pipeline.name, m.expectedSubstring)
}

func (m *haveErrorContainingMatcher) NegatedFailureMessage(actual interface{}) string {
	pipeline := actual.(*PipelineResource)
	return fmt.Sprintf("Expected pipeline %s/%s not to have error containing '%s'",
		pipeline.namespace, pipeline.name, m.expectedSubstring)
}

// HaveErrorContaining returns a matcher that checks if error message contains substring
func HaveErrorContaining(substring string) types.GomegaMatcher {
	return &haveErrorContainingMatcher{expectedSubstring: substring}
}
