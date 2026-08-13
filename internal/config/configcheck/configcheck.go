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

package configcheck

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	api_errors "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kaasops/vector-operator/internal/utils/k8s"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

type ConfigCheck struct {
	Config []byte

	Client    client.Client
	ClientSet kubernetes.Interface

	Name                     string
	Namespace                string
	Initiator                string
	Image                    string
	ImagePullPolicy          corev1.PullPolicy
	ImagePullSecrets         []corev1.LocalObjectReference
	Envs                     []corev1.EnvVar
	EnvFrom                  []corev1.EnvFromSource
	Hash                     string
	Tolerations              []corev1.Toleration
	Resources                corev1.ResourceRequirements
	SecurityContext          *corev1.PodSecurityContext
	ContainerSecurityContext *corev1.SecurityContext
	CompressedConfig         bool
	ConfigReloaderImage      string
	ConfigReloaderResources  corev1.ResourceRequirements
	ConfigCheckTimeout       time.Duration
	Annotations              map[string]string
	Labels                   map[string]string
	Volumes                  []corev1.Volume
	VolumeMounts             []corev1.VolumeMount
	SecretAssetsSecretName   string
	SecretAssets             map[string][]byte
}

func New(
	config []byte,
	c client.Client,
	cs kubernetes.Interface,
	vc *vectorv1alpha1.VectorCommon,
	name, namespace string,
	timeout time.Duration,
	initiator string,
	secretAssets map[string][]byte,
) *ConfigCheck {
	image := vc.Image
	if vc.ConfigCheck.Image != nil {
		image = *vc.ConfigCheck.Image
	}

	env := vc.Env

	tolerations := vc.Tolerations
	if vc.ConfigCheck.Tolerations != nil {
		tolerations = *vc.ConfigCheck.Tolerations
	}

	resources := vc.Resources
	if vc.ConfigCheck.Resources != nil {
		resources = *vc.ConfigCheck.Resources
	}

	return &ConfigCheck{
		Config:                   config,
		Client:                   c,
		ClientSet:                cs,
		Name:                     name,
		Namespace:                namespace,
		Image:                    image,
		ImagePullPolicy:          vc.ImagePullPolicy,
		ImagePullSecrets:         vc.ImagePullSecrets,
		Envs:                     env,
		EnvFrom:                  vc.EnvFrom,
		Tolerations:              tolerations,
		Resources:                resources,
		SecurityContext:          vc.SecurityContext,
		ContainerSecurityContext: vc.ContainerSecurityContext,
		CompressedConfig:         vc.CompressConfigFile,
		ConfigReloaderImage:      vc.ConfigReloaderImage,
		ConfigReloaderResources:  vc.ConfigReloaderResources,
		ConfigCheckTimeout:       timeout,
		Annotations:              vc.ConfigCheck.Annotations,
		Labels:                   vc.ConfigCheck.Labels,
		Volumes:                  vc.Volumes,
		VolumeMounts:             vc.VolumeMounts,
		Initiator:                initiator,
		SecretAssets:             secretAssets,
	}
}

// namespaceIsTerminating reports whether the target namespace is being deleted or
// already gone. A ConfigCheck launched into such a namespace can never create its
// pod/secret — the API server rejects new content — so the caller must skip it
// instead of blocking a reconcile worker until ConfigCheckTimeout.
func (cc *ConfigCheck) namespaceIsTerminating(ctx context.Context) (bool, error) {
	return k8s.NamespaceIsTerminating(ctx, cc.Client, cc.Namespace)
}

func (cc *ConfigCheck) Run(ctx context.Context) (reason string, err error) {
	log := log.FromContext(ctx).WithValues("Vector ConfigCheck", cc.Initiator)
	log.Info("================= Started ConfigCheck =================")

	// A terminating namespace cannot accept new content, so the configcheck pod and
	// secret would never be admitted and getCheckResult would block until
	// ConfigCheckTimeout, holding the reconcile worker. Skip instead.
	if terminating, err := cc.namespaceIsTerminating(ctx); err != nil {
		return "", err
	} else if terminating {
		log.Info("Skipping ConfigCheck: namespace is terminating or gone", "namespace", cc.Namespace)
		return "", ErrConfigcheckSkipped
	}

	if err := cc.ensureVectorConfigCheckRBAC(ctx); err != nil && !api_errors.IsAlreadyExists(err) { // TODO(aa1ex): error is silenced, is that ok?
		return "", err
	}

	cc.Hash = randStringRunes()

	vectorConfigCheckSecret, err := cc.createVectorConfigCheckConfig(ctx)
	if err != nil {
		return "", err
	}

	var vectorSecretAssetsSecret *corev1.Secret
	if len(cc.SecretAssets) > 0 {
		vectorSecretAssetsSecret = cc.createVectorConfigCheckSecretAssets()
		cc.SecretAssetsSecretName = vectorSecretAssetsSecret.Name
	}

	vectorConfigCheckPod := cc.createVectorConfigCheckPod()

	// Named returns so this assignment actually reaches the caller: a plain
	// `defer func() { err = ... }()` over unnamed returns only ever mutates a local
	// variable that the already-evaluated return statement has no way to see again.
	defer func() {
		cleanupErr := cc.cleanup(ctx, vectorConfigCheckSecret, vectorSecretAssetsSecret)
		if cleanupErr == nil {
			return
		}
		if err == nil {
			err = cleanupErr
			return
		}
		err = errors.Join(err, cleanupErr)
	}()

	if err = k8s.CreateOrUpdateResource(ctx, vectorConfigCheckSecret, cc.Client); err != nil {
		return "", err
	}

	// Create temporary secret assets secret if needed
	if vectorSecretAssetsSecret != nil {
		if err = controllerutil.SetOwnerReference(vectorConfigCheckSecret, vectorSecretAssetsSecret, cc.Client.Scheme()); err != nil {
			return "", err
		}
		if err = k8s.CreateOrUpdateResource(ctx, vectorSecretAssetsSecret, cc.Client); err != nil {
			return "", err
		}
	}

	// Set OwnerReference to pod
	if err = controllerutil.SetOwnerReference(vectorConfigCheckSecret, vectorConfigCheckPod, cc.Client.Scheme()); err != nil {
		return "", err
	}

	err = k8s.CreatePod(ctx, vectorConfigCheckPod, cc.Client)
	if err != nil {
		return "", err
	}

	reason, err = cc.getCheckResult(ctx, vectorConfigCheckPod)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return reason, err
		}
		return "", err
	}

	return reason, err
}

