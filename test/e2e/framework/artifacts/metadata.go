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
	"encoding/json"
	"fmt"
	"time"
)

// RunMetadata contains metadata about an entire test run
type RunMetadata struct {
	RunID        string            `json:"run_id"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time,omitempty"`
	TotalTests   int               `json:"total_tests"`
	FailedTests  int               `json:"failed_tests"`
	PassedTests  int               `json:"passed_tests"`
	Environment  map[string]string `json:"environment"`
	ArtifactsDir string            `json:"artifacts_dir"`
	// Git information for tracking test run version
	GitCommit   string `json:"git_commit,omitempty"`
	GitBranch   string `json:"git_branch,omitempty"`
	GitDirty    string `json:"git_dirty,omitempty"`   // "dirty", "staged", or empty if clean
	Description string `json:"description,omitempty"` // Optional user description
}

// TestMetadata contains metadata about a single test execution
type TestMetadata struct {
	Name           string        `json:"name"`
	Namespace      string        `json:"namespace"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration_ms"` // in milliseconds for JSON
	Failed         bool          `json:"failed"`
	FailureMessage string        `json:"failure_message,omitempty"`
	Labels         []string      `json:"labels"`

	// Test sequence tracking (for degradation analysis)
	TestSequenceNumber int           `json:"test_sequence_number"` // Which test in the run (1, 2, 3...)
	OperatorAge        time.Duration `json:"operator_age_seconds"` // How long operator has been running

	// Collected artifacts inventory
	Artifacts ArtifactInventory `json:"artifacts"`
}

// ArtifactInventory tracks what artifacts were collected
type ArtifactInventory struct {
	PodCount       int      `json:"pod_count"`
	LogFiles       []string `json:"log_files"`
	ResourceFiles  []string `json:"resource_files"`
	EventFiles     []string `json:"event_files"`
	TotalSizeBytes int64    `json:"total_size_bytes"`
	CollectionTime string   `json:"collection_time"` // Human-readable duration
}

// MetadataBuilder helps build and write metadata files
type MetadataBuilder struct {
	storage *Storage
}

// NewMetadataBuilder creates a new metadata builder
func NewMetadataBuilder(storage *Storage) *MetadataBuilder {
	return &MetadataBuilder{
		storage: storage,
	}
}

// WriteTestMetadata writes test metadata to JSON file
func (m *MetadataBuilder) WriteTestMetadata(meta TestMetadata, testDir string) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal test metadata: %w", err)
	}

	return m.storage.WriteFile(testDir, "", "metadata.json", data)
}

// WriteRunMetadata writes run metadata to JSON file
func (m *MetadataBuilder) WriteRunMetadata(meta RunMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal run metadata: %w", err)
	}

	return m.storage.WriteFileInRunDir("metadata.json", data)
}

// BuildTestMetadata creates TestMetadata from TestInfo
func BuildTestMetadata(info TestInfo, artifacts ArtifactInventory) TestMetadata {
	return TestMetadata{
		Name:               info.Name,
		Namespace:          info.Namespace,
		StartTime:          info.StartTime,
		EndTime:            info.EndTime,
		Duration:           info.Duration,
		Failed:             info.Failed,
		FailureMessage:     info.FailureMessage,
		Labels:             info.Labels,
		TestSequenceNumber: info.SequenceNumber,
		OperatorAge:        info.OperatorAge,
		Artifacts:          artifacts,
	}
}
