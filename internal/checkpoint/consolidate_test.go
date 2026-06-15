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

package checkpoint

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// checkpoint file as vector v0.48 writes it
const vectorFormat = `{"version":"1","checkpoints":[
  {"fingerprint":{"first_lines_checksum":11111},"position":100,"modified":"2026-06-12T10:00:00Z"},
  {"fingerprint":{"first_lines_checksum":22222},"position":200,"modified":"2026-06-12T10:00:00Z"}
]}`

func writeFile(t *testing.T, dataDir, source, content string) {
	t.Helper()
	dir := filepath.Join(dataDir, source)
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, CheckpointFile), []byte(content), 0640); err != nil {
		t.Fatal(err)
	}
}

func readState(t *testing.T, dataDir, source string) map[string]uint64 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dataDir, source, CheckpointFile))
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Version     string `json:"version"`
		Checkpoints []struct {
			Fingerprint struct {
				FirstLinesChecksum uint64 `json:"first_lines_checksum"`
			} `json:"fingerprint"`
			Position uint64 `json:"position"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if st.Version != "1" {
		t.Fatalf("version changed to %q", st.Version)
	}
	res := map[string]uint64{}
	for _, c := range st.Checkpoints {
		res[itoa(c.Fingerprint.FirstLinesChecksum)] = c.Position
	}
	return res
}

func itoa(v uint64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func TestKubernetesLogsSources(t *testing.T) {
	cfg := []byte(`{"sources":{
		"a":{"type":"kubernetes_logs"},
		"b":{"type":"journald"},
		"optimizedSource-1234":{"type":"kubernetes_logs"}}}`)
	got, err := KubernetesLogsSources(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "optimizedSource-1234" {
		t.Fatalf("got %v", got)
	}
}

func TestSeedsNewSourceWithFullUnion(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "old-a", vectorFormat)
	writeFile(t, dataDir, "old-b", `{"version":"1","checkpoints":[
	  {"fingerprint":{"first_lines_checksum":11111},"position":150,"modified":"2026-06-12T11:00:00Z"},
	  {"fingerprint":{"first_lines_checksum":33333},"position":300,"modified":"2026-06-12T11:00:00Z"}]}`)

	if err := Consolidate(dataDir, []string{"optimizedSource-new"}, testLog()); err != nil {
		t.Fatal(err)
	}

	got := readState(t, dataDir, "optimizedSource-new")
	want := map[string]uint64{"11111": 150, "22222": 200, "33333": 300}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("fingerprint %s: got position %d, want %d", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d checkpoints, want %d", len(got), len(want))
	}
}

func TestUnionAdvancesAndAddsIntoExisting(t *testing.T) {
	dataDir := t.TempDir()
	// rollback / re-enable case: target dir pre-exists with stale positions,
	// another dir is ahead and holds a fingerprint the target lacks. The target
	// must both advance its own entries and pick up the missing fingerprint,
	// otherwise the file behind that fingerprint is re-read from the start.
	writeFile(t, dataDir, "target", vectorFormat)
	writeFile(t, dataDir, "other", `{"version":"1","checkpoints":[
	  {"fingerprint":{"first_lines_checksum":11111},"position":999,"modified":"2026-06-12T12:00:00Z"},
	  {"fingerprint":{"first_lines_checksum":44444},"position":400,"modified":"2026-06-12T12:00:00Z"}]}`)

	if err := Consolidate(dataDir, []string{"target"}, testLog()); err != nil {
		t.Fatal(err)
	}

	got := readState(t, dataDir, "target")
	if got["11111"] != 999 {
		t.Errorf("position not advanced: got %d, want 999", got["11111"])
	}
	if got["22222"] != 200 {
		t.Errorf("untouched entry changed: got %d, want 200", got["22222"])
	}
	if got["44444"] != 400 {
		t.Errorf("missing fingerprint not added: got %d, want 400 — this is the re-read bug", got["44444"])
	}
}

func TestExistingOptimizedDirGetsNewFingerprints(t *testing.T) {
	dataDir := t.TempDir()
	// the bug found on the stand: the optimized source dir survives a previous
	// run with stale entries; new files (their checkpoints under legacy dirs)
	// must be seeded into it on re-enable, not ignored.
	writeFile(t, dataDir, "optimizedSource-abcd", `{"version":"1","checkpoints":[
	  {"fingerprint":{"first_lines_checksum":99999},"position":10,"modified":"2026-06-10T00:00:00Z"}]}`)
	writeFile(t, dataDir, "vp-ns-0001-pipeline-logs", vectorFormat)

	if err := Consolidate(dataDir, []string{"optimizedSource-abcd"}, testLog()); err != nil {
		t.Fatal(err)
	}

	got := readState(t, dataDir, "optimizedSource-abcd")
	for fp, want := range map[string]uint64{"11111": 100, "22222": 200, "99999": 10} {
		if got[fp] != want {
			t.Errorf("fingerprint %s: got %d, want %d", fp, got[fp], want)
		}
	}
}

func TestUnknownVersionSkipped(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "future", `{"version":"2","checkpoints":[
	  {"fingerprint":{"first_lines_checksum":55555},"position":500}]}`)
	writeFile(t, dataDir, "old", vectorFormat)

	if err := Consolidate(dataDir, []string{"new", "future"}, testLog()); err != nil {
		t.Fatal(err)
	}

	// v2 entries not merged in
	if _, ok := readState(t, dataDir, "new")["55555"]; ok {
		t.Error("entry from unsupported version merged")
	}
	// v2 file not rewritten
	data, _ := os.ReadFile(filepath.Join(dataDir, "future", CheckpointFile))
	var st state
	if err := json.Unmarshal(data, &st); err != nil || st.Version != "2" {
		t.Error("unsupported version file was modified")
	}
}

func TestPreservesUnknownEntryFields(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "old", `{"version":"1","checkpoints":[
	  {"fingerprint":{"first_lines_checksum":11111},"position":100,"modified":"2026-06-12T10:00:00Z","future_field":{"x":1}}]}`)

	if err := Consolidate(dataDir, []string{"new"}, testLog()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "new", CheckpointFile))
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Checkpoints []map[string]any `json:"checkpoints"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Checkpoints[0]["future_field"]; !ok {
		t.Error("unknown entry field dropped on rewrite")
	}
	if _, ok := st.Checkpoints[0]["modified"]; !ok {
		t.Error("modified field dropped on rewrite")
	}
}

func TestIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "old-a", vectorFormat)

	if err := Consolidate(dataDir, []string{"new"}, testLog()); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dataDir, "new", CheckpointFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := Consolidate(dataDir, []string{"new"}, testLog()); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dataDir, "new", CheckpointFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("second run changed the result")
	}
}

func TestCorruptSourceDirSkippedOthersConsolidate(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "corrupt", "}{ not json at all")
	writeFile(t, dataDir, "good", vectorFormat)

	if err := Consolidate(dataDir, []string{"new"}, testLog()); err != nil {
		t.Fatal(err)
	}
	// the valid dir's checkpoints still seed the target despite the corrupt sibling
	got := readState(t, dataDir, "new")
	if got["11111"] != 100 || got["22222"] != 200 {
		t.Errorf("valid checkpoints not consolidated past corrupt dir: %v", got)
	}
}

func TestCorruptTargetDirSkippedNotAborted(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "good", vectorFormat)
	writeFile(t, dataDir, "corrupt-target", "garbage")
	writeFile(t, dataDir, "clean-target", `{"version":"1","checkpoints":[]}`)

	// a corrupt target must not abort the run for other targets
	if err := Consolidate(dataDir, []string{"corrupt-target", "clean-target"}, testLog()); err != nil {
		t.Fatal(err)
	}
	got := readState(t, dataDir, "clean-target")
	if got["11111"] != 100 || got["22222"] != 200 {
		t.Errorf("clean target not seeded when a sibling target was corrupt: %v", got)
	}
}

func TestFingerprintVariantsDoNotCollide(t *testing.T) {
	dataDir := t.TempDir()
	// distinct vector FileFingerprint variants that share the numeric payload must
	// be keyed apart (external tag), and a dev_inode array entry must round-trip.
	writeFile(t, dataDir, "mixed", `{"version":"1","checkpoints":[
	  {"fingerprint":{"checksum":5},"position":10,"modified":"2026-06-12T10:00:00Z"},
	  {"fingerprint":{"first_lines_checksum":5},"position":20,"modified":"2026-06-12T10:00:00Z"},
	  {"fingerprint":{"dev_inode":[1,2]},"position":30,"modified":"2026-06-12T10:00:00Z"}]}`)

	if err := Consolidate(dataDir, []string{"new"}, testLog()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "new", CheckpointFile))
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Checkpoints []struct {
			Fingerprint map[string]any `json:"fingerprint"`
			Position    uint64         `json:"position"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Checkpoints) != 3 {
		t.Fatalf("variants collided: got %d entries, want 3", len(st.Checkpoints))
	}
	var sawDevInode bool
	for _, c := range st.Checkpoints {
		if _, ok := c.Fingerprint["dev_inode"]; ok {
			sawDevInode = true
			if c.Position != 30 {
				t.Errorf("dev_inode position not preserved: %d", c.Position)
			}
		}
	}
	if !sawDevInode {
		t.Error("dev_inode variant lost on round-trip")
	}
}

func TestEmptyDataDir(t *testing.T) {
	dataDir := t.TempDir()
	if err := Consolidate(dataDir, []string{"new"}, testLog()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "new")); !os.IsNotExist(err) {
		t.Error("target dir created with no checkpoints to seed")
	}
}
