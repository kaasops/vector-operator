package aggregator

import (
	"context"
	"fmt"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	resourcev1 "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/internal/buildinfo"
	"github.com/kaasops/vector-operator/internal/config"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

type Aggregator interface {
	client.Object
}

type Controller struct {
	client.Client
	// APIReader is an uncached read-only client (mgr.GetAPIReader()), used by every
	// secret-assets safeguard - the write-order gate, the bridge plan and the prune gate -
	// see the agent Controller's identical field and secretAssetsSafetyReader for why none
	// of them may be answered from the cache, and why this is not defaulted to Client.
	APIReader           client.Reader
	id                  string
	Name                string
	Namespace           string
	VectorAggregator    Aggregator
	APIVersion          string
	Kind                string
	Spec                *vectorv1alpha1.VectorAggregatorCommon
	Status              *vectorv1alpha1.VectorCommonStatus
	ConfigBytes         []byte
	Config              *config.VectorConfig
	ClientSet           kubernetes.Interface
	isClusterAggregator bool

	// SecretAssets holds the resolved pipeline secret data (cfg.SecretAssets()) to
	// materialize into the secret-assets Secret and mount into the workload. Empty
	// when no pipeline references a secret (zero-churn: no Secret, no volume, no mount).
	SecretAssets map[string][]byte
}

func NewController(
	v Aggregator,
	c client.Client,
	cs kubernetes.Interface,
) *Controller {
	ctrl := &Controller{
		Client:           c,
		VectorAggregator: v,
		ClientSet:        cs,
	}

	switch agg := v.(type) {
	case *vectorv1alpha1.VectorAggregator:
		ctrl.isClusterAggregator = false
		ctrl.Spec = &agg.Spec.VectorAggregatorCommon
		ctrl.Name = agg.Name
		ctrl.Namespace = agg.Namespace
		ctrl.Status = &agg.Status.VectorCommonStatus
		ctrl.APIVersion = agg.APIVersion
		ctrl.Kind = agg.Kind
		ctrl.id = types.NamespacedName{Name: agg.Name, Namespace: agg.Namespace}.String()
	case *vectorv1alpha1.ClusterVectorAggregator:
		ctrl.isClusterAggregator = true
		ctrl.Spec = &agg.Spec.VectorAggregatorCommon
		ctrl.Name = agg.Name
		ctrl.Namespace = agg.Spec.ResourceNamespace
		ctrl.Status = &agg.Status.VectorCommonStatus
		ctrl.APIVersion = agg.APIVersion
		ctrl.Kind = agg.Kind
		ctrl.id = types.NamespacedName{Name: agg.Name}.String()
	}

	ctrl.setDefault()
	return ctrl
}

// EnsureVectorAggregator reconciles the aggregator Deployment/StatefulSet and
// everything it depends on.
//
// ctrl.SecretAssets is written before the config that may reference it - see
// ensureVectorAggregatorSecretAssets' doc comment for why this order is the one that
// cannot crash-loop the workload, and for why the CALLER (not this function) is
// responsible for choosing exactly what ctrl.SecretAssets should contain this round.
//
// Where the workload's own pod template write falls relative to assets/config
// depends on whether this round is CROSSING the secret-assets mount's
// presence/absence boundary - see hasSecretAssetsMount and the agent's identical
// EnsureVectorAgent for the full rationale (a new pod, created by ANY trigger, gets
// whatever template happens to be persisted at that instant, so the mount must
// never be behind what the config it will read references).
//
// configPublishing is true exactly when the caller's configUnchanged was false this
// round - see the agent's identical EnsureVectorAgent and StampConfigPublishing's
// doc comment for why, when true, the publish mark is stamped after assets succeeds
// and immediately before the first config write.
func (ctrl *Controller) EnsureVectorAggregator(ctx context.Context, configPublishing bool) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator")

	monitoringCRD, err := k8s.ResourceExists(ctrl.ClientSet.Discovery(), monitorv1.SchemeGroupVersion.String(), monitorv1.PodMonitorsKind)
	if err != nil {
		return err
	}

	ensureWorkload := ctrl.ensureVectorAggregatorDeployment
	if ctrl.persistenceEnabled() {
		ensureWorkload = ctrl.ensureVectorAggregatorStatefulSet
	}

	// One uncached read of both potential workload kinds, serving two decisions that
	// need DIFFERENT answers out of it - see secretAssetsMountState's doc comment.
	mountState, err := ctrl.readSecretAssetsMountState(ctx)
	if err != nil {
		return err
	}
	willHaveMount := len(ctrl.SecretAssets) > 0
	obsoleteWorkloadExists := mountState.obsoleteWorkloadExists(ctrl.persistenceEnabled())

	stampBeforeConfig := func() error {
		if !configPublishing {
			return nil
		}
		return ctrl.StampConfigPublishing(ctx)
	}

	switch {
	// Gaining the mount asks whether EVERY live workload already has it: a config
	// that references a key is unsafe for any workload still missing the mount, so
	// one unmounted kind is enough to require this order.
	case willHaveMount && !mountState.allMounted():
		if err := ctrl.ensureVectorAggregatorSecretAssets(ctx); err != nil {
			return err
		}
		if err := ensureWorkload(ctx, obsoleteWorkloadExists); err != nil {
			return err
		}
		if err := stampBeforeConfig(); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAggregatorConfig(ctx); err != nil {
			return err
		}
	// Losing it asks the opposite question - whether ANY live workload still has it:
	// the assets Secret may not be deleted while a single pod anywhere still mounts it.
	case !willHaveMount && mountState.anyMounted():
		// Stamped here too, and unconditionally - see the agent's identical branch in
		// EnsureVectorAgent for why "this branch implies configPublishing is false" is
		// not an invariant this switch can rely on.
		if err := stampBeforeConfig(); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAggregatorConfig(ctx); err != nil {
			return err
		}
		if err := ensureWorkload(ctx, obsoleteWorkloadExists); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAggregatorSecretAssets(ctx); err != nil {
			return err
		}
	default:
		if err := ctrl.ensureVectorAggregatorSecretAssets(ctx); err != nil {
			return err
		}
		if err := stampBeforeConfig(); err != nil {
			return err
		}
		if err := ctrl.ensureVectorAggregatorConfig(ctx); err != nil {
			return err
		}
		// Kept right after config, before RBAC/Service/PodMonitor below - a
		// non-essential step's transient failure must not withhold the template
		// update indefinitely.
		if err := ensureWorkload(ctx, obsoleteWorkloadExists); err != nil {
			return err
		}
	}

	if err := ctrl.ensureVectorAggregatorRBAC(ctx); err != nil {
		return err
	}

	if err := ctrl.ensureVectorAggregatorService(ctx); err != nil {
		return err
	}

	if ctrl.Spec.InternalMetrics && monitoringCRD {
		if err := ctrl.ensureVectorAggregatorPodMonitor(ctx); err != nil {
			return err
		}
	}

	if err := ctrl.ensureEventCollector(ctx); err != nil {
		return err
	}

	if err := ctrl.ensureVectorAggregatorHPA(ctx); err != nil {
		return err
	}

	// Kept last: a rejected PodDisruptionBudget write (admission policy, quota,
	// RBAC) must not keep the aggregator from getting its event collector and HPA.
	if err := ctrl.ensureVectorAggregatorPodDisruptionBudget(ctx); err != nil {
		return err
	}

	return nil
}

