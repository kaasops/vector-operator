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
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Storage handles filesystem operations for artifact collection
type Storage struct {
	baseDir string
	runDir  string
	maxSize int64
	runID   string
}

// NewStorage creates a new storage instance with specified configuration
func NewStorage(baseDir string, runID string, maxSize int64) (*Storage, error) {
	var runDir string

	// Check if baseDir already contains a run directory (e.g., from E2E_ARTIFACTS_DIR)
	// This prevents nested run-{timestamp}/run-{timestamp}/ structure
	if filepath.Base(baseDir) == "artifacts" && isRunDirectory(filepath.Dir(baseDir)) {
		// baseDir is already inside a run directory (e.g., test/e2e/results/run-{timestamp}/artifacts/)
		// Use it directly without creating another run-{runID} subdirectory
		runDir = baseDir
	} else {
		// Standard case: create run-{runID} subdirectory
		runDir = filepath.Join(baseDir, "run-"+runID)
	}

	// Create run directory
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run directory %s: %w", runDir, err)
	}

	return &Storage{
		baseDir: baseDir,
		runDir:  runDir,
		maxSize: maxSize,
		runID:   runID,
	}, nil
}

// isRunDirectory checks if a directory name matches the run-{timestamp} pattern
func isRunDirectory(path string) bool {
	base := filepath.Base(path)
	return len(base) > 4 && base[:4] == "run-"
}

// WriteFile writes content to a file within a test directory with size limits
// testDir: test-specific directory name (e.g., "test-normal-mode")
// category: subdirectory within test dir (e.g., "logs", "resources", "events")
// filename: name of the file to write
func (s *Storage) WriteFile(testDir, category, filename string, content []byte) error {
	// Check and enforce size limit
	if int64(len(content)) > s.maxSize {
		content = s.truncateContent(content, "size limit exceeded")
	}

	// Build full directory path
	var dir string
	if category != "" {
		dir = filepath.Join(s.runDir, testDir, category)
	} else {
		dir = filepath.Join(s.runDir, testDir)
	}

	// Create category directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write file atomically (write to temp, then rename)
	path := filepath.Join(dir, filename)
	tempPath := path + ".tmp"

	if err := os.WriteFile(tempPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write temp file %s: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file %s to %s: %w", tempPath, path, err)
	}

	return nil
}

// WriteFileInRunDir writes a file directly in the run directory (not test-specific)
// Used for run-level metadata
func (s *Storage) WriteFileInRunDir(filename string, content []byte) error {
	path := filepath.Join(s.runDir, filename)
	tempPath := path + ".tmp"

	if err := os.WriteFile(tempPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write temp file %s: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file %s to %s: %w", tempPath, path, err)
	}

	return nil
}

// WriteStream writes content from a reader to a file with size limits
// Useful for streaming command output without loading all into memory
func (s *Storage) WriteStream(testDir, category, filename string, reader io.Reader, maxLines int) error {
	// Build full directory path
	var dir string
	if category != "" {
		dir = filepath.Join(s.runDir, testDir, category)
	} else {
		dir = filepath.Join(s.runDir, testDir)
	}

	// Create category directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write to temp file
	path := filepath.Join(dir, filename)
	tempPath := path + ".tmp"

	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file %s: %w", tempPath, err)
	}
	defer tempFile.Close()

	// Copy with size limit
	written, err := io.CopyN(tempFile, reader, s.maxSize)
	if err != nil && err != io.EOF {
		// If we hit the limit, add truncation marker
		if written >= s.maxSize {
			truncationMarker := []byte("\n\n... [TRUNCATED - exceeds size limit] ...\n")
			_, _ = tempFile.Write(truncationMarker)
		}
	}

	tempFile.Close()

	// Rename to final path
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file %s to %s: %w", tempPath, path, err)
	}

	return nil
}

// GetRunDir returns the run directory path
func (s *Storage) GetRunDir() string {
	return s.runDir
}

// GetRunID returns the run ID
func (s *Storage) GetRunID() string {
	return s.runID
}

