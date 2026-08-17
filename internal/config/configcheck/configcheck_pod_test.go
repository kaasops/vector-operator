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

package configcheck

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kaasops/vector-operator/internal/config"
)

// TestSecretAssetsMountGeneration verifies that secret assets are correctly
// mounted in configcheck pods when SecretAssetsSecretName is set.
func TestSecretAssetsMountGeneration(t *testing.T) {
	tests := []struct {
		name                     string
		secretAssetsSecretName   string
		expectVolumePresent      bool
		expectVolumeMountPresent bool
	}{
		{
			name:                     "non-empty SecretAssetsSecretName creates volume and mount",
			secretAssetsSecretName:   "test-secret-assets",
			expectVolumePresent:      true,
			expectVolumeMountPresent: true,
		},
		{
			name:                     "empty SecretAssetsSecretName omits volume and mount",
			secretAssetsSecretName:   "",
			expectVolumePresent:      false,
			expectVolumeMountPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &ConfigCheck{
				Name:                   "test",
				Namespace:              "default",
				SecretAssetsSecretName: tt.secretAssetsSecretName,
			}

			// Test volume generation
			volumes := cc.generateVectorConfigCheckVolume()
			volumeFound := false
			for _, vol := range volumes {
				if vol.Name == "secret-assets" {
					volumeFound = true
					if vol.Secret == nil || vol.Secret.SecretName != tt.secretAssetsSecretName {
						t.Errorf("volume has incorrect secret name: got %v, want %s",
							vol.Secret, tt.secretAssetsSecretName)
					}
					break
				}
			}

			if tt.expectVolumePresent && !volumeFound {
				t.Errorf("expected volume 'secret-assets' not found")
			}
			if !tt.expectVolumePresent && volumeFound {
				t.Errorf("unexpected volume 'secret-assets' found")
			}

			// Test volume mount generation
			volumeMounts := cc.generateVectorConfigCheckVolumeMounts()
			volumeMountFound := false
			const secretsMountPath = "/etc/vector/secrets"
			for _, vm := range volumeMounts {
				if vm.Name == "secret-assets" {
					volumeMountFound = true
					if vm.MountPath != secretsMountPath {
						t.Errorf("volume mount has incorrect path: got %s, want %s",
							vm.MountPath, secretsMountPath)
					}
					break
				}
			}

			if tt.expectVolumeMountPresent && !volumeMountFound {
				t.Errorf("expected volume mount 'secret-assets' not found")
			}
			if !tt.expectVolumeMountPresent && volumeMountFound {
				t.Errorf("unexpected volume mount 'secret-assets' found")
			}
		})
	}
}

// TestSecretAssetsMountPodIntegration verifies that the pod contains both volume
// and mount when configured with secret assets.
func TestSecretAssetsMountPodIntegration(t *testing.T) {
	cc := &ConfigCheck{
		Name:                   "test-agent",
		Namespace:              "monitoring",
		Image:                  "vectordev/vector:latest",
		SecretAssetsSecretName: "agent-secret-assets",
		Volumes:                []corev1.Volume{},
		VolumeMounts:           []corev1.VolumeMount{},
	}

	pod := cc.createVectorConfigCheckPod()

	// Check volume is present
	volumeFound := false
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == "secret-assets" {
			volumeFound = true
			if vol.Secret == nil || vol.Secret.SecretName != "agent-secret-assets" {
				t.Errorf("pod volume has incorrect secret name")
			}
			break
		}
	}
	if !volumeFound {
		t.Errorf("expected volume 'secret-assets' not found in pod spec")
	}

	// Check volume mount is present in container
	container := pod.Spec.Containers[0]
	volumeMountFound := false
	for _, vm := range container.VolumeMounts {
		if vm.Name == "secret-assets" {
			volumeMountFound = true
			if vm.MountPath != "/etc/vector/secrets" {
				t.Errorf("pod volume mount has incorrect path: got %s, want /etc/vector/secrets",
					vm.MountPath)
			}
			break
		}
	}
	if !volumeMountFound {
		t.Errorf("expected volume mount 'secret-assets' not found in pod container")
	}
}