// secretAssetsMountState is what one uncached read of BOTH potential workload kinds
// (Deployment and StatefulSet - they share a name, and a persistence toggle can
// leave both alive at once) says about the operator's own secret-assets mount.
//
// It is kept as per-kind facts rather than a single boolean because the two write
// orders it feeds ask genuinely different questions of it, and folding both kinds
// into one "somebody has the mount" answer gets one of them wrong:
//
//   - Losing the mount is about ANY workload still having it (anyMounted): the assets
//     Secret must not be deleted while a single live pod anywhere still mounts it.
//   - Gaining it is about EVERY workload already having it (allMounted): a config that
//     references a key is unsafe for any workload that does NOT mount it, so one
//     unmounted kind is enough to require the gaining order.
//
// That mixed state is reachable, not theoretical: ensureWorkload writes the new kind
// BEFORE deleting the old one, so a failed or not-yet-run delete leaves both alive with
// independent pod templates - a Deployment still carrying the mount next to a StatefulSet
// created on a round without secrets. Folded with OR, the next round that adds a secret
// back would answer "already mounted", take the ordinary order, and publish a config
// referencing a key the StatefulSet's pods cannot resolve.
type secretAssetsMountState struct {
	deploymentExists   bool
	deploymentMounted  bool
	statefulSetExists  bool
	statefulSetMounted bool
}

