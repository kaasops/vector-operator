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
		collapseSourceGroup(cfg, sig, sources)
	}
}

// collapseSourceGroup replaces the group sources with one source watching the union
// of their namespaces and routes its events back to the original per-source streams.
func collapseSourceGroup(cfg *VectorConfig, sig string, sources []*Source) {
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })

	namespaces := make([]string, 0, len(sources))
	seen := make(map[string]bool)
	for _, src := range sources {
		ns, _ := singleNamespaceOf(src)
		if !seen[ns] {
			seen[ns] = true
			namespaces = append(namespaces, ns)
		}
	}
	sort.Strings(namespaces)

	gid := fmt.Sprintf("%08x", hash.Get([]byte(sig)))
	collapsed := *sources[0]
	collapsed.Name = fmt.Sprintf("%s-%s", optimizedSourcePrefix, gid)
	collapsed.ExtraNamespaceLabelSelector = fmt.Sprintf("kubernetes.io/metadata.name in (%s)", strings.Join(namespaces, ","))

	routerInput := collapsed.Name
	routeOutput := make(map[string]string, len(sources))
	if len(namespaces) <= flatRoutingThreshold {
		router := fmt.Sprintf("%s-%s", optimizedRouterPrefix, gid)
		cfg.Transforms[router] = &Transform{
			Name:    router,
			Type:    RouteTransformType,
			Inputs:  []string{routerInput},
			Options: map[string]interface{}{"route": namespaceRoutes(sources)},
		}
		for _, src := range sources {
			routeOutput[src.Name] = fmt.Sprintf("%s.%s", router, src.Name)
		}
	} else {
		buckets := bucketCount(len(namespaces))
		bucketer := fmt.Sprintf("%s-%s", optimizedBucketerPrefix, gid)
		cfg.Transforms[bucketer] = &Transform{
			Name:   bucketer,
			Type:   RemapTransformType,
			Inputs: []string{routerInput},
			Options: map[string]interface{}{
				"source": fmt.Sprintf("%%bucket = parse_int!(slice!(md5(string!(.kubernetes.pod_namespace)), 0, 2), base: 16) %% %d", buckets),
			},
		}
		l1 := fmt.Sprintf("%s-%s-l1", optimizedRouterPrefix, gid)
		l1Routes := make(map[string]string)
		bucketSources := make(map[int][]*Source)
		for _, src := range sources {
			ns, _ := singleNamespaceOf(src)
			b := nsBucket(ns, buckets)
			bucketSources[b] = append(bucketSources[b], src)
		}
		for b, members := range bucketSources {
			key := fmt.Sprintf("%d", b)
			l1Routes[key] = fmt.Sprintf("%%bucket == %d", b)
			l2 := fmt.Sprintf("%s-%s-%s", optimizedRouterPrefix, gid, key)
			cfg.Transforms[l2] = &Transform{
				Name:    l2,
				Type:    RouteTransformType,
				Inputs:  []string{fmt.Sprintf("%s.%s", l1, key)},
				Options: map[string]interface{}{"route": namespaceRoutes(members)},
			}
			for _, src := range members {
				routeOutput[src.Name] = fmt.Sprintf("%s.%s", l2, src.Name)
			}
		}
		cfg.Transforms[l1] = &Transform{
			Name:    l1,
			Type:    RouteTransformType,
			Inputs:  []string{bucketer},
			Options: map[string]interface{}{"route": l1Routes},
		}
	}

	for _, src := range sources {
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

// namespaceRoutes returns a route condition per source matching its namespace.
// An event matching several routes is sent to all of them, which preserves the
// fan-out semantics of the original per-pipeline sources.
func namespaceRoutes(sources []*Source) map[string]string {
	routes := make(map[string]string, len(sources))
	for _, src := range sources {
		ns, _ := singleNamespaceOf(src)
		routes[src.Name] = fmt.Sprintf(".kubernetes.pod_namespace == %q", ns)
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