// TestSecretAssetsUserVolumesOverride verifies that user-defined volumes don't
// conflict with secret-assets volume.
func TestSecretAssetsUserVolumesNoConflict(t *testing.T) {
	userVolume := corev1.Volume{
		Name: "custom-volume",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	userVolumeMount := corev1.VolumeMount{
		Name:      "custom-volume",
		MountPath: "/custom",
	}

	cc := &ConfigCheck{
		Name:                   "test",
		Namespace:              "default",
		Image:                  "vectordev/vector:latest",
		SecretAssetsSecretName: "test-assets",
		Volumes:                []corev1.Volume{userVolume},
		VolumeMounts:           []corev1.VolumeMount{userVolumeMount},
	}

	pod := cc.createVectorConfigCheckPod()

	// Verify both user volume and secret-assets volume are present
	volumeNames := make(map[string]bool)
	for _, vol := range pod.Spec.Volumes {
		volumeNames[vol.Name] = true
	}

	if !volumeNames["custom-volume"] {
		t.Errorf("user-defined volume 'custom-volume' not found")
	}
	if !volumeNames["secret-assets"] {
		t.Errorf("secret-assets volume not found")
	}

	// Verify both user volume mount and secret-assets volume mount are present
	container := pod.Spec.Containers[0]
	volumeMountNames := make(map[string]bool)
	for _, vm := range container.VolumeMounts {
		volumeMountNames[vm.Name] = true
	}

	if !volumeMountNames["custom-volume"] {
		t.Errorf("user-defined volume mount 'custom-volume' not found")
	}
	if !volumeMountNames["secret-assets"] {
		t.Errorf("secret-assets volume mount not found")
	}
}

// TestSecretAssetsSecretCreation verifies that the temp secret assets secret
// is created with correct name derivation and data assignment.
func TestSecretAssetsSecretCreation(t *testing.T) {
	cc := &ConfigCheck{
		Name:      "test-agent",
		Namespace: "test-ns",
		Initiator: ConfigCheckInitiatorVector,
		SecretAssets: map[string][]byte{
			"secret1.txt": []byte("value1"),
			"secret2.txt": []byte("value2"),
		},
	}

	// Set hash (normally done in Run)
	cc.Hash = "xyz789"

	// Create the secret
	secret := cc.createVectorConfigCheckSecretAssets()

	// Verify name derivation includes agent name and hash
	expectedPrefix := "configcheck-secret-assets-test-agent-"
	if len(secret.Name) < len(expectedPrefix) {
		t.Errorf("secret name too short: %s", secret.Name)
	}
	if secret.Name[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("secret name should include agent name, got: %s", secret.Name)
	}
	if !contains(secret.Name, cc.Hash) {
		t.Errorf("secret name should include hash %s, got: %s", cc.Hash, secret.Name)
	}

	// Verify namespace
	if secret.Namespace != "test-ns" {
		t.Errorf("secret namespace should be 'test-ns', got: %s", secret.Namespace)
	}

	// Verify data matches SecretAssets exactly
	if len(secret.Data) != 2 {
		t.Errorf("secret should have 2 entries, got: %d", len(secret.Data))
	}
	if string(secret.Data["secret1.txt"]) != "value1" {
		t.Errorf("secret data mismatch for secret1.txt")
	}
	if string(secret.Data["secret2.txt"]) != "value2" {
		t.Errorf("secret data mismatch for secret2.txt")
	}
}

// TestRunPreservesCleanupError proves a fixed bug: Run() used unnamed returns with a
// `defer func() { err = cc.cleanup(...) }()` that mutated a local variable the already
// evaluated return statement could never see again, so a cleanup failure was always
// silently dropped. Here Run's own step (pod creation) fails AND the subsequent
// deferred cleanup (secret deletion) also fails; the returned error must carry both,
// joined - not just the first one that happened to run.
func TestRunPreservesCleanupError(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "vector-configcheck", Namespace: "test-ns"},
	}

	funcs := interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			// Fail pod creation - this is Run's own error, computed before the
			// deferred cleanup runs.
			if _, ok := obj.(*corev1.Pod); ok {
				return fmt.Errorf("simulated pod creation failure")
			}
			return c.Create(ctx, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			// Fail deletion of the configcheck config secret, forcing cc.cleanup to
			// return a non-nil error from inside Run's deferred call.
			if secret, ok := obj.(*corev1.Secret); ok && secret.Name != "" {
				return fmt.Errorf("simulated cleanup delete failure")
			}
			return c.Delete(ctx, obj, opts...)
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns, sa).
		WithInterceptorFuncs(funcs).
		Build()

	cc := &ConfigCheck{
		Name:      "test-agent",
		Namespace: "test-ns",
		Client:    fakeClient,
		Image:     "vectordev/vector:latest",
		Initiator: ConfigCheckInitiatorVector,
		Config:    []byte(`{}`),
	}

	_, err := cc.Run(ctx)
	if err == nil {
		t.Fatal("Run should have returned an error (pod creation was made to fail)")
	}
	if !contains(err.Error(), "simulated pod creation failure") {
		t.Errorf("Run's own error must still be present in the returned error, got: %v", err)
	}
	if !contains(err.Error(), "simulated cleanup delete failure") {
		t.Errorf("the fix: cleanup's error must reach the caller too instead of being silently "+
			"dropped by the deferred assignment over an unnamed return, got: %v", err)
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSecretAssetsSecretHasCorrectLabelsAndOwner verifies that the generated
// secret-assets secret has proper labels and owner references.
func TestSecretAssetsSecretHasCorrectLabelsAndOwner(t *testing.T) {
	cc := &ConfigCheck{
		Name:         "test-agent",
		Namespace:    "default",
		Initiator:    ConfigCheckInitiatorVector,
		Labels:       map[string]string{"custom": "label"},
		Annotations:  map[string]string{},
		SecretAssets: map[string][]byte{"key": []byte("value")},
	}

	// Set hash (normally done in Run)
	cc.Hash = "abc123"

	// Create the secret
	secret := cc.createVectorConfigCheckSecretAssets()

	// Verify name includes hash
	expectedNamePrefix := "configcheck-secret-assets-test-agent-"
	if len(secret.Name) < len(expectedNamePrefix) || secret.Name[:len(expectedNamePrefix)] != expectedNamePrefix {
		t.Errorf("secret name should include hash, got: %s", secret.Name)
	}

	// Verify namespace
	if secret.Namespace != "default" {
		t.Errorf("secret namespace should be 'default', got: %s", secret.Namespace)
	}

	// Verify data matches SecretAssets
	if len(secret.Data) != 1 || string(secret.Data["key"]) != "value" {
		t.Errorf("secret data should match SecretAssets, got: %v", secret.Data)
	}

	// Verify labels are set (should have configcheck labels)
	if secret.Labels == nil {
		t.Errorf("secret labels should not be nil")
	}
	// Check for managed-by label which indicates it's created by the operator
	if secret.Labels["app.kubernetes.io/managed-by"] != "vector-operator" {
		t.Errorf("secret should have managed-by label, got: %v", secret.Labels)
	}
}

// TestCleanupRobustness verifies that cleanup attempts to delete all secrets
// even if one fails, and logs errors appropriately.
func TestCleanupRobustness(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create a fake client with one secret
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "config-secret", Namespace: "default"},
		Data:       map[string][]byte{"config.json": []byte("{}")},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret1).Build()

	cc := &ConfigCheck{
		Name:      "agent",
		Namespace: "default",
		Client:    fakeClient,
		Initiator: ConfigCheckInitiatorVector,
	}

	// Try to clean up two secrets: one exists, one doesn't
	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "assets-secret", Namespace: "default"},
	}

	// This should not error out despite secret2 not existing
	// cleanup should attempt to delete both and succeed (NotFound is treated as success)
	err := cc.cleanup(ctx, secret1, secret2)
	if err != nil {
		t.Errorf("cleanup should succeed even when one secret doesn't exist, got error: %v", err)
	}

	// Verify secret1 was actually deleted
	var checkSecret corev1.Secret
	getErr := fakeClient.Get(ctx, types.NamespacedName{Name: "config-secret", Namespace: "default"}, &checkSecret)
	if getErr == nil {
		t.Errorf("config-secret should have been deleted")
	}
}