// anyMounted reports whether at least one live workload carries the mount.
func (s secretAssetsMountState) anyMounted() bool {
	return s.deploymentMounted || s.statefulSetMounted
}

// allMounted reports whether every live workload carries the mount. A workload that
// does not exist at all makes this false rather than vacuously true: the very first
// reconcile of a fresh aggregator has to take the gaining order (assets, then the
// workload that mounts them, then the config that references them), exactly like a
// workload whose template predates the feature.
func (s secretAssetsMountState) allMounted() bool {
	if !s.deploymentExists && !s.statefulSetExists {
		return false
	}
	return (!s.deploymentExists || s.deploymentMounted) &&
		(!s.statefulSetExists || s.statefulSetMounted)
}

// obsoleteWorkloadExists reports whether the kind the CURRENT persistence setting does
// not select is still on the cluster - the leftover deleteObsoleteWorkload has to
// remove. Answering from this snapshot is what keeps that decision off the cache; see
// deleteObsoleteWorkload's doc comment.
func (s secretAssetsMountState) obsoleteWorkloadExists(persistenceEnabled bool) bool {
	if persistenceEnabled {
		return s.deploymentExists
	}
	return s.statefulSetExists
}

// readSecretAssetsMountState takes that snapshot: both kinds CURRENTLY PERSISTED on
// the cluster - not the one this reconcile is about to write - and whether each has
// the OPERATOR'S OWN secret-assets mount in its pod template (see
// k8s.HasOperatorSecretAssetsMount's doc comment for why a bare volume-name match is
// not enough).
//
// Both reads go through ctrl.APIReader - an uncached snapshot from the API server at
// decision time - see the agent's hasSecretAssetsMount for why a gate that picks the
// write order must not be decided off the informer cache, and why a read failure
// aborts the round before any of the writes it orders instead of falling back to the
// cache.
//
// Both kinds are read unconditionally and the order is immaterial - there is nothing to
// short-circuit. Only anyMounted() could stop at the first mounted kind; allMounted()
// must know about the other kind (an unmounted leftover is what it exists to notice), and
// obsoleteWorkloadExists() asks about the kind persistenceEnabled() does NOT select, so
// it needs exactly the read a current-kind-first order would have skipped. Two reads
// always, served from the API server rather than the cache - the deliberate price of not
// deciding a safeguard off possibly stale data.
func (ctrl *Controller) readSecretAssetsMountState(ctx context.Context) (secretAssetsMountState, error) {
	var state secretAssetsMountState
	if ctrl.APIReader == nil {
		return state, fmt.Errorf("APIReader is not set: the secret-assets write-order gate must not be decided from the cache")
	}
	key := client.ObjectKey{Namespace: ctrl.Namespace, Name: ctrl.getNameVectorAggregator()}
	secretName := ctrl.getSecretAssetsName()

	dep := &appsv1.Deployment{}
	if err := ctrl.APIReader.Get(ctx, key, dep); err != nil {
		if !api_errors.IsNotFound(err) {
			return state, err
		}
	} else {
		state.deploymentExists = true
		state.deploymentMounted = k8s.HasOperatorSecretAssetsMount(dep.Spec.Template.Spec, secretName, config.SecretsMountPath)
	}

	sts := &appsv1.StatefulSet{}
	if err := ctrl.APIReader.Get(ctx, key, sts); err != nil {
		if !api_errors.IsNotFound(err) {
			return state, err
		}
	} else {
		state.statefulSetExists = true
		state.statefulSetMounted = k8s.HasOperatorSecretAssetsMount(sts.Spec.Template.Spec, secretName, config.SecretsMountPath)
	}

	return state, nil
}

func (ctrl *Controller) DeleteVectorAggregator(ctx context.Context) error {
	if err := ctrl.deleteVectorAggregatorClusterRole(ctx); err != nil {
		return err
	}
	if err := ctrl.deleteVectorAggregatorClusterRoleBinding(ctx); err != nil {
		return err
	}
	return nil
}