func (cc *ConfigCheck) ensureVectorConfigCheckRBAC(ctx context.Context) error {
	return cc.ensureVectorConfigCheckServiceAccount(ctx)
}

func (cc *ConfigCheck) ensureVectorConfigCheckServiceAccount(ctx context.Context) error {
	vectorAgentServiceAccount := cc.createVectorConfigCheckServiceAccount()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentServiceAccount, cc.Client)
}

func (cc *ConfigCheck) labelsForVectorConfigCheck() map[string]string {
	basicLabels := map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector-configcheck",
		k8s.ComponentLabelKey: "ConfigCheck",
	}
	labels := k8s.MergeLabels(basicLabels, cc.Labels)

	return labels
}

func (cc *ConfigCheck) annotationsForVectorConfigCheck() map[string]string {
	return cc.Annotations
}

func (cc *ConfigCheck) getNameVectorConfigCheck() string {
	n := "configcheck" + "-" + cc.Name + "-" + cc.Hash

	return n
}

func randStringRunes() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

	b := make([]rune, 5)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// unstartableReason reports a container waiting state that cannot resolve without a
// spec change (missing secret/configmap referenced by env, invalid image name), so
// waiting for the configcheck pod to complete is pointless.
func unstartableReason(pod *corev1.Pod) (string, bool) {
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	for _, st := range statuses {
		w := st.State.Waiting
		if w == nil {
			continue
		}
		switch w.Reason {
		case "CreateContainerConfigError", "InvalidImageName":
			return fmt.Sprintf("configcheck pod cannot start, %s: %s", w.Reason, w.Message), true
		}
	}
	return "", false
}

func (cc *ConfigCheck) getCheckResult(ctx context.Context, pod *corev1.Pod) (reason string, err error) {
	log := log.FromContext(ctx).WithValues("Vector ConfigCheck", pod.Name)
	log.Info("Trying to get configcheck result")

	watcher, err := cc.ClientSet.CoreV1().Pods(cc.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(metav1.ObjectNameField, pod.Name).String(),
		// LabelSelector: labelsForVectorConfigCheck(),
	})

	if err != nil {
		log.Error(err, "cannot create Pod event watcher")
		return "", err
	}

	defer watcher.Stop()

	// Use time.NewTimer instead of time.After to prevent memory leak.
	// time.After() creates a new timer on each select iteration that won't be GC'd
	// until it fires, causing ~280 bytes leak per iteration.
	timer := time.NewTimer(cc.ConfigCheckTimeout)
	defer timer.Stop()

	for {
		select {
		case e, open := <-watcher.ResultChan():
			if !open {
				// We lost sight of the pod; reporting success here would let an
				// unvalidated config through.
				return "", fmt.Errorf("configcheck: watch for pod %s closed before a result", pod.Name)
			}
			pod, ok := e.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			switch e.Type {
			// Added matters too: a resourceVersion-less watch is seeded with synthetic
			// Added events, so a pod that reached a final state before the watch was
			// established would otherwise be ignored until ConfigCheckTimeout.
			case watch.Added, watch.Modified:
				if pod.DeletionTimestamp != nil {
					continue
				}
				switch pod.Status.Phase {
				case corev1.PodSucceeded:
					log.Info("Config Check completed successfully")
					return "", nil
				case corev1.PodFailed:
					log.Info("Config Check Failed")
					reason, err := k8s.GetPodLogs(ctx, pod, cc.ClientSet)
					if err != nil {
						return "", err
					}
					return reason, ErrValidation
				default:
					// A pod that can never start (e.g. env from a missing secret →
					// CreateContainerConfigError) stays Pending forever; fail the check
					// now instead of holding the reconcile worker until timeout.
					if reason, unstartable := unstartableReason(pod); unstartable {
						log.Info("Config Check pod cannot start", "reason", reason)
						return reason, ErrValidation
					}
				}
			case watch.Deleted:
				return "", fmt.Errorf("configcheck: pod %s was deleted before producing a result", pod.Name)
			}
			// Reset timer after processing event to restart timeout window
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(cc.ConfigCheckTimeout)
		case <-ctx.Done():
			return "", nil
		case <-timer.C:
			return "", ErrConfigcheckTimeout
		}
	}
}