// truncateContent truncates content to fit within maxSize and adds a marker
func (s *Storage) truncateContent(content []byte, reason string) []byte {
	marker := []byte(fmt.Sprintf("\n\n... [TRUNCATED: %s - max %d bytes] ...\n", reason, s.maxSize))

	// If marker itself is too large, truncate it
	if int64(len(marker)) >= s.maxSize {
		return marker[:s.maxSize]
	}

	// Calculate how much content we can keep
	keepSize := s.maxSize - int64(len(marker))
	if keepSize < 0 {
		keepSize = 0
	}

	// Keep the end of the content (most recent logs are usually most relevant)
	// But also include first few bytes to show what file it is
	headerSize := int64(100)
	if headerSize > keepSize/2 {
		headerSize = keepSize / 2
	}

	var truncated []byte
	if headerSize > 0 && int64(len(content)) > headerSize {
		// Include header + marker + tail
		tailSize := keepSize - headerSize
		tailStart := int64(len(content)) - tailSize
		if tailStart < headerSize {
			tailStart = headerSize
		}

		truncated = append(truncated, content[:headerSize]...)
		truncated = append(truncated, []byte("\n... [CONTENT SKIPPED] ...\n")...)
		if tailStart < int64(len(content)) {
			truncated = append(truncated, content[tailStart:]...)
		}
	} else {
		// Just take what fits
		truncated = content[:keepSize]
	}

	return append(truncated, marker...)
}

// TruncateLogLines truncates log output to specified number of lines
// Takes the LAST N lines (most recent logs are most relevant for debugging)
func TruncateLogLines(content []byte, maxLines int) []byte {
	if maxLines <= 0 {
		return content
	}

	lines := []byte{}
	lineCount := 0
	newlineCount := 0

	// Count newlines from the end
	for i := len(content) - 1; i >= 0; i-- {
		if content[i] == '\n' {
			newlineCount++
			if newlineCount >= maxLines {
				// Found enough lines, this is our cut point
				lines = content[i+1:]
				lineCount = maxLines
				break
			}
		}
	}

	// If we didn't find enough newlines, return all content
	if lineCount == 0 {
		return content
	}

	// Skip leading empty lines and trim leading whitespace from first line
	start := 0
	for start < len(lines) {
		// Find end of current line
		end := start
		for end < len(lines) && lines[end] != '\n' {
			end++
		}

		// Check if line has any non-whitespace content
		lineContent := bytes.TrimSpace(lines[start:end])
		if len(lineContent) > 0 {
			// Found first non-empty line
			// Build result: trimmed first line + rest
			result := lineContent
			if end < len(lines) {
				// Append the rest (from \n onwards)
				result = append(result, lines[end:]...)
			}
			lines = result
			break
		}

		// Move to next line (skip the \n)
		start = end + 1
	}

	// Add truncation marker at the beginning
	marker := []byte(fmt.Sprintf("... [Showing last %d lines] ...\n", lineCount))
	return append(marker, lines...)
}

// CreateTestDir creates a directory for a specific test
func (s *Storage) CreateTestDir(testName string) (string, error) {
	// Sanitize test name for filesystem
	sanitized := sanitizeFilename(testName)
	testDir := filepath.Join(s.runDir, sanitized)

	if err := os.MkdirAll(testDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create test directory %s: %w", testDir, err)
	}

	return sanitized, nil
}

// sanitizeFilename removes characters that are problematic in filenames
func sanitizeFilename(name string) string {
	// Replace spaces and problematic characters with hyphens
	result := []byte(name)
	for i, c := range result {
		switch c {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', ' ':
			result[i] = '-'
		}
	}

	// Limit length to avoid filesystem issues
	const maxLength = 200
	if len(result) > maxLength {
		// Use a timestamp suffix to ensure uniqueness
		suffix := fmt.Sprintf("-%d", time.Now().Unix())
		cutPoint := maxLength - len(suffix)
		if cutPoint < 0 {
			cutPoint = 0
		}
		result = append(result[:cutPoint], []byte(suffix)...)
	}

	return string(result)
}
