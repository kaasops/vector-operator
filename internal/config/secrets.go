package config

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/pipeline"
)

const (
	SecretsMountPath     = "/etc/vector/secrets"
	SecretsBackendName   = "k8s"
	secretTypeKubernetes = "kubernetes_secret"
)

type secretRef struct {
	Alias string
	Key   string
}

var secretRefRegex = regexp.MustCompile(`SECRET\[([A-Za-z0-9_]+)\.([A-Za-z0-9_./\-]+)\]`)
var keyCharsetRegex = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// SecretKeyTooLongError marks a SECRET[alias.key] reference whose generated flat key
// (flatKey: namespace + pipeline + alias + source key, joined and sanitized) exceeds
// Kubernetes' Secret/ConfigMap key length limit - validation.DNS1123SubdomainMaxLength
// (253), the same bound k8s.io/apimachinery's IsConfigMapKey enforces, reused here
// directly so this check can never drift from what the API server actually accepts.
//
// A source key can be short and valid on its own and only become invalid once the
// operator prepends its prefix, so this is checked before any API call rather than left
// to surface as a raw "must be no more than 253 characters" error from the aggregated
// Secret write - a write that bundles every pipeline on the workload and so implicates
// whichever build ran last, not the pipeline that owns the long key.
//
// Error() does not repeat the pipeline name: processPipelineSecrets wraps every error
// scanAndRewriteSecretRefs returns in "pipeline %s: %w". Pipeline stays as a field for
// callers acting on the failure structurally.
type SecretKeyTooLongError struct {
	Pipeline string // namespace/name (or bare name for a cluster-scoped pipeline)
	Alias    string
	Key      string
	FlatKey  string
}

func (e *SecretKeyTooLongError) Error() string {
	return fmt.Sprintf(
		"secret reference %q: generated key %q is %d characters, over the %d character Kubernetes Secret key limit; shorten the pipeline name, the secret alias, or the source key",
		e.Alias+"."+e.Key, e.FlatKey, len(e.FlatKey), validation.DNS1123SubdomainMaxLength,
	)
}

func scanAndRewriteSecretRefs(options map[string]any, ns, name string, declared map[string]v1alpha1.PipelineSecretBackend) ([]secretRef, error) {
	// Named pid, not pipelineID, to avoid shadowing the package-level
	// pipelineID(pipeline.Pipeline) function below: this one is built straight from
	// the raw ns/name strings already in scope, since there is no pipeline.Pipeline
	// value here to call that function on.
	pid := name
	if ns != "" {
		pid = ns + "/" + name
	}

	var refs []secretRef
	var walk func(v any) (any, error)
	walk = func(v any) (any, error) {
		switch val := v.(type) {
		case string:
			var walkErr error
			out := secretRefRegex.ReplaceAllStringFunc(val, func(m string) string {
				sub := secretRefRegex.FindStringSubmatch(m)
				alias, key := sub[1], sub[2]
				if _, ok := declared[alias]; !ok {
					walkErr = fmt.Errorf("secret reference %q: backend %q is not declared in spec.secret", m, alias)
					return m
				}
				if !keyCharsetRegex.MatchString(key) {
					walkErr = fmt.Errorf("secret reference %q: key %q must match ^[A-Za-z0-9_.-]+$", m, key)
					return m
				}
				flat := flatKey(ns, name, alias, key)
				if len(flat) > validation.DNS1123SubdomainMaxLength {
					walkErr = &SecretKeyTooLongError{Pipeline: pid, Alias: alias, Key: key, FlatKey: flat}
					return m
				}
				refs = append(refs, secretRef{Alias: alias, Key: key})
				return "SECRET[" + SecretsBackendName + "." + flat + "]"
			})
			return out, walkErr
		case map[string]any:
			for k, item := range val {
				nv, err := walk(item)
				if err != nil {
					return nil, err
				}
				val[k] = nv
			}
			return val, nil
		case []any:
			for i, item := range val {
				nv, err := walk(item)
				if err != nil {
					return nil, err
				}
				val[i] = nv
			}
			return val, nil
		default:
			return v, nil
		}
	}
	_, err := walk(options)
	return refs, err
}

func flatKey(ns, name, alias, key string) string {
	base := generateName(ns, name)
	joined := base + "-" + alias + "-" + key
	return strings.ReplaceAll(joined, "-", "_")
}

// UsedSecretBackends returns the set of spec.secret aliases actually referenced by a
// SECRET[alias.key] placeholder anywhere in the pipeline's source/transform/sink
// options. It walks a freshly unmarshaled copy of the spec without validating or
// rewriting anything: undeclared aliases and malformed keys are left for config build
// (scanAndRewriteSecretRefs) to report, exactly as before. Callers use this to scope
// per-pipeline secret resolution and watch registration to backends that a reference
// actually uses, so a declared-but-unused backend is never read, hashed, or watched.
func UsedSecretBackends(p pipeline.Pipeline) (map[string]struct{}, error) {
	declared := p.GetSpec().Secret
	if len(declared) == 0 {
		return nil, nil
	}
	cfg := &PipelineConfig{}
	if err := UnmarshalJson(p.GetSpec(), cfg); err != nil {
		return nil, fmt.Errorf("pipeline %s: %w", pipelineID(p), err)
	}
	var comps []map[string]any
	for _, v := range cfg.Sources {
		comps = append(comps, v.Options)
	}
	for _, v := range cfg.Transforms {
		comps = append(comps, v.Options)
	}
	for _, v := range cfg.Sinks {
		comps = append(comps, v.Options)
	}

	used := make(map[string]struct{})
	var walk func(v any)
	walk = func(v any) {
		switch val := v.(type) {
		case string:
			for _, sub := range secretRefRegex.FindAllStringSubmatch(val, -1) {
				if _, ok := declared[sub[1]]; ok {
					used[sub[1]] = struct{}{}
				}
			}
		case map[string]any:
			for _, item := range val {
				walk(item)
			}
		case []any:
			for _, item := range val {
				walk(item)
			}
		}
	}
	for _, c := range comps {
		walk(c)
	}
	if len(used) == 0 {
		return nil, nil
	}
	return used, nil
}

