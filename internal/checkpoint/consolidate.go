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

// Package checkpoint consolidates vector file checkpoints across source data
// directories. Vector keys checkpoints by a fingerprint of the file content
// (not by the source name), so when the operator renames kubernetes_logs
// sources (config optimization on/off), positions saved under the old source
// directories remain valid for the new ones and can be merged: this avoids
// a full re-read of the retained log files after the rename.
package checkpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// CheckpointFile is the checkpoint file name used by vector
// (lib/file-source/src/checkpointer.rs).
const CheckpointFile = "checkpoints.json"

// supportedVersion is the only checkpoint format version we understand
// (unchanged in vector since v0.20). Directories with any other version are
// left untouched: vector re-reads their files, which is the pre-migration
// behavior, not a corruption.
const supportedVersion = "1"

// entry mirrors one element of the "checkpoints" array. The fingerprint and
// any fields we do not know about are carried opaquely, so consolidation
// survives additive format changes within version 1.
type entry struct {
	raw         json.RawMessage
	fingerprint string
	position    uint64
}

type state struct {
	Version     string            `json:"version"`
	Checkpoints []json.RawMessage `json:"checkpoints"`
}

type entryFields struct {
	Fingerprint json.RawMessage `json:"fingerprint"`
	Position    uint64          `json:"position"`
}

// KubernetesLogsSources returns the names of kubernetes_logs sources of an
// agent config (the decoded content of the config Secret).
func KubernetesLogsSources(configJSON []byte) ([]string, error) {
	var cfg struct {
		Sources map[string]struct {
			Type string `json:"type"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	var names []string
	for name, src := range cfg.Sources {
		if src.Type == "kubernetes_logs" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// Consolidate merges checkpoints of all source directories under dataDir and
// materializes the result into the directories of the given sources:
//   - a source directory without a checkpoint file gets the full merged set
//     (a renamed source picks up the positions saved under the old names);
//   - an existing checkpoint file only gets its own entries advanced to the
//     merged positions, no foreign fingerprints are added (keeps per-source
//     files small; vector persists everything it loads).
//
// The operation is idempotent and never deletes anything. Directories with an
// unknown format are skipped both as a merge input and as a target.
func Consolidate(dataDir string, sources []string, log *slog.Logger) error {
	dirs, err := os.ReadDir(dataDir)
	if err != nil {
		return fmt.Errorf("read data dir: %w", err)
	}

	merged := make(map[string]entry)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		entries, err := readCheckpoints(filepath.Join(dataDir, d.Name(), CheckpointFile))
		if err != nil {
			if !os.IsNotExist(err) {
				log.Warn("skipping source dir", "dir", d.Name(), "error", err)
			}
			continue
		}
		for _, e := range entries {
			if cur, ok := merged[e.fingerprint]; !ok || e.position > cur.position {
				merged[e.fingerprint] = e
			}
		}
	}
	if len(merged) == 0 {
		log.Info("no checkpoints found, nothing to consolidate", "dataDir", dataDir)
		return nil
	}

	for _, src := range sources {
		dir := filepath.Join(dataDir, src)
		path := filepath.Join(dir, CheckpointFile)
		existing, err := readCheckpoints(path)
		switch {
		case os.IsNotExist(err):
			if err := os.MkdirAll(dir, 0750); err != nil {
				log.Warn("skipping target", "source", src, "error", err)
				continue
			}
			if err := writeCheckpoints(path, mapValues(merged)); err != nil {
				log.Warn("skipping target", "source", src, "error", err)
				continue
			}
			log.Info("seeded checkpoints for new source", "source", src, "checkpoints", len(merged))
		case err != nil:
			log.Warn("skipping target with unreadable checkpoints", "source", src, "error", err)
		default:
			advanced := 0
			for i, e := range existing {
				if m, ok := merged[e.fingerprint]; ok && m.position > e.position {
					existing[i] = m
					advanced++
				}
			}
			if advanced == 0 {
				continue
			}
			if err := writeCheckpoints(path, existing); err != nil {
				log.Warn("skipping target", "source", src, "error", err)
				continue
			}
			log.Info("advanced checkpoints", "source", src, "advanced", advanced)
		}
	}
	return nil
}

func readCheckpoints(path string) ([]entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if st.Version != supportedVersion {
		return nil, fmt.Errorf("unsupported checkpoint version %q in %s", st.Version, path)
	}
	entries := make([]entry, 0, len(st.Checkpoints))
	for _, raw := range st.Checkpoints {
		var f entryFields
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("parse checkpoint entry in %s: %w", path, err)
		}
		var key bytes.Buffer
		if err := json.Compact(&key, f.Fingerprint); err != nil {
			return nil, fmt.Errorf("parse fingerprint in %s: %w", path, err)
		}
		entries = append(entries, entry{raw: raw, fingerprint: key.String(), position: f.Position})
	}
	return entries, nil
}

func writeCheckpoints(path string, entries []entry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].fingerprint < entries[j].fingerprint })
	st := state{Version: supportedVersion, Checkpoints: make([]json.RawMessage, len(entries))}
	for i, e := range entries {
		st.Checkpoints[i] = e.raw
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	// atomic same-dir replace, mirrors vector's own tmp+rename write
	tmp := path + ".merging"
	if err := os.WriteFile(tmp, data, 0640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func mapValues(m map[string]entry) []entry {
	values := make([]entry, 0, len(m))
	for _, e := range m {
		values = append(values, e)
	}
	return values
}
