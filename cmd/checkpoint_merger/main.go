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

// checkpoint_merger runs as an init container of the vector agent pod when
// checkpoint migration is enabled. Before vector starts it consolidates file
// checkpoints saved under the previous source names into the directories of
// the sources of the mounted config, so a config-optimization switch does not
// re-deliver the log files retained on the node.
//
// The binary is fail-open by design: an init container failure would block
// the agent pod, while the worst case of a skipped consolidation is a one-time
// duplicate delivery — the pre-migration status quo. It always exits 0.
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/kaasops/vector-operator/internal/buildinfo"
	"github.com/kaasops/vector-operator/internal/checkpoint"
)

func main() {
	dataDir := flag.String("data-dir", "/vector-data-dir", "vector data dir with per-source checkpoint directories")
	configPath := flag.String("config", "/etc/vector/agent.json", "path to the decoded vector config from the agent Secret")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	log.Info("build info", "version", buildinfo.Version)
	run(*dataDir, *configPath, log)
}

// run is fail-open: any problem (missing/unreadable/unparseable config, no
// sources, consolidation error) is logged and swallowed so the init container
// never blocks the agent pod. The worst outcome of a skip is a one-time re-read,
// the pre-migration status quo.
func run(dataDir, configPath string, log *slog.Logger) {
	configJSON, err := os.ReadFile(configPath)
	if err != nil {
		log.Error("cannot read config, skipping consolidation", "error", err)
		return
	}
	sources, err := checkpoint.KubernetesLogsSources(configJSON)
	if err != nil {
		log.Error("cannot parse config, skipping consolidation", "error", err)
		return
	}
	if len(sources) == 0 {
		log.Info("no kubernetes_logs sources in config, nothing to do")
		return
	}
	if err := checkpoint.Consolidate(dataDir, sources, log); err != nil {
		log.Error("consolidation skipped", "error", err)
	}
}
