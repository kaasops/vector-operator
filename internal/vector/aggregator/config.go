package aggregator

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/compression"
	"github.com/kaasops/vector-operator/internal/utils/k8s"
)

func (ctrl *Controller) ensureVectorAggregatorConfig(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-secret", ctrl.Name)
	log.Info("start Reconcile Vector Aggregator Secret")

	vectorAggregatorSecret, err := ctrl.createVectorAggregatorConfig(ctx)
	if err != nil {
		return err
	}

	return k8s.CreateOrUpdateResource(ctx, vectorAggregatorSecret, ctrl.Client)
}

func (ctrl *Controller) createVectorAggregatorConfig(ctx context.Context) (*corev1.Secret, error) {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-config", ctrl.Name)
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()
	data := ctrl.ConfigBytes

	if ctrl.Spec.CompressConfigFile {
		data = compression.Compress(ctrl.ConfigBytes, log)
	}
	config := map[string][]byte{
		"config.json": data,
	}
	secret := &corev1.Secret{
		ObjectMeta: ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace),
		Data:       config,
	}
	return secret, nil
}

// secretAssetsSafetyReader returns the uncached reader every secret-assets safeguard has
// to read through. Three decisions depend on it - the write-order gate, the bridge plan,
// and the prune gate - and all three are safeguards, so none of them may be decided by
// how far behind the informer cache happens to be: a stale snapshot that understates
// what the assets Secret currently holds makes a prune look free and drops a key a live
// config still references.
//
// Deliberately not defaulted to ctrl.Client: a caller that forgets to set APIReader
// fails loudly here instead of silently going back to reading the cache.
func (ctrl *Controller) secretAssetsSafetyReader(decision string) (client.Reader, error) {
	if ctrl.APIReader == nil {
		return nil, fmt.Errorf("APIReader is not set: %s must not be decided from the cache", decision)
	}
	return ctrl.APIReader, nil
}

// PublishedConfigMatches reports whether the config Secret currently on the cluster
// already holds exactly byteConfig (compressed the same way this round would, if
// CompressConfigFile is enabled) - a direct byte comparison against the actually
// deployed artifact, not a hash. A 32-bit CRC32 (internal/utils/hash, originally only
// a configcheck-skip optimization where an occasional false "changed" just cost one
// extra validation pod) is not safe enough now that the identical signal also gates
// whether it is safe to prune the assets Secret: two distinct generated configs have
// been observed sharing a CRC32 (10648681) on a real snapshot, and a
// false "unchanged" there means pruning a key a live, different config still
// references. Byte comparison cannot collide - two different configs can never
// compare equal. Absent (not yet created) counts as "not published", i.e. changed -
// there is nothing live yet to safely prune against.
func (ctrl *Controller) PublishedConfigMatches(ctx context.Context, byteConfig []byte) (bool, error) {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-config", ctrl.Name)
	reader, err := ctrl.secretAssetsSafetyReader("the secret-assets prune gate")
	if err != nil {
		return false, err
	}
	secret := &corev1.Secret{}
	err = reader.Get(ctx, client.ObjectKey{Namespace: ctrl.Namespace, Name: ctrl.getNameVectorAggregator()}, secret)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	want := byteConfig
	if ctrl.Spec.CompressConfigFile {
		want = compression.Compress(byteConfig, log)
	}
	return bytes.Equal(secret.Data["config.json"], want), nil
}

// ExistingSecretAssets reads the assets Secret's CURRENT Data - empty (not an
// error) if it does not exist yet, the common case (no pipeline references a secret
// yet, or this is the workload's first reconcile). The caller (createOrUpdateVectorAggregator)
// uses this to compute a safe bridge round via config.BridgeAssets/planSecretAssetsBridge
// before ever building the config this reconcile will publish - see the agent's
// identical ExistingSecretAssets for the full rationale.
func (ctrl *Controller) ExistingSecretAssets(ctx context.Context) (map[string][]byte, error) {
	reader, err := ctrl.secretAssetsSafetyReader("the secret-assets bridge and prune plan")
	if err != nil {
		return nil, err
	}
	secret := &corev1.Secret{}
	err = reader.Get(ctx, client.ObjectKey{Namespace: ctrl.Namespace, Name: ctrl.getSecretAssetsName()}, secret)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return map[string][]byte{}, nil
		}
		return nil, err
	}
	existing := make(map[string][]byte, len(secret.Data))
	for k, v := range secret.Data {
		existing[k] = v
	}
	return existing, nil
}

// ensureVectorAggregatorSecretAssets keeps the secret-assets Secret in sync with
// ctrl.SecretAssets: replaces its Data outright with whatever the caller put there.
// Empty (the common case: no pipeline references a secret) means no Secret is
// created, and any leftover from a previous non-empty state is deleted so removing
// the last secret reference cleans up after itself.
//
// Called before ensureVectorAggregatorConfig - see the agent's identical
// ensureVectorAgentSecretAssets for the full rationale (assets-before-config for
// anything added: a config referencing a not-yet-mounted key crash-loops the pod on
// its next restart, for every pipeline sharing the workload). This function itself
// no longer decides whether to write the full union or the exact target - that
// decision, and the read of the Secret's current contents it depends on, happens
// upstream in createOrUpdateVectorAggregator via
// ExistingSecretAssets/planSecretAssetsBridge, before ctrl.SecretAssets is even set.
func (ctrl *Controller) ensureVectorAggregatorSecretAssets(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues(ctrl.prefix()+"vector-aggregator-secret-assets", ctrl.Name)

	if len(ctrl.SecretAssets) == 0 {
		return ctrl.deleteSecretAssetsSecret(ctx)
	}

	log.Info("start Reconcile Vector Aggregator Secret Assets")
	secret := ctrl.createSecretAssetsSecret()
	return k8s.CreateOrUpdateResource(ctx, secret, ctrl.Client)
}

// createSecretAssetsSecret builds the Secret that materializes ctrl.SecretAssets
// (resolved pipeline secret data) for mounting at config.SecretsMountPath. Uses the
// same ownerRef/labels mechanism as the config Secret.
func (ctrl *Controller) createSecretAssetsSecret() *corev1.Secret {
	labels := ctrl.labelsForVectorAggregator()
	annotations := ctrl.annotationsForVectorAggregator()
	meta := ctrl.objectMetaVectorAggregator(labels, annotations, ctrl.Namespace)
	meta.Name = ctrl.getSecretAssetsName()

	return &corev1.Secret{
		ObjectMeta: meta,
		Data:       ctrl.SecretAssets,
	}
}

// deleteSecretAssetsSecret best-effort removes the secret-assets Secret by name.
func (ctrl *Controller) deleteSecretAssetsSecret(ctx context.Context) error {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: ctrl.getSecretAssetsName(), Namespace: ctrl.Namespace}}
	if err := ctrl.Delete(ctx, secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	return nil
}