// pipelineID is the identifier used to attribute a secret reference (and a collision)
// to a pipeline in error/status messages: "namespace/name" for a namespaced
// VectorPipeline, or just "name" for a cluster-scoped ClusterVectorPipeline.
func pipelineID(p pipeline.Pipeline) string {
	ns, name := p.GetNamespace(), p.GetName()
	if ns != "" {
		return ns + "/" + name
	}
	return name
}

// SecretResolveError marks a failure to fetch a Secret referenced by a pipeline's
// SECRET[] placeholder during config build (resolvePendingSecrets' getter call), as
// opposed to any other Build*Config failure (spec/parse problems, a referenced key
// missing from the Secret's Data). It signals the same class of failure as
// resolveRelatedSecrets' resolve error in the controller package - possibly transient
// (the API server hiccuped, or the Secret briefly doesn't exist) - so the pipeline
// controller can retry it on a timer via errors.As instead of treating it like a
// permanent spec problem.
type SecretResolveError struct {
	err error
}

func (e *SecretResolveError) Error() string { return e.err.Error() }
func (e *SecretResolveError) Unwrap() error { return e.err }

// SecretValueUnsafeError marks a SECRET[alias.key] reference whose CURRENTLY STORED
// value cannot be safely substituted into vector's generated config - see
// secretValueSafeForJSONText for exactly what that requires and why.
//
// It is a permanent failure of the CURRENT value: unlike SecretResolveError it must not
// be retried on a timer, since nothing changes until the Secret itself does. Rotation to
// a safe value is what resolves it, and the Secret watch (or the scoped rotation poll -
// see docs/secrets.md) already wakes a reconcile for exactly that.
//
// Error() names only the Secret and key - never the value, the offending byte, or its
// position. This text lands in `.status.reason`, readable by anyone who can read the
// pipeline, so it must not become a channel for leaking the content.
type SecretValueUnsafeError struct {
	SecretNamespace string
	SecretName      string
	Key             string
}

func (e *SecretValueUnsafeError) Error() string {
	return fmt.Sprintf(
		"secret %s/%s: key %q holds a value that cannot be safely substituted into the generated config (it must be valid UTF-8 with no double quote, backslash, or control byte); rotate it to a value that satisfies this",
		e.SecretNamespace, e.SecretName, e.Key,
	)
}

// secretValueSafeForJSONText reports whether value can be substituted byte for byte into
// vector's generated config. That config is published as JSON TEXT and vector interpolates
// a SECRET[] reference into it BEFORE parsing (the operator writes the Secret's raw bytes
// as-is into the mounted directory backend - see resolvePendingSecrets), so a value has to
// survive the substitution intact:
//
//   - Valid UTF-8: vector's directory backend reads the file with read_to_string
//     (src/secrets/directory.rs), so invalid bytes would fail vector-side instead of
//     failing here, attributed.
//   - No `"` (0x22): closes the surrounding JSON string early, corrupting every field
//     after it.
//   - No `\` (0x5c): next to a character JSON does not treat as an escape it breaks the
//     parse, but next to one it does (`\n`, `\t`, `\uXXXX`, ...) it forms a VALID escape
//     that decodes to different bytes than the Secret holds - which `vector validate` and
//     the live parse both accept. That silent half is why this guard exists at all rather
//     than leaving the case to configcheck.
//   - No control byte (0x00-0x1F): JSON forbids them unescaped (RFC 8259), and this
//     operator does not escape values on the way in.
//
// Anything else valid UTF-8 can express passes untouched - this guard keeps the
// substitution safe, it does not restrict what a credential may contain beyond that. An
// empty value is safe here; whether the directory backend accepts one is vector's own
// contract, not this guard's to enforce.
func secretValueSafeForJSONText(value []byte) bool {
	if len(value) == 0 {
		return true
	}
	if !utf8.Valid(value) {
		return false
	}
	for _, b := range value {
		if b == '"' || b == '\\' || b < 0x20 {
			return false
		}
	}
	return true
}

// pendingSecretRef ties a SECRET[k8s.<flat>] reference already rewritten into a
// pipeline's config to the backend it must be resolved from.
type pendingSecretRef struct {
	flat       string // matches the SECRET[k8s.<flat>] rewritten into the config
	resolveNS  string // namespace to fetch the Secret from
	secretName string
	key        string // key inside the Secret's Data
	pipeline   string // namespace/name of the pipeline that produced this ref, for error messages
}