func (ctrl *Controller) setDefault() {
	if ctrl.Spec.Image == "" {
		ctrl.Spec.Image = "timberio/vector:0.48.0-distroless-libc"
	}

	if ctrl.Spec.Resources.Requests == nil {
		ctrl.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.Spec.Resources.Limits == nil {
		ctrl.Spec.Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}

	if ctrl.Spec.DataDir == "" {
		ctrl.Spec.DataDir = "/var/lib/vector"
	}

	if ctrl.persistenceEnabled() {
		if len(ctrl.Spec.Persistence.AccessModes) == 0 {
			ctrl.Spec.Persistence.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		}
		if ctrl.Spec.Persistence.Size.IsZero() {
			ctrl.Spec.Persistence.Size = resourcev1.MustParse("10Gi")
		}
		if ctrl.Spec.Persistence.RetentionPolicy == nil {
			ctrl.Spec.Persistence.RetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			}
		}
	}

	if ctrl.Spec.Volumes == nil {
		ctrl.Spec.Volumes = []corev1.Volume{
			{
				Name: "var-log",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/log/",
					},
				},
			},
			{
				Name: "journal",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/log/journal",
					},
				},
			},
			{
				Name: "var-lib",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/var/lib/",
					},
				},
			},
		}
	}

	if ctrl.Spec.ReadinessProbe == nil && ctrl.Spec.Api.Enabled && ctrl.Spec.Api.Healthcheck {
		ctrl.Spec.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.IntOrString{
						Type:   intstr.Type(0),
						IntVal: 8686,
					},
				},
			},
			PeriodSeconds:       20,
			InitialDelaySeconds: 15,
			TimeoutSeconds:      3,
			SuccessThreshold:    0,
			FailureThreshold:    0,
		}
	}
	if ctrl.Spec.LivenessProbe == nil && ctrl.Spec.Api.Enabled && ctrl.Spec.Api.Healthcheck {
		ctrl.Spec.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.IntOrString{
						Type:   intstr.Type(0),
						IntVal: 8686,
					},
				},
			},
			PeriodSeconds:       20,
			InitialDelaySeconds: 15,
			TimeoutSeconds:      3,
			SuccessThreshold:    0,
			FailureThreshold:    0,
		}
	}

	if ctrl.Spec.VolumeMounts == nil {
		ctrl.Spec.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "var-log",
				MountPath: "/var/log/",
			},
			{
				Name:      "journal",
				MountPath: "/run/log/journal",
			},
			{
				Name:      "var-lib",
				MountPath: "/var/lib/",
			},
		}
	}
	if ctrl.Spec.CompressConfigFile && ctrl.Spec.ConfigReloaderImage == "" {
		ctrl.Spec.ConfigReloaderImage = "docker.io/kaasops/config-reloader:v0.1.4"
	}
	if ctrl.Spec.CompressConfigFile && ctrl.Spec.ConfigReloaderResources.Requests == nil {
		ctrl.Spec.ConfigReloaderResources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("200Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("100m"),
		}
	}
	if ctrl.Spec.CompressConfigFile && ctrl.Spec.ConfigReloaderResources.Limits == nil {
		ctrl.Spec.ConfigReloaderResources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		}
	}
	if ctrl.Spec.EventCollector.Image == "" {
		ctrl.Spec.EventCollector.Image = "kaasops/event-collector:" + buildinfo.Version
	}
	if ctrl.Spec.EventCollector.ImagePullPolicy == "" {
		ctrl.Spec.EventCollector.ImagePullPolicy = corev1.PullIfNotPresent
	}
	if ctrl.Spec.EventCollector.MaxBatchSize <= 0 {
		ctrl.Spec.EventCollector.MaxBatchSize = 250
	}
}

// statusPatchBase returns the patch base for a status write that clears the reason. A merge
// patch only clears the keys it mentions, and the base can predate the reason it has to
// clear, so the base carries a reason whatever it was read with.
func (ctrl *Controller) statusPatchBase() client.Object {
	base := ctrl.VectorAggregator.DeepCopyObject().(client.Object)
	switch agg := base.(type) {
	case *vectorv1alpha1.VectorAggregator:
		agg.Status.Reason = ptr.To("")
	case *vectorv1alpha1.ClusterVectorAggregator:
		agg.Status.Reason = ptr.To("")
	}
	return base
}

