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

// Test labels for selective test execution
const (
	// Execution speed labels
	LabelSmoke      = "smoke"
	LabelFast       = "fast"
	LabelSlow       = "slow"
	LabelRegression = "regression"
	LabelStress     = "stress"
	LabelParallel   = "parallel"

	// Priority labels (P0 = critical, must always pass)
	LabelP0 = "p0"
	LabelP1 = "p1"
	LabelP2 = "p2"

	// Category labels
	LabelSecurity   = "security"
	LabelConstraint = "constraint"
)

// Resource naming suffixes
const (
	AggregatorSuffix = "-aggregator"
	AgentSuffix      = "-agent"
)

// Kubernetes labels
const (
	ComponentLabel      = "app.kubernetes.io/component"
	AggregatorComponent = "Aggregator"
	AgentComponent      = "Agent"
)