// processPipelineSecrets validates the pipeline's declared secret backends (namespace
// is forbidden on VectorPipeline, required on ClusterVectorPipeline), scans/rewrites
// SECRET[] references in each of the pipeline's component option maps, and queues the
// resolved references onto pending. comps may safely include nil/empty maps.
func processPipelineSecrets(p pipeline.Pipeline, getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error), comps []map[string]any, pending *[]pendingSecretRef) error {
	declared := p.GetSpec().Secret
	_, isVP := p.(*v1alpha1.VectorPipeline)

	for alias, backend := range declared {
		if isVP && backend.Namespace != "" {
			return fmt.Errorf("pipeline %s: secret backend %q: namespace is not allowed in VectorPipeline", p.GetName(), alias)
		}
		if !isVP && backend.Namespace == "" {
			return fmt.Errorf("pipeline %s: secret backend %q: namespace is required", p.GetName(), alias)
		}
	}
	if len(declared) > 0 && getter == nil {
		return fmt.Errorf("pipeline %s: secrets are not supported in this context", p.GetName())
	}

	ns, name := p.GetNamespace(), p.GetName()
	id := pipelineID(p)
	for _, opts := range comps {
		if len(opts) == 0 {
			continue
		}
		refs, err := scanAndRewriteSecretRefs(opts, ns, name, declared)
		if err != nil {
			return fmt.Errorf("pipeline %s: %w", name, err)
		}
		for _, ref := range refs {
			backend := declared[ref.Alias]
			resolveNS := ns
			if !isVP {
				resolveNS = backend.Namespace
			}
			*pending = append(*pending, pendingSecretRef{
				flat:       flatKey(ns, name, ref.Alias, ref.Key),
				resolveNS:  resolveNS,
				secretName: backend.Name,
				key:        ref.Key,
				pipeline:   id,
			})
		}
	}
	return nil
}

// resolvePendingSecrets fetches every pending secret reference (memoized per
// namespace/name so a Secret referenced multiple times is fetched once), and
// materializes cfg.Secret + cfg.internal.secretAssets. A no-op when pending is empty,
// leaving cfg exactly as it would have been without secrets support (zero churn).
func resolvePendingSecrets(ctx context.Context, cfg *VectorConfig, getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error), pending []pendingSecretRef) error {
	if len(pending) == 0 {
		return nil
	}

	cache := make(map[string]*corev1.Secret)
	data := make(map[string][]byte, len(pending))
	origins := make(map[string]pendingSecretRef, len(pending))
	for _, ref := range pending {
		if origin, seen := origins[ref.flat]; seen {
			if origin.resolveNS != ref.resolveNS || origin.secretName != ref.secretName || origin.key != ref.key {
				return fmt.Errorf("secret flat key %q collides between pipeline %s and pipeline %s: their namespace/name pair is only distinguishable by a hyphen; rename one of them", ref.flat, origin.pipeline, ref.pipeline)
			}
		} else {
			origins[ref.flat] = ref
		}

		cacheKey := ref.resolveNS + "/" + ref.secretName
		secret, ok := cache[cacheKey]
		if !ok {
			var err error
			secret, err = getter(ctx, ref.resolveNS, ref.secretName)
			if err != nil {
				return &SecretResolveError{err: fmt.Errorf("failed to get secret %s/%s: %w", ref.resolveNS, ref.secretName, err)}
			}
			cache[cacheKey] = secret
		}
		val, ok := secret.Data[ref.key]
		if !ok {
			return fmt.Errorf("secret %s/%s: key %q not found", ref.resolveNS, ref.secretName, ref.key)
		}
		if !secretValueSafeForJSONText(val) {
			return &SecretValueUnsafeError{SecretNamespace: ref.resolveNS, SecretName: ref.secretName, Key: ref.key}
		}
		data[ref.flat] = val
	}

	cfg.Secret = map[string]any{
		SecretsBackendName: map[string]any{
			"type": "directory",
			"path": SecretsMountPath,
		},
	}
	cfg.internal.secretAssets = data
	return nil
}

// SecretCollision reports that Victim's SECRET[] references collided, on flat key
// FlatKey, with the pipeline identified by Survivor - discovered by
// DetectSecretCollisions scanning the merged pipeline list a workload build is about
// to use. Victim must be excluded from that build; Survivor keeps its references
// untouched.
type SecretCollision struct {
	Victim   pipeline.Pipeline
	FlatKey  string
	Survivor string
}