// configPublished is true when the config Secret was actually (re-)written this
// round (i.e. the caller's configUnchanged was false) - see the agent's identical
// SetSuccessStatus for the full rationale, including why LastConfigPublishedAt is
// also seeded when still nil regardless of configPublished.
func (ctrl *Controller) SetSuccessStatus(ctx context.Context, hash, globCfgHash *int64, configPublished bool) error {
	base := ctrl.statusPatchBase()
	var status = true
	ctrl.Status.ConfigCheckResult = &status
	ctrl.Status.Reason = nil
	ctrl.Status.LastAppliedConfigHash = hash
	ctrl.Status.LastAppliedGlobalConfigHash = globCfgHash
	if configPublished || ctrl.Status.LastConfigPublishedAt == nil {
		now := metav1.Now()
		ctrl.Status.LastConfigPublishedAt = &now
	}
	return k8s.PatchStatus(ctx, ctrl.VectorAggregator, base, ctrl.Client)
}

// StampConfigPublishing writes ONLY LastConfigPublishedAt - see the agent's
// identical StampConfigPublishing for the full rationale (the gap SetSuccessStatus
// alone leaves between a successful config write and the end-of-reconcile status
// update, and why this must run after assets are staged and immediately before the
// first config write, never before or after) and for why it has to write through a
// DEEP COPY of the underlying CR, never ctrl.VectorAggregator itself: the same
// controller-runtime behavior applies here (Status().Update() decodes the server's
// full response, including the not-actually-persisted-yet spec defaults
// setDefault() applied only in memory, back into whatever pointer it was given),
// and ensureVectorAggregatorConfig/the workload write immediately after this call
// would otherwise build from a silently reverted spec.
func (ctrl *Controller) StampConfigPublishing(ctx context.Context) error {
	now := metav1.Now()
	stamped, ok := ctrl.VectorAggregator.DeepCopyObject().(Aggregator)
	if !ok {
		return fmt.Errorf("StampConfigPublishing: %T does not deep-copy into an Aggregator", ctrl.VectorAggregator)
	}

	base, ok := ctrl.VectorAggregator.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("StampConfigPublishing: %T does not deep-copy into a client.Object", ctrl.VectorAggregator)
	}

	var resourceVersion string
	switch agg := stamped.(type) {
	case *vectorv1alpha1.VectorAggregator:
		agg.Status.LastConfigPublishedAt = &now
		if err := k8s.PatchStatus(ctx, agg, base, ctrl.Client); err != nil {
			return err
		}
		resourceVersion = agg.ResourceVersion
	case *vectorv1alpha1.ClusterVectorAggregator:
		agg.Status.LastConfigPublishedAt = &now
		if err := k8s.PatchStatus(ctx, agg, base, ctrl.Client); err != nil {
			return err
		}
		resourceVersion = agg.ResourceVersion
	default:
		return fmt.Errorf("StampConfigPublishing: unsupported Aggregator type %T", stamped)
	}

	ctrl.VectorAggregator.SetResourceVersion(resourceVersion)
	ctrl.Status.LastConfigPublishedAt = &now
	return nil
}

func (ctrl *Controller) SetFailedStatus(ctx context.Context, reason string) error {
	base := ctrl.VectorAggregator.DeepCopyObject().(client.Object)
	var status = false
	ctrl.Status.ConfigCheckResult = &status
	ctrl.Status.Reason = &reason
	return k8s.PatchStatus(ctx, ctrl.VectorAggregator, base, ctrl.Client)
}

func (ctrl *Controller) matchLabelsForVectorAggregator() map[string]string {
	return map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector",
		k8s.ComponentLabelKey: "Aggregator",
		k8s.InstanceLabelKey:  ctrl.Name,
	}
}

func (ctrl *Controller) labelsForVectorAggregator() map[string]string {
	basicLabels := ctrl.matchLabelsForVectorAggregator()

	labels := k8s.MergeLabels(basicLabels, ctrl.Spec.Labels)

	return labels
}