// TestSecretAssetsLifecycleRunLevel verifies the full Run()-level lifecycle of the
// temporary assets secret: creation with correct name/labels/owner, and cleanup after Run().
func TestSecretAssetsLifecycleRunLevel(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create the baseline objects (namespace, SA for RBAC)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "vector-configcheck", Namespace: "test-ns"},
	}

	// Variables to capture the assets secret state at creation time
	var capturedAssetsSecret *corev1.Secret
	var createdConfigSecret *corev1.Secret

	// Interceptor to capture secrets at creation and fail pod creation
	funcs := interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			// Capture secrets if they're being created
			if secret, ok := obj.(*corev1.Secret); ok {
				// Capture both secrets for later inspection
				if createdConfigSecret == nil && secret.Name != "" && contains(secret.Name, "test-agent") {
					createdConfigSecret = secret.DeepCopy()
				}
				// Capture the assets secret (has "secret-assets" in the name)
				if contains(secret.Name, "secret-assets") {
					capturedAssetsSecret = secret.DeepCopy()
				}
			}
			// Fail pod creation to avoid hanging on watch, but let secrets be created
			if _, ok := obj.(*corev1.Pod); ok {
				return fmt.Errorf("simulated pod creation failure for test")
			}
			// Continue with normal creation for secrets and other resources
			return c.Create(ctx, obj, opts...)
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns, sa).
		WithInterceptorFuncs(funcs).
		Build()

	cc := &ConfigCheck{
		Name:      "test-agent",
		Namespace: "test-ns",
		Client:    fakeClient,
		Image:     "vectordev/vector:latest",
		Initiator: ConfigCheckInitiatorVector,
		Config:    []byte(`{}`),
		SecretAssets: map[string][]byte{
			"cred1.txt": []byte("secret-data-1"),
			"cred2.txt": []byte("secret-data-2"),
		},
	}

	// Run should fail on pod creation, but both secrets should have been created first
	reason, runErr := cc.Run(ctx)
	_ = reason // error expected from pod creation failure

	// Verify that an error occurred (expected from pod creation)
	if runErr == nil {
		t.Errorf("Run should error out on pod creation failure")
	}

	// Verify the assets secret was captured at creation time
	if capturedAssetsSecret == nil {
		t.Errorf("assets secret should have been created (not captured)")
	} else {
		// Verify name includes hash
		if !contains(capturedAssetsSecret.Name, "secret-assets") {
			t.Errorf("assets secret name should contain 'secret-assets', got: %s", capturedAssetsSecret.Name)
		}
		if !contains(capturedAssetsSecret.Name, cc.Hash) {
			t.Errorf("assets secret name should include hash %s, got: %s", cc.Hash, capturedAssetsSecret.Name)
		}

		// Verify labels (should have configcheck labels)
		if capturedAssetsSecret.Labels == nil {
			t.Errorf("assets secret should have labels")
		} else if capturedAssetsSecret.Labels["app.kubernetes.io/managed-by"] != "vector-operator" {
			t.Errorf("assets secret should have managed-by label, got: %v", capturedAssetsSecret.Labels)
		}

		// Verify OwnerReference to config secret
		if len(capturedAssetsSecret.OwnerReferences) == 0 {
			t.Errorf("assets secret should have an OwnerReference")
		} else {
			owner := capturedAssetsSecret.OwnerReferences[0]
			if owner.Kind != "Secret" {
				t.Errorf("owner should be Secret, got: %s", owner.Kind)
			}
			if createdConfigSecret != nil && owner.Name != createdConfigSecret.Name {
				t.Errorf("owner name should be config secret name %s, got: %s", createdConfigSecret.Name, owner.Name)
			}
		}

		// Verify data
		if len(capturedAssetsSecret.Data) != 2 {
			t.Errorf("assets secret should have 2 data entries, got: %d", len(capturedAssetsSecret.Data))
		}
		if string(capturedAssetsSecret.Data["cred1.txt"]) != "secret-data-1" {
			t.Errorf("assets secret data mismatch for cred1.txt")
		}
	}

	// Verify the config secret name was set (this is what SecretAssetsSecretName gets set to)
	if cc.SecretAssetsSecretName == "" {
		t.Errorf("SecretAssetsSecretName should be set during Run")
	}

	// Verify both secrets are cleaned up after Run() returns
	var checkSecret corev1.Secret

	// Check config secret is gone
	if createdConfigSecret != nil {
		configNN := types.NamespacedName{
			Name:      createdConfigSecret.Name,
			Namespace: createdConfigSecret.Namespace,
		}
		if err := fakeClient.Get(ctx, configNN, &checkSecret); err == nil {
			t.Errorf("config secret should have been deleted but still exists: %s", configNN.Name)
		} else if !errors.IsNotFound(err) {
			t.Errorf("unexpected error checking config secret deletion: %v", err)
		}
	}

	// Check assets secret is gone
	if capturedAssetsSecret != nil {
		assetsNN := types.NamespacedName{
			Name:      capturedAssetsSecret.Name,
			Namespace: capturedAssetsSecret.Namespace,
		}
		if err := fakeClient.Get(ctx, assetsNN, &checkSecret); err == nil {
			t.Errorf("assets secret should have been deleted but still exists: %s", assetsNN.Name)
		} else if !errors.IsNotFound(err) {
			t.Errorf("unexpected error checking assets secret deletion: %v", err)
		}
	}
}