// gatherPendingSecretsByPipeline scans every pipeline's spec for SECRET[] references
// (processPipelineSecrets), grouping the resulting pendingSecretRef list by the
// pipeline that produced it and ordering the pipeline IDs oldest-CreationTimestamp
// first (ties broken by ascending pipeline ID for determinism). Both
// DetectSecretCollisions and DetectSecretSizeOverflow attribute a shared, exhausted
// resource (a flat key, the aggregated Secret's byte budget) using this exact
// traversal order: whichever pipeline existed first keeps the resource, younger
// pipelines are excluded - so they share this one gathering/ordering pass instead of
// risking the two attribution policies drifting out of sync with each other.
func gatherPendingSecretsByPipeline(getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error), pipelines []pipeline.Pipeline) (byID map[string]pipeline.Pipeline, byPipeline map[string][]pendingSecretRef, orderedIDs []string, err error) {
	byID = make(map[string]pipeline.Pipeline, len(pipelines))
	var pending []pendingSecretRef
	for _, p := range pipelines {
		cfg := &PipelineConfig{}
		if err := UnmarshalJson(p.GetSpec(), cfg); err != nil {
			return nil, nil, nil, fmt.Errorf("pipeline %s: %w", pipelineID(p), err)
		}
		var comps []map[string]any
		for _, v := range cfg.Sources {
			comps = append(comps, v.Options)
		}
		for _, v := range cfg.Transforms {
			comps = append(comps, v.Options)
		}
		for _, v := range cfg.Sinks {
			comps = append(comps, v.Options)
		}
		if err := processPipelineSecrets(p, getter, comps, &pending); err != nil {
			return nil, nil, nil, err
		}
		byID[pipelineID(p)] = p
	}

	byPipeline = make(map[string][]pendingSecretRef)
	for _, ref := range pending {
		byPipeline[ref.pipeline] = append(byPipeline[ref.pipeline], ref)
	}

	orderedIDs = make([]string, 0, len(byPipeline))
	for id := range byPipeline {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Slice(orderedIDs, func(i, j int) bool {
		ti := byID[orderedIDs[i]].GetCreationTimestamp()
		tj := byID[orderedIDs[j]].GetCreationTimestamp()
		if !ti.Equal(&tj) {
			return ti.Before(&tj)
		}
		return orderedIDs[i] < orderedIDs[j]
	})
	return byID, byPipeline, orderedIDs, nil
}

// DetectSecretCollisions is a read-only pre-pass over the same pipeline list a
// workload build (BuildAgentConfig/BuildAggregatorConfig) is about to receive,
// looking for flat-key collisions that only become visible once pipelines are merged -
// processPipelineSecrets validates one pipeline at a time and can never see them.
//
// Attribution is one globally consistent pass over the pool, not a per-key decision.
// Pipelines are visited oldest-to-youngest (CreationTimestamp, ties broken by ascending
// ID) and accepted unless one of their flat keys already belongs to an accepted pipeline
// under a genuinely different (namespace, secretName, key) tuple - a real collision - in
// which case that pipeline is rejected and none of its keys are ever claimed. Since a
// rejected pipeline claims nothing, it cannot victimize anyone else: with A/B/C colliding
// pairwise (A-B on one key, B-C on another), B loses to A, and C then finds nothing
// accepted on its key and survives. Sharing an accepted pipeline's exact tuple is not a
// collision at any age - that is the identical value, not a conflict. The result is the
// maximal deterministic collision-free greedy oldest-first SUBSET, not a prefix: the same
// set every call, independent of argument order.
//
// A pipeline-level error while gathering references (bad spec, disallowed backend shape)
// is returned as the second value rather than folded into "no collisions found": an
// aborted scan is not proof the pool is collision-free. Build*Config runs right after in
// every caller and hits the identical problem.
func DetectSecretCollisions(getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error), pipelines ...pipeline.Pipeline) ([]SecretCollision, error) {
	if len(pipelines) < 2 {
		return nil, nil
	}

	byID, byPipeline, orderedIDs, err := gatherPendingSecretsByPipeline(getter, pipelines)
	if err != nil {
		return nil, err
	}
	if len(byPipeline) == 0 {
		return nil, nil
	}

	// Visit pipelines oldest-to-youngest, greedily accepting each one unless one of
	// its flat keys already belongs to a previously-accepted pipeline with a
	// genuinely different tuple. See this function's doc comment for why this has to
	// be a single pass over the whole pool rather than an independent decision per
	// flat key.
	type acceptedOwner struct {
		resolveNS, secretName, key, id string
	}
	accepted := make(map[string]acceptedOwner)
	victims := make(map[string]SecretCollision)

	for _, id := range orderedIDs {
		refs := append([]pendingSecretRef(nil), byPipeline[id]...)
		// Sorting by flat key makes which conflicting key gets reported (when a
		// rejected pipeline has more than one) deterministic.
		sort.Slice(refs, func(i, j int) bool { return refs[i].flat < refs[j].flat })

		var collision *SecretCollision
		for _, ref := range refs {
			owner, ok := accepted[ref.flat]
			if !ok {
				continue
			}
			// Sharing the accepted owner's exact tuple is not a real collision - it
			// is just another pipeline correctly reading the identical value (same
			// rule resolvePendingSecrets applies, see
			// TestSecretFlatKeySameTupleTwiceSucceeds).
			if owner.resolveNS == ref.resolveNS && owner.secretName == ref.secretName && owner.key == ref.key {
				continue
			}
			collision = &SecretCollision{Victim: byID[id], FlatKey: ref.flat, Survivor: owner.id}
			break
		}
		if collision != nil {
			// Rejected: none of this pipeline's keys are added to accepted, so it
			// can never go on to victimize a later pipeline itself.
			victims[id] = *collision
			continue
		}
		for _, ref := range refs {
			if _, already := accepted[ref.flat]; !already {
				accepted[ref.flat] = acceptedOwner{ref.resolveNS, ref.secretName, ref.key, id}
			}
		}
	}
	if len(victims) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(victims))
	for id := range victims {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]SecretCollision, 0, len(ids))
	for _, id := range ids {
		result = append(result, victims[id])
	}
	return result, nil
}

// SecretSizeDataError marks a DetectSecretSizeOverflow failure that happened while
// reading one specific pipeline's declared secret value (as opposed to a structural
// error gathering references, which is returned unwrapped) - see
// DetectSecretSizeOverflow's doc comment for why callers must treat the two
// differently.
type SecretSizeDataError struct {
	err error
}

func (e *SecretSizeDataError) Error() string { return e.err.Error() }
func (e *SecretSizeDataError) Unwrap() error { return e.err }