func (ctrl *Controller) annotationsForVectorAggregator() map[string]string {
	return ctrl.Spec.Annotations
}

func (ctrl *Controller) objectMetaVectorAggregator(labels map[string]string, annotations map[string]string, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            ctrl.getNameVectorAggregator(),
		Namespace:       namespace,
		Labels:          labels,
		Annotations:     annotations,
		OwnerReferences: ctrl.getControllerReference(),
	}
}

func (ctrl *Controller) getNameVectorAggregator() string {
	name := ctrl.Name + "-aggregator"
	return name
}

// getSecretAssetsName returns the name of the Secret that materializes the
// pipeline secret data mounted at config.SecretsMountPath.
func (ctrl *Controller) getSecretAssetsName() string {
	return ctrl.getNameVectorAggregator() + "-secret-assets"
}

// SecretAssetsPrototype returns the assets Secret as this controller would build it,
// minus the data - see the agent Controller's identical accessor.
func (ctrl *Controller) SecretAssetsPrototype() *corev1.Secret {
	prototype := ctrl.createSecretAssetsSecret()
	prototype.Data = nil
	return prototype
}

// getHeadlessServiceName returns the name of the headless service that governs
// the StatefulSet, providing stable per replica DNS in persistent mode.
func (ctrl *Controller) getHeadlessServiceName() string {
	return ctrl.getNameVectorAggregator() + "-headless"
}

func (ctrl *Controller) getControllerReference() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         ctrl.APIVersion,
			Kind:               ctrl.Kind,
			Name:               ctrl.VectorAggregator.GetName(),
			UID:                ctrl.VectorAggregator.GetUID(),
			BlockOwnerDeletion: ptr.To(true),
			Controller:         ptr.To(true),
		},
	}
}

func (ctrl *Controller) GetServiceName() string {
	return ctrl.getNameVectorAggregator()
}

// persistenceEnabled reports whether the aggregator should run as a StatefulSet
// with persistent disk buffers. It is true when persistence is explicitly enabled
// or when raw volume claim templates are supplied through the escape hatch.
func (ctrl *Controller) persistenceEnabled() bool {
	return ctrl.Spec.Persistence.Enabled || len(ctrl.Spec.Persistence.VolumeClaimTemplates) > 0
}

// deleteObsoleteWorkload removes a workload of the opposite kind left over from a
// previous persistence mode. Toggling persistence switches between a Deployment
// and a StatefulSet that share a name and pod labels, so the stale one must be
// removed or its pods keep serving alongside the new workload - and, while pipeline
// secrets are in play, keep mounting an assets Secret the mount-losing order is about
// to delete.
//
// It issues the DELETE unconditionally and treats a server-side NotFound as success.
// It used to preflight with a cached GET and return early on NotFound, which made a
// cache lag decide a safety step: a workload the API server still has, but the
// informer has not observed yet, was reported gone, no DELETE was sent, and the round
// continued as if the old kind were retired - publishing a config, or deleting the
// assets Secret, out from under its still-running pods. Callers instead gate this on
// secretAssetsMountState's uncached snapshot, so the "skip the no-op DELETE in steady
// state" economy is kept without the cache getting a vote (an extra DELETE per
// reconcile per aggregator is not free on a busy API server, which is why the guard
// was moved rather than dropped). The NotFound tolerance covers the remaining gap:
// the snapshot is taken before the new workload is written, and the leftover may be
// gone by the time this runs.
func (ctrl *Controller) deleteObsoleteWorkload(ctx context.Context, obj client.Object) error {
	obj.SetName(ctrl.getNameVectorAggregator())
	obj.SetNamespace(ctrl.Namespace)
	if err := ctrl.Delete(ctx, obj); err != nil && !api_errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (ctrl *Controller) prefix() string {
	if ctrl.isClusterAggregator {
		return "cluster-"
	}
	return ""
}

func (ctrl *Controller) globalConfigChanged() bool {
	globalCfgHash := ctrl.Config.GetGlobalConfigHash()
	if ctrl.Status.LastAppliedGlobalConfigHash == nil {
		return false
	}
	return *ctrl.Status.LastAppliedGlobalConfigHash != *globalCfgHash
}