// cleanupTimeout bounds the detached cleanup below: long enough for a Delete against a
// healthy API server, short enough that a wedged one cannot hold a shutting-down operator
// open indefinitely.
const cleanupTimeout = 30 * time.Second

// cleanup removes the temporary Secrets a check created - the config one and, when the
// pipelines reference secrets, the assets one holding their plaintext values.
//
// Deletes straight by name, with no Get first. Delete is idempotent and NotFound is
// success, so the read bought nothing - and it could actively lose the objects: the
// manager's client reads through the informer cache, which under --watch-name is
// label-filtered to the operator's own objects, while configcheck objects carry
// app.kubernetes.io/name=vector-configcheck. The Get then returned NotFound for a Secret
// that really exists, cleanup skipped the Delete, and the plaintext copy stayed on the
// cluster until the next operator start swept it.
//
// ctx is detached (context.WithoutCancel) and given its own deadline, because the caller's
// reconcile ctx may already be cancelled by the time the deferred cleanup runs - which is
// exactly when the check was interrupted and its Secrets are most likely to be left
// behind. Deleting through a cancelled context fails every call for the same reason it was
// cancelled, silently keeping the plaintext Secret alive.
func (cc *ConfigCheck) cleanup(ctx context.Context, secrets ...*corev1.Secret) error {
	l := log.FromContext(ctx).WithValues("Vector ConfigCheck", cc.Initiator)

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()

	var errs []error
	for _, secret := range secrets {
		if secret == nil {
			continue
		}
		nn := types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}
		target := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace}}
		// One failure must not stop the rest: the assets Secret carries plaintext
		// credentials and is the one that matters most to remove.
		if err := k8s.DeleteSecret(ctx, target, cc.Client); err != nil && !api_errors.IsNotFound(err) {
			l.Error(err, "failed to delete secret", "secret", nn)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (cc *ConfigCheck) CleanAll(ctx context.Context) error {
	listOpts, err := cc.configCheckListOpts()
	if err != nil {
		return err
	}
	secrets, err := k8s.ListSecret(ctx, cc.Client, listOpts)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		if err := k8s.DeleteSecret(ctx, &secret, cc.Client); err != nil {
			return err
		}
	}
	return nil
}

func (cc *ConfigCheck) configCheckListOpts() (client.ListOptions, error) {
	configCheckLabels := cc.labelsForVectorConfigCheck()
	var requirements []labels.Requirement
	for k, v := range configCheckLabels {
		r, err := labels.NewRequirement(k, "==", []string{v})
		if err != nil {
			return client.ListOptions{}, err
		}
		requirements = append(requirements, *r)
	}
	labelsSelector := labels.NewSelector().Add(requirements...)

	return client.ListOptions{
		LabelSelector: labelsSelector,
		Namespace:     cc.Namespace,
	}, nil
}

// orphanSweepLabels is the label set every configcheck Secret carries
// (labelsForVectorConfigCheck's basic labels, before per-instance additions), used by
// SweepOrphans to find leftovers from a previous operator process.
func orphanSweepLabels() labels.Selector {
	return labels.SelectorFromSet(map[string]string{
		k8s.ManagedByLabelKey: "vector-operator",
		k8s.NameLabelKey:      "vector-configcheck",
		k8s.ComponentLabelKey: "ConfigCheck",
	})
}

// SweepOrphans deletes configcheck Secrets left behind by a previous operator
// process. Nothing owns the root configcheck Secret (the pod is owned by it, not the
// other way around), so a crash between creating it and the process-local deferred
// cleanup strands it - and with pipeline secrets in play the orphaned
// configcheck-secret-assets child holds a plaintext copy of referenced credentials.
// Only Secrets created before startedBefore (the current process start) are swept, so
// configchecks racing with the sweep are never touched; the orphaned pod, if any,
// follows its root Secret via ownerRef GC.
func SweepOrphans(ctx context.Context, reader client.Reader, c client.Client, startedBefore time.Time) error {
	var list corev1.SecretList
	if err := reader.List(ctx, &list, &client.ListOptions{LabelSelector: orphanSweepLabels()}); err != nil {
		return err
	}
	var errs []error
	for i := range list.Items {
		secret := &list.Items[i]
		if !secret.CreationTimestamp.Time.Before(startedBefore) {
			continue
		}
		if err := c.Delete(ctx, secret); err != nil && !api_errors.IsNotFound(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