// etcdDefaultMaxRequestBytes is etcd's own default for --max-request-bytes: the ceiling
// the whole serialized write - key and value, wrapped in etcd's InternalRaftRequest -
// has to clear. Not importable, since this operator has no etcd dependency of its own,
// so it is pinned here as the value etcd ships with. See SecretAssetsObjectBudget's doc
// comment for why a cluster running a lowered --max-request-bytes is out of scope.
const etcdDefaultMaxRequestBytes = 1572864 // 1.5 MiB

// wrapperReserve is headroom for what the object model still cannot see once the object
// leaves this package, on top of the annotation reserve below: etcd's
// InternalRaftRequest framing around the value (measured in the hundreds of bytes) and
// managedFields, which the API server maintains on every object touched by server-side
// apply and can grow into the tens of KiB on a Secret whose keys churn across many
// reconciles. Unlike the annotation reserve, this is not a hard Kubernetes ceiling -
// ValidateManagedFields (k8s.io/apimachinery/pkg/apis/meta/v1/validation) checks field
// manager names and operation types, not a size or entry-count limit on managedFields
// itself, so nothing caps how large it could theoretically grow. 64 KiB is therefore a
// conservative safety reserve sized against what has actually been observed, not a
// proven bound.
const wrapperReserve = 65536 // 64 KiB

// SecretAssetsObjectBudget caps the MODELLED SIZE OF THE WHOLE assets Secret object -
// the second of the two independent budgets DetectSecretSizeOverflow enforces, the
// first being corev1.MaxSecretSize over values alone.
//
// It exists because a Secret write can be rejected for two unrelated reasons, and only
// one is about values. Past validation the write still has to fit etcd's request limit,
// which counts the whole serialized object - every KEY NAME included. This operator
// generates one flat key per (namespace, pipeline, alias, key), so a workload with many
// pipelines accumulates key-name mass the values-only budget cannot see. Measured on kind
// against a default etcd: ~65-character keys with 24-byte values were accepted at 16250
// keys and rejected at 16560 with a raw `etcdserver: request is too large`, while the sum
// of values was under half of MaxSecretSize. Measured as PROTOBUF, which is what etcd
// stores: Secret.Size() 1 543 789 accepted, 1 573 239 rejected, straddling etcd's default
// MaxRequestBytes of 1 572 864. (The same fixtures as a JSON manifest read ~1.63-1.74 MiB
// - a different serialization, not the number to reason about here.)
//
// The annotation part of the budget is sized against a real Kubernetes ceiling rather than
// a margin that has held so far. The model charges the prototype the workload's own
// builder produces, so it sees the operator's own annotations; what it cannot see is a
// foreign annotation on the EXISTING Secret, because CreateOrUpdateResource merges
// annotations into the existing object instead of replacing them, and one can land in the
// gap between any read and the write - re-reading first does not close that window.
// Kubernetes bounds the blind spot itself: no object may carry more than
// TotalAnnotationSizeLimitB (256 KiB) of annotations, so reserving the full limit closes
// it regardless of what is on the object. wrapperReserve on top is a different kind of
// margin - a conservative reserve for what is invisible for other reasons (request
// framing, managedFields), sized against measurements, see its own doc comment. Together:
//
//	SecretAssetsObjectBudget = etcdDefaultMaxRequestBytes - TotalAnnotationSizeLimitB - wrapperReserve
//	                         = 1 572 864 - 262 144 - 65 536 = 1 245 184 (~1.19 MiB)
//
// It is NOT a promise that an arbitrarily lowered `--max-request-bytes` will still
// work - that is a cluster-side setting no constant here can guarantee, the same way
// SecretAssetsPruneGracePeriod cannot guarantee a cluster's kubelet sync interval.
const SecretAssetsObjectBudget = etcdDefaultMaxRequestBytes - apivalidation.TotalAnnotationSizeLimitB - wrapperReserve

// secretObjectBaseSize is the modelled size of the assets Secret carrying no Data at
// all, so per-entry costs can be added to it. Computed through the generated protobuf
// Size() rather than by hand: etcd stores protobuf, and a hand-rolled model would
// drift from upstream silently.
//
// The caller passes the Secret its own BUILDER produces (the workload controllers'
// SecretAssetsPrototype), not a synthetic one carrying just a name: that object also has
// labels, an ownerReference, and the annotations the user set on the CR's spec - sized at
// the user's discretion up to Kubernetes' 256 KiB ceiling. Modelling only name and
// namespace understated the object by exactly that much, enough on its own to push an
// "accepted" candidate over etcd's limit, and taking the builder's object keeps the model
// from drifting if the builder starts attaching something else.
//
// An annotation put on the EXISTING Secret by something other than this builder is still
// invisible here - it never appears on the prototype. SecretAssetsObjectBudget closes
// that blind spot with a reserve instead.
func secretObjectBaseSize(prototype *corev1.Secret) int {
	if prototype == nil {
		return (&corev1.Secret{}).Size()
	}
	base := prototype.DeepCopy()
	base.Data = nil
	base.StringData = nil
	return base.Size()
}

