/*
Copyright 2022.

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

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kaasops/vector-operator/internal/utils/hash"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

const (
	optimizedSourcePrefix   = "optimizedSource"
	optimizedBucketerPrefix = "optimizedBucketer"
	optimizedRouterPrefix   = "optimizedRouter"

	// For small groups a single flat route transform is cheaper per event
	// than the two-level (bucketed) routing.
	flatRoutingThreshold = 16
	maxRouteBuckets      = 256

	// Larger groups are split to keep the generated namespace selector
	// (one name per namespace) well below any request-URI limits.
	maxNamespacesPerSource = 1000
)

// optimizeAgentSources collapses kubernetes_logs sources scoped to a single namespace
// (the form generated for namespaced VectorPipelines) into one source per group of
// identical settings. Per-pipeline event streams are restored with route transforms
// matching on the pod namespace, so inputs of downstream transforms and sinks are
// rewritten to the corresponding route output. Sources with any other namespace
// selector or with unique settings are left as is.
func optimizeAgentSources(cfg *VectorConfig) {
	groups := make(map[string][]*Source)
	for _, src := range cfg.Sources {
		if src.Type != KubernetesLogsType {
			continue
		}
		if _, ok := singleNamespaceOf(src); !ok {
			continue
		}
		sig := sourceSignature(src)
		groups[sig] = append(groups[sig], src)
	}

	for sig, sources := range groups {
		if len(sources) < 2 {
			continue
		}
		namespaces := sortedNamespacesOf(sources)
		gid := fmt.Sprintf("%08x", hash.Get([]byte(sig)))
		chunks := (len(namespaces) + maxNamespacesPerSource - 1) / maxNamespacesPerSource
		if chunks == 1 {
			collapseSourceGroup(cfg, gid, namespaces, sources)
		} else {
			nsChunk := make(map[string]int, len(namespaces))
			for i, ns := range namespaces {
				nsChunk[ns] = i / maxNamespacesPerSource
			}
			for c := 0; c < chunks; c++ {
				chunkNamespaces := namespaces[c*maxNamespacesPerSource : min((c+1)*maxNamespacesPerSource, len(namespaces))]
				chunkSources := make([]*Source, 0, len(chunkNamespaces))
				for _, src := range sources {
					ns, _ := singleNamespaceOf(src)
					if nsChunk[ns] == c {
						chunkSources = append(chunkSources, src)
					}
				}
				collapseSourceGroup(cfg, fmt.Sprintf("%s-%d", gid, c), chunkNamespaces, chunkSources)
			}
		}
		cfg.internal.optimizedSources += len(sources)
		cfg.internal.sourceGroups += chunks
	}
}

// collapseSourceGroup replaces the group sources with one source watching the union
// of their namespaces and routes its events back to the original per-source streams.
// Routes are keyed by namespace: consumers of all pipelines of a namespace share one
// route output, which keeps the number of conditions evaluated per event equal to
// the number of namespaces, not sources.
func collapseSourceGroup(cfg *VectorConfig, gid string, namespaces []string, sources []*Source) {
	collapsed := *sources[0]
	collapsed.Name = fmt.Sprintf("%s-%s", optimizedSourcePrefix, gid)
	collapsed.ExtraNamespaceLabelSelector = fmt.Sprintf("kubernetes.io/metadata.name in (%s)", strings.Join(namespaces, ","))

	nsOutput := make(map[string]string, len(namespaces))
	if len(namespaces) <= flatRoutingThreshold {
		router := fmt.Sprintf("%s-%s", optimizedRouterPrefix, gid)
		cfg.Transforms[router] = &Transform{
			Name:    router,
			Type:    RouteTransformType,
			Inputs:  []string{collapsed.Name},
			Options: map[string]interface{}{"route": namespaceRoutes(namespaces)},
		}
		for _, ns := range namespaces {
			nsOutput[ns] = fmt.Sprintf("%s.%s", router, ns)
		}
	} else {
		buckets := bucketCount(len(namespaces))
		bucketer := fmt.Sprintf("%s-%s", optimizedBucketerPrefix, gid)
		cfg.Transforms[bucketer] = &Transform{
			Name:   bucketer,
			Type:   RemapTransformType,
			Inputs: []string{collapsed.Name},
			Options: map[string]interface{}{
				// NB: the mod() function, not the % operator: after an expression
				// VRL parses `%` as the start of a metadata query.
				"source": fmt.Sprintf("%%bucket = mod(parse_int!(slice!(md5(string!(.kubernetes.pod_namespace)), 0, 2), base: 16), %d)", buckets),
			},
		}
		bucketNamespaces := make(map[int][]string)
		for _, ns := range namespaces {
			b := nsBucket(ns, buckets)
			bucketNamespaces[b] = append(bucketNamespaces[b], ns)
		}
		l1 := fmt.Sprintf("%s-%s-l1", optimizedRouterPrefix, gid)
		l1Routes := make(map[string]string, len(bucketNamespaces))
		for b, members := range bucketNamespaces {
			key := fmt.Sprintf("%d", b)
			l1Routes[key] = fmt.Sprintf("%%bucket == %d", b)
			l2 := fmt.Sprintf("%s-%s-%s", optimizedRouterPrefix, gid, key)
			cfg.Transforms[l2] = &Transform{
				Name:    l2,
				Type:    RouteTransformType,
				Inputs:  []string{fmt.Sprintf("%s.%s", l1, key)},
				Options: map[string]interface{}{"route": namespaceRoutes(members)},
			}
			for _, ns := range members {
				nsOutput[ns] = fmt.Sprintf("%s.%s", l2, ns)
			}
		}
		cfg.Transforms[l1] = &Transform{
			Name:    l1,
			Type:    RouteTransformType,
			Inputs:  []string{bucketer},
			Options: map[string]interface{}{"route": l1Routes},
		}
	}

	routeOutput := make(map[string]string, len(sources))
	for _, src := range sources {
		ns, _ := singleNamespaceOf(src)
		routeOutput[src.Name] = nsOutput[ns]
		delete(cfg.Sources, src.Name)
	}
	cfg.Sources[collapsed.Name] = &collapsed

	for _, t := range cfg.Transforms {
		rewriteInputs(t.Inputs, routeOutput)
	}
	for _, s := range cfg.Sinks {
		rewriteInputs(s.Inputs, routeOutput)
	}
}

// namespaceRoutes returns a route condition per namespace. An event is sent to
// every consumer of its namespace route, which preserves the fan-out semantics
// of the original per-pipeline sources.
func namespaceRoutes(namespaces []string) map[string]string {
	routes := make(map[string]string, len(namespaces))
	for _, ns := range namespaces {
		routes[ns] = fmt.Sprintf(".kubernetes.pod_namespace == %q", ns)
	}
	return routes
}

// singleNamespaceOf reports the namespace if the source is scoped to exactly one
// namespace by name, i.e. has the selector generated for namespaced VectorPipelines.
func singleNamespaceOf(src *Source) (string, bool) {
	ns := strings.TrimPrefix(src.ExtraNamespaceLabelSelector, k8s.NamespaceNameToLabel(""))
	if ns == "" || ns == src.ExtraNamespaceLabelSelector || strings.ContainsAny(ns, ",=!() ") {
		return "", false
	}
	return ns, true
}

func sortedNamespacesOf(sources []*Source) []string {
	seen := make(map[string]bool, len(sources))
	namespaces := make([]string, 0, len(sources))
	for _, src := range sources {
		ns, _ := singleNamespaceOf(src)
		if !seen[ns] {
			seen[ns] = true
			namespaces = append(namespaces, ns)
		}
	}
	sort.Strings(namespaces)
	return namespaces
}

// sourceSignature identifies sources which differ only in the watched namespace.
func sourceSignature(src *Source) string {
	b, _ := json.Marshal(struct {
		Type               string
		ExtraLabelSelector string
		ExtraFieldSelector string
		UseApiServerCache  bool
		Options            map[string]any
	}{src.Type, src.ExtraLabelSelector, src.ExtraFieldSelector, src.UseApiServerCache, src.Options})
	return string(b)
}

// bucketCount returns the number of routing buckets: the power of two near
// sqrt(n), which minimizes route conditions evaluated per event (n/buckets + buckets).
func bucketCount(n int) int {
	buckets := 1
	for buckets*buckets < n && buckets < maxRouteBuckets {
		buckets *= 2
	}
	if half := buckets / 2; half > 0 && half+(n+half-1)/half <= buckets+(n+buckets-1)/buckets {
		return half
	}
	return buckets
}

// nsBucket mirrors the bucket expression generated for the remap transform:
// the first byte of the md5 hex digest modulo the bucket count.
func nsBucket(ns string, buckets int) int {
	return int(md5.Sum([]byte(ns))[0]) % buckets
}

func rewriteInputs(inputs []string, routeOutput map[string]string) {
	for i, input := range inputs {
		if out, ok := routeOutput[input]; ok {
			inputs[i] = out
		}
	}
}