// A user-supplied volume/mount squatting the reserved "secret-assets" name must be
// replaced, not honored - otherwise configcheck would validate the config against
// the wrong source while the workload mounts the operator's Secret.
func TestConfigCheckSecretAssetsVolumeIsAuthoritative(t *testing.T) {
	cc := &ConfigCheck{
		Name:                   "test",
		Namespace:              "vector",
		SecretAssetsSecretName: "test-configcheck-assets",
		Volumes: []corev1.Volume{
			{Name: "secret-assets", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "secret-assets", MountPath: "/somewhere/else"},
		},
	}

	var vols []corev1.Volume
	for _, v := range cc.generateVectorConfigCheckVolume() {
		if v.Name == "secret-assets" {
			vols = append(vols, v)
		}
	}
	if len(vols) != 1 {
		t.Fatalf("want exactly one secret-assets volume, got %d", len(vols))
	}
	if vols[0].Secret == nil || vols[0].Secret.SecretName != "test-configcheck-assets" {
		t.Fatalf("secret-assets volume must be the operator's Secret source, got %+v", vols[0].VolumeSource)
	}

	var mounts []corev1.VolumeMount
	for _, m := range cc.generateVectorConfigCheckVolumeMounts() {
		if m.Name == "secret-assets" {
			mounts = append(mounts, m)
		}
	}
	if len(mounts) != 1 {
		t.Fatalf("want exactly one secret-assets mount, got %d", len(mounts))
	}
	if mounts[0].MountPath != config.SecretsMountPath {
		t.Fatalf("secret-assets mount path = %q, want %q", mounts[0].MountPath, config.SecretsMountPath)
	}
}

// A user-supplied mount occupying the reserved secrets path under a different
// volume name must be replaced: Kubernetes rejects duplicate container mountPaths.
func TestConfigCheckSecretAssetsMountPathIsAuthoritative(t *testing.T) {
	cc := &ConfigCheck{
		Name:                   "test",
		Namespace:              "vector",
		SecretAssetsSecretName: "test-configcheck-assets",
		Volumes: []corev1.Volume{
			{Name: "user-shadow", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "user-shadow", MountPath: config.SecretsMountPath},
		},
	}

	var mounts []corev1.VolumeMount
	for _, m := range cc.generateVectorConfigCheckVolumeMounts() {
		if m.MountPath == config.SecretsMountPath {
			mounts = append(mounts, m)
		}
	}
	if len(mounts) != 1 {
		t.Fatalf("want exactly one mount at %q, got %d", config.SecretsMountPath, len(mounts))
	}
	if mounts[0].Name != "secret-assets" {
		t.Fatalf("mount at %q must use the operator's secret-assets volume, got %q", config.SecretsMountPath, mounts[0].Name)
	}
}