// secretDataEntrySize is what a single Data entry adds to a Secret's serialized size:
// the key name, the raw value, and protobuf's per-entry framing. Data entries
// contribute independently in the generated Size() (each is summed in its own loop
// iteration), which is what makes a running total valid instead of re-serializing the
// whole candidate map for every pipeline considered - TestSecretObjectSizeModelIsAdditive
// pins that additivity against the real Size(), so an upstream change cannot quietly
// invalidate it.
func secretDataEntrySize(key string, value []byte) int {
	return (&corev1.Secret{Data: map[string][]byte{key: value}}).Size() - (&corev1.Secret{}).Size()
}

// secretObjectSize models the serialized size of the assets Secret that would hold
// data, under the name it is actually written as.
func secretObjectSize(prototype *corev1.Secret, data map[string][]byte) int {
	total := secretObjectBaseSize(prototype)
	for k, v := range data {
		total += secretDataEntrySize(k, v)
	}
	return total
}

// SecretSizeExclusion reports that Victim was excluded from a workload build because
// including its secret values would have pushed the aggregated secret-assets Secret
// past one of the two budgets it has to satisfy: corev1.MaxSecretSize over values
// alone, or SecretAssetsObjectBudget over the modelled size of the whole object. Both
// constrain what every pipeline selected by the workload shares, not any single
// pipeline's own values. See DetectSecretSizeOverflow's doc comment for the
// attribution policy, which is identical for either budget.
type SecretSizeExclusion struct {
	Victim pipeline.Pipeline
	// AcceptedTotal is the number of bytes already committed to pipelines older than
	// Victim (or tied and ordered before it) at the point Victim was considered.
	AcceptedTotal int
	// PipelineBytes is the number of bytes Victim's own secret values would have
	// added, deduplicated by flat key (see DetectSecretSizeOverflow's doc comment on
	// why a value must never be charged twice).
	PipelineBytes int

	// ObjectBudget tells the two budgets apart. False means the values-only limit was
	// the one exceeded, and AcceptedTotal/PipelineBytes describe it. True means the
	// values fit but the modelled OBJECT size did not, and the two Object* fields
	// below are the ones that describe the overflow.
	//
	// The distinction reaches the user, deliberately: the two have different causes
	// (large values vs many long key names) and different remedies, so reporting an
	// object-size overflow in the values-limit wording would tell a workload whose
	// values are nowhere near 1 MiB to go looking at the wrong thing entirely.
	ObjectBudget bool
	// AcceptedObjectBytes is the modelled object size already committed - the
	// Secret's own metadata included - at the point Victim was considered.
	AcceptedObjectBytes int
	// PipelineObjectBytes is what Victim's own entries (key names, values, and
	// protobuf framing) would have added to that modelled size.
	PipelineObjectBytes int
}

// DetectSecretSizeOverflow is a read-only pre-pass over the same pipeline list a
// workload build (BuildAgentConfig/BuildAggregatorConfig) is about to receive,
// checking whether their combined secret values fit under corev1.MaxSecretSize before
// the aggregated secret-assets Secret is actually built. Without this pre-pass,
// Build*Config only discovers an overflow by trying to write that Secret through the
// ephemeral configcheck Secret and getting back a raw "Too long: may not be more than
// 1048576 bytes" API error - which fails the WHOLE merged build (every pipeline on
// the workload, not just whichever ones' combined values overflowed it) and
// attributes the failure to nobody in particular.
//
// Attribution mirrors DetectSecretCollisions and shares its traversal
// (gatherPendingSecretsByPipeline: oldest to youngest, ties broken by ID). Pipelines are
// accepted greedily while their own deduplicated bytes still fit next to every accepted
// pipeline's; one that would push the total over is rejected whole, contributing neither
// bytes nor flat keys. So a rejection never hands its budget to someone younger, and
// never disqualifies them either - a smaller pipeline after a rejected one is still
// evaluated on its own merits. The result is the maximal greedy oldest-first SUBSET that
// fits, not a prefix: the same set every call, independent of argument order.
//
// A flat key already claimed by an accepted pipeline is not charged twice. Two pipelines
// can share a flat key only by referencing the identical (namespace, secretName, key)
// tuple - anything else on a shared key is a collision, already removed by
// DetectSecretCollisions - so Secret.Data holds that value once, and double-charging it
// would reject pipelines the real write would have accepted. Identical bytes under two
// different flat keys ARE charged twice, matching how Kubernetes counts them.
//
// pipelines is expected to already exclude collision victims: a victim never reaches
// Build*Config either, so it must not consume any of the size budget. Unlike
// DetectSecretCollisions this reads actual values through getter, memoized per Secret -
// size cannot be known from identity alone.
//
// Two error classes, which callers must tell apart:
//
//   - A structural error gathering references (bad spec, disallowed backend shape) is
//     returned as a plain error: the scan never got far enough to say anything about ANY
//     pipeline, so the caller must not read it as proof the pool fits.
//   - A per-pipeline data error reading a value (Secret gone, key removed, API failure)
//     is wrapped in *SecretSizeDataError. It is local to that one pipeline and says
//     nothing about the rest of the pool, so resolveWorkloadPipelines skips size
//     attribution for the round and lets Build*Config's own resolvePendingSecrets report
//     it against the pipeline it belongs to.
func DetectSecretSizeOverflow(ctx context.Context, getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error), assetsPrototype *corev1.Secret, pipelines ...pipeline.Pipeline) ([]SecretSizeExclusion, error) {
	if len(pipelines) == 0 {
		return nil, nil
	}

	byID, byPipeline, orderedIDs, err := gatherPendingSecretsByPipeline(getter, pipelines)
	if err != nil {
		return nil, err
	}
	if len(byPipeline) == 0 {
		return nil, nil
	}

	secretCache := make(map[string]*corev1.Secret)
	valueOf := func(ref pendingSecretRef) ([]byte, error) {
		cacheKey := ref.resolveNS + "/" + ref.secretName
		secret, ok := secretCache[cacheKey]
		if !ok {
			var err error
			secret, err = getter(ctx, ref.resolveNS, ref.secretName)
			if err != nil {
				return nil, &SecretSizeDataError{err: fmt.Errorf("failed to get secret %s/%s: %w", ref.resolveNS, ref.secretName, err)}
			}
			secretCache[cacheKey] = secret
		}
		val, ok := secret.Data[ref.key]
		if !ok {
			return nil, &SecretSizeDataError{err: fmt.Errorf("secret %s/%s: key %q not found", ref.resolveNS, ref.secretName, ref.key)}
		}
		if !secretValueSafeForJSONText(val) {
			return nil, &SecretSizeDataError{err: &SecretValueUnsafeError{SecretNamespace: ref.resolveNS, SecretName: ref.secretName, Key: ref.key}}
		}
		return val, nil
	}

	accepted := make(map[string]struct{}, len(byPipeline)) // flat keys already counted
	total := 0
	// The object budget starts at what an empty assets Secret already costs: its
	// metadata is charged to the workload whether any pipeline uses secrets or not.
	objectTotal := secretObjectBaseSize(assetsPrototype)
	var exclusions []SecretSizeExclusion

	for _, id := range orderedIDs {
		refs := byPipeline[id]

		// Dedup within this pipeline's own refs first (the same flat key can be
		// referenced by more than one component), then drop anything already
		// claimed by an older accepted pipeline - see the doc comment above for why
		// neither case may be charged twice.
		seen := make(map[string]struct{}, len(refs))
		var own []pendingSecretRef
		for _, ref := range refs {
			if _, dup := seen[ref.flat]; dup {
				continue
			}
			seen[ref.flat] = struct{}{}
			if _, already := accepted[ref.flat]; already {
				continue
			}
			own = append(own, ref)
		}

		marginal := 0
		objectMarginal := 0
		for _, ref := range own {
			val, err := valueOf(ref)
			if err != nil {
				return nil, err
			}
			marginal += len(val)
			objectMarginal += secretDataEntrySize(ref.flat, val)
		}

		// The values budget is checked first, so a pipeline that breaches both is
		// reported against the limit the API server itself would name - the one whose
		// error text ("Too long: may not be more than 1048576 bytes") a user can look
		// up. Only a pipeline whose values genuinely fit is attributed to the object
		// budget, which is the case with no API error text of its own worth quoting.
		if total+marginal > corev1.MaxSecretSize {
			exclusions = append(exclusions, SecretSizeExclusion{
				Victim:        byID[id],
				AcceptedTotal: total,
				PipelineBytes: marginal,
			})
			continue
		}

		if objectTotal+objectMarginal > SecretAssetsObjectBudget {
			exclusions = append(exclusions, SecretSizeExclusion{
				Victim:              byID[id],
				AcceptedTotal:       total,
				PipelineBytes:       marginal,
				ObjectBudget:        true,
				AcceptedObjectBytes: objectTotal,
				PipelineObjectBytes: objectMarginal,
			})
			continue
		}

		total += marginal
		objectTotal += objectMarginal
		for _, ref := range own {
			accepted[ref.flat] = struct{}{}
		}
	}

	if len(exclusions) == 0 {
		return nil, nil
	}
	sort.Slice(exclusions, func(i, j int) bool {
		return pipelineID(exclusions[i].Victim) < pipelineID(exclusions[j].Victim)
	})
	return exclusions, nil
}

// BridgeAssets solves a different problem than DetectSecretSizeOverflow: given the
// FINAL target pipeline set (already the deterministic oldest-first survivors of
// DetectSecretSizeOverflow - computed independently of any Secret's current
// contents), decide which of them can actually be staged into the assets Secret THIS
// round without exceeding corev1.MaxSecretSize, given that existing (the assets
// Secret's current Data) may already hold values a not-yet-rolled pod's live config
// could still need and must not be dropped yet.
//
// The two-object design (config Secret + assets Secret, no transaction between them, no
// rollout-completion signal - see docs/secrets.md) means a pipeline whose values fit the
// final target can still, transiently, not fit ALONGSIDE stale data the assets Secret has
// not been pruned of yet: swapping one large pipeline for another near the ceiling
// overflows in neither the before nor the after state, but briefly does in their union.
// BridgeAssets makes that transition converge over a few reconciles instead of corrupting
// a live pod's config or refusing forever:
//
//   - bridgeData is existing augmented with every accepted pipeline's values, never a
//     wholesale replacement, so nothing already staged is dropped - always safe to write
//     immediately and guaranteed <= corev1.MaxSecretSize.
//   - waiting is the subset that could not be accepted this round. They stay part of the
//     final target; the caller must show them as "waiting for room" - neither failed nor
//     valid - and reconsider them once a later prune has freed space.
//
// Pipelines are walked oldest-first, accepted greedily while the running total stays under
// the limit, so an older pipeline that fits is never blocked by a younger one that does
// not - the same deterministic, non-knapsack policy as size attribution. A flat key
// already present in existing with byte-identical value costs nothing; a rotated one is
// charged its actual net change, measured by tentatively applying it, so a config change
// and a rotation in the same round compose instead of both being charged worst-case.
//
// When the full target fits in one step - the common case - waiting is empty and
// bridgeData is exactly the target's Secret.Data, so len(waiting) == 0 distinguishes "no
// bridge needed" from "a bridge round happened".
func BridgeAssets(ctx context.Context, getter func(ctx context.Context, namespace, name string) (*corev1.Secret, error), assetsPrototype *corev1.Secret, existing map[string][]byte, finalPipelines []pipeline.Pipeline) (bridgeData map[string][]byte, waiting []pipeline.Pipeline, err error) {
	bridgeData = make(map[string][]byte, len(existing))
	for k, v := range existing {
		bridgeData[k] = v
	}
	if len(finalPipelines) == 0 {
		return bridgeData, nil, nil
	}

	byID, byPipeline, orderedIDs, err := gatherPendingSecretsByPipeline(getter, finalPipelines)
	if err != nil {
		return nil, nil, err
	}
	if len(byPipeline) == 0 {
		return bridgeData, nil, nil
	}

	secretCache := make(map[string]*corev1.Secret)
	valueOf := func(ref pendingSecretRef) ([]byte, error) {
		cacheKey := ref.resolveNS + "/" + ref.secretName
		secret, ok := secretCache[cacheKey]
		if !ok {
			var err error
			secret, err = getter(ctx, ref.resolveNS, ref.secretName)
			if err != nil {
				return nil, &SecretSizeDataError{err: fmt.Errorf("failed to get secret %s/%s: %w", ref.resolveNS, ref.secretName, err)}
			}
			secretCache[cacheKey] = secret
		}
		val, ok := secret.Data[ref.key]
		if !ok {
			return nil, &SecretSizeDataError{err: fmt.Errorf("secret %s/%s: key %q not found", ref.resolveNS, ref.secretName, ref.key)}
		}
		if !secretValueSafeForJSONText(val) {
			return nil, &SecretSizeDataError{err: &SecretValueUnsafeError{SecretNamespace: ref.resolveNS, SecretName: ref.secretName, Key: ref.key}}
		}
		return val, nil
	}

	// Measured once, up front, over whatever the assets Secret already holds; from
	// here on both totals move by per-pipeline deltas only.
	valuesTotal := secretDataSize(bridgeData)
	objectTotal := secretObjectSize(assetsPrototype, bridgeData)

	for _, id := range orderedIDs {
		refs := byPipeline[id]

		seen := make(map[string]struct{}, len(refs))
		tentative := make(map[string][]byte, len(refs))
		var fetchErr error
		for _, ref := range refs {
			if _, dup := seen[ref.flat]; dup {
				continue
			}
			seen[ref.flat] = struct{}{}
			val, err := valueOf(ref)
			if err != nil {
				fetchErr = err
				break
			}
			tentative[ref.flat] = val
		}
		if fetchErr != nil {
			return nil, nil, fetchErr
		}

		// Apply tentatively (own flat keys only - already-accepted pipelines from
		// earlier in this loop stay in bridgeData regardless of what happens to
		// this one), measure the real total, and roll back precisely if it does
		// not fit: capture only the keys this pipeline actually touches, and
		// whether each one existed before, so a rollback restores bridgeData
		// exactly rather than guessing at a delta.
		type priorState struct {
			existed bool
			value   []byte
		}
		prior := make(map[string]priorState, len(tentative))
		// Both totals are carried across the loop and moved by this pipeline's own
		// delta instead of being re-measured over the whole candidate map every time.
		// At the scale the object budget exists for - thousands of pipelines over
		// thousands of keys - re-measuring is quadratic, and the object measurement in
		// particular serialises per entry. The deltas are exact for the same reason the
		// pre-pass may keep a running total: Data entries contribute independently to
		// the modelled size (see secretDataEntrySize and
		// TestSecretObjectSizeModelIsAdditive). A key that REPLACES an existing one is
		// charged the difference between its new and old entry, never the new one on
		// top of the old.
		valuesDelta, objectDelta := 0, 0
		for k, v := range tentative {
			old, existed := bridgeData[k]
			prior[k] = priorState{existed: existed, value: old}
			if existed {
				valuesDelta -= len(old)
				objectDelta -= secretDataEntrySize(k, old)
			}
			valuesDelta += len(v)
			objectDelta += secretDataEntrySize(k, v)
			bridgeData[k] = v
		}

		// Both budgets, for the same reason the pre-pass checks both: a staged union
		// that fits by values can still be an object the API server refuses to take.
		// A bridge round that ignored the object budget would hand back data whose
		// write fails outright, which is worse than making the pipeline wait.
		if valuesTotal+valuesDelta > corev1.MaxSecretSize ||
			objectTotal+objectDelta > SecretAssetsObjectBudget {
			for k, p := range prior {
				if p.existed {
					bridgeData[k] = p.value
				} else {
					delete(bridgeData, k)
				}
			}
			waiting = append(waiting, byID[id])
			continue
		}

		valuesTotal += valuesDelta
		objectTotal += objectDelta
	}

	return bridgeData, waiting, nil
}

// secretDataSize sums the byte length of every value in a Secret's Data map, the
// same quantity the Kubernetes API server enforces corev1.MaxSecretSize against
// (only values count, not keys).
func secretDataSize(data map[string][]byte) int {
	total := 0
	for _, v := range data {
		total += len(v)
	}
	return total
}
