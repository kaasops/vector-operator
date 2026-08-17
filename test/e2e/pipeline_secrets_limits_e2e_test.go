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

package e2e

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
	"github.com/kaasops/vector-operator/test/e2e/framework/kubectl"
)

const (
	// limitsSecretValueSize matches the live cross-tenant overflow repro and
	// internal/controller's own unit tests (secretSizeExclusionReasonPrefix
	// suite): two values that are each individually well under
	// corev1.MaxSecretSize (1 048 576) but overflow it combined.
	limitsSecretValueSize = 600000

	// limitsBridgeTimeout has to cover, in sequence: the round that discovers the
	// overflow and excludes/waits the affected pipeline, the round(s) that confirm
	// the surviving config unchanged, SecretAssetsPruneGracePeriod itself (90s,
	// see internal/controller/secrets.go) once a stale key actually needs pruning
	// before the freed room can be reused, and the final admitting round - plus
	// margin for configcheck pods (Context A only - Context B disables it) and
	// kubelet Secret sync on top of all of that.
	limitsBridgeTimeout = 6 * time.Minute
	limitsBridgePoll    = 5 * time.Second
)

// bigSecretYAML renders a single-key Opaque Secret manifest whose value is `size`
// bytes, filled with `filler` repeated - used instead of a static testdata file
// because the fixture only needs to be big, not meaningful, and checking a ~600 KB
// literal into the repo for every variant this scenario needs would be worse than
// generating it here. An explicit namespace is embedded so the same helper works
// for the cross-tenant victim, which deliberately lives in a SEPARATE namespace -
// see applyBigObject, which is how every caller of this actually applies it.
func bigSecretYAML(namespace, name, key string, filler byte, size int) string {
	value := strings.Repeat(string(filler), size)
	return fmt.Sprintf("apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n  %s: %q\n",
		name, namespace, key, value)
}

// smallSecretYAML renders a single-key Opaque Secret manifest with an ordinary,
// short value - used to shrink a previously oversized Secret back under the limit.
func smallSecretYAML(namespace, name, key, value string) string {
	return fmt.Sprintf("apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n  %s: %q\n",
		name, namespace, key, value)
}

// victimPipelineYAML renders the cross-tenant victim's VectorPipeline manifest with
// an explicit namespace - generated in Go rather than a testdata/ fixture for the
// same reason as bigSecretYAML: ApplyTestData's automatic namespace substitution
// always targets f.Namespace(), and this pipeline deliberately does not live there.
// Carries the same e2e label as pipeline-survivor.yaml so vlimits' selector (see
// agent.yaml) picks it up despite living in a different namespace.
func victimPipelineYAML(namespace, name string) string {
	return fmt.Sprintf(`apiVersion: observability.kaasops.io/v1alpha1
kind: VectorPipeline
metadata:
  name: %s
  namespace: %s
  labels:
    e2e: pipeline-secrets-limits-crosstenant
spec:
  secret:
    es:
      type: kubernetes_secret
      name: victim-creds
  sources:
    logsVictim:
      type: kubernetes_logs
  sinks:
    outVictim:
      type: elasticsearch
      inputs:
        - logsVictim
      endpoints:
        - "http://elasticsearch.example.com:9200"
      healthcheck:
        enabled: false
      auth:
        strategy: basic
        user: "SECRET[es.cert]"
        password: "SECRET[es.cert]"
`, name, namespace)
}

// applyToNamespace applies a manifest that already carries its own metadata.namespace
// field, without a Framework instance forcing its own namespace onto it - the
// mechanism the cross-tenant victim PIPELINE (a small object) relies on. The big
// Secrets below deliberately do NOT go through this - see applyBigObject.
func applyToNamespace(yamlContent string) {
	Expect(kubectl.NewClient("").ApplyWithoutNamespaceOverride(yamlContent)).To(Succeed())
}

// applyBigObject creates or replaces an object too large for `kubectl apply`:
// client-side apply always writes the object's full previous config into the
// kubectl.kubernetes.io/last-applied-configuration annotation, and Kubernetes caps
// the total size of an object's annotations at 262144 bytes - independently of, and
// below, corev1.MaxSecretSize (1 048 576) itself, so a single ~600 KB Secret value
// blows straight through the annotation limit on the very first `kubectl apply`,
// before this suite's own scenario ever gets to run. (The shared framework's
// ApplyYAML is not touched for this - it is relied on by every other e2e spec, and
// changing its semantics for the sake of this one suite is not worth the risk.)
//
// Deleting first (best-effort, a missing object is fine) and then creating fresh
// sidesteps client-side apply entirely and is also simpler than reasoning about
// server-side apply's per-field merge semantics for whether replacing data/
// stringData actually drops a key that is no longer in the new manifest (relevant
// here: the swap scenario replaces one 600 KB key with another under the same
// Secret name) - the object is always exactly, and only, what this manifest says.
func applyBigObject(namespace, kind, name, yamlContent string) {
	_ = exec.Command("kubectl", "delete", kind, name, "-n", namespace, "--ignore-not-found").Run()
	cmd := exec.Command("kubectl", "create", "-f", "-")
	cmd.Stdin = strings.NewReader(yamlContent)
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "kubectl create failed for %s/%s: %s", namespace, name, string(output))
}

// secretRefPattern matches a rewritten SECRET[k8s.<flat>] reference in a published
// Vector config, mirroring the rewrite internal/config/secrets.go performs.
var secretRefPattern = regexp.MustCompile(`SECRET\[k8s\.([A-Za-z0-9_]+)\]`)

// assertConfigOnlyReferencesMountedKeys is the phase-by-phase counterpart to the
// final-state-only ContainSubstring/HaveKey assertions elsewhere in this suite: it
// parses every SECRET[k8s.<flat>] reference out of the agent's CURRENTLY PUBLISHED
// config and fails unless each one is already a key in the CURRENTLY MOUNTED assets
// Secret. This is the exact invariant the write ordering and the prune grace period
// exist to hold at EVERY phase of a transition, not only once everything has
// settled - so it is called at each checkpoint of the scenarios below, including
// mid-transition, not just at the end.
func assertConfigOnlyReferencesMountedKeys(f *framework.Framework, vectorName string) {
	cfg, err := f.GetSecret(vectorName + "-agent")
	Expect(err).NotTo(HaveOccurred())
	assets, err := f.GetSecret(vectorName + "-agent-secret-assets")
	if err != nil {
		assets = map[string][]byte{}
	}
	for _, match := range secretRefPattern.FindAllStringSubmatch(string(cfg["agent.json"]), -1) {
		flat := match[1]
		Expect(assets).To(HaveKey(flat),
			fmt.Sprintf("the published config references SECRET[k8s.%s] but the currently mounted secret-assets Secret does not have it - "+
				"this is exactly the missing-key state that crash-loops a pod restarting at this instant", flat))
	}
}

// restartAgentPodAndVerifyClean force-deletes one currently running vectorName agent
// pod (simulating an eviction, a node drain, or a new node scheduling a replacement)
// and asserts the DaemonSet-scheduled replacement becomes Ready with zero restarts.
// This is the core assertion this whole suite exists for: a config on disk
// referencing a secret-assets key the mounted Secret does not have makes Vector's
// directory backend fail to resolve it on startup, and the pod never becomes Ready -
// either CrashLoopBackOff (a config error) or stuck in
// ContainerCreating with a FailedMount event (a missing volume source).
// WaitForPodReady's own timeout is the proof: it does not need to distinguish
// between those two failure modes, either one fails this call.
func restartAgentPodAndVerifyClean(f *framework.Framework, vectorName string) {
	podsBefore, err := f.GetAgentPods(vectorName)
	Expect(err).NotTo(HaveOccurred())
	Expect(podsBefore).NotTo(BeEmpty())
	targetPod := podsBefore[0]

	By(fmt.Sprintf("deleting pod %s, simulating an eviction, node drain, or new node", targetPod))
	f.DeleteResource("pod", targetPod)

	By("waiting for the DaemonSet to schedule a replacement")
	var replacementPod string
	Eventually(func() (string, error) {
		pods, err := f.GetAgentPods(vectorName)
		if err != nil {
			return "", err
		}
		for _, p := range pods {
			if p != targetPod {
				replacementPod = p
				return p, nil
			}
		}
		return "", fmt.Errorf("replacement for %s not scheduled yet", targetPod)
	}, config.PipelineValidTimeout, config.DefaultPollInterval).ShouldNot(BeEmpty())

	f.WaitForPodReady(replacementPod)

	restartCount, err := f.Kubectl().GetWithJsonPath("pod", replacementPod, ".status.containerStatuses[*].restartCount")
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.TrimSpace(restartCount)).To(Equal("0"),
		"the agent container must come up clean on the very first try - any restart here means it crashed before settling")
}

// Pipeline Secrets Limits is the live counterpart to
// internal/controller/vector_controller_secret_size_test.go,
// vector_controller_secret_assets_deferred_prune_test.go, and the alt-lag/
// template-order fault-injection suites: those envtest suites prove the
// attribution, deferred-prune, and write-ordering logic with either no kubelet at
// all or a synthetic injected failure. This suite runs the same class of scenario
// against a real kind cluster with a real kubelet, and restarts the agent pod WHILE
// a transition is actually in flight - not once everything has already settled -
// which is the specific window a stale-mount crash-loop can slip through.
var _ = Describe("Pipeline Secrets Limits", Label(config.LabelRegression, config.LabelSlow), Ordered, func() {
	f := framework.NewUniqueFramework("test-pipeline-secrets-limits")

	const (
		vectorName    = "vlimits"
		survivorName  = "limits-survivor"
		victimName    = "limits-victim"
		survivorAlias = "es"
		victimAlias   = "es"
		certKey       = "cert"

		swapVectorName = "vlimits-swap"
		swapName       = "limits-swap"
		swapAlias      = "es"
		oldKey         = "oldval"
		newKey         = "newval"
	)

	// victimNS is a SEPARATE namespace from f.Namespace(): the victim pipeline
	// belongs to a different tenant than the survivor, and the whole point of
	// this suite is that one tenant's secret data can freeze or crash-loop
	// another's workload despite neither namespace having any access to the
	// other's Secrets. Putting both pipelines in the same namespace (as an earlier
	// version of this suite did) tests the size-attribution logic but not the
	// cross-tenant blast radius the bug report is actually about.
	var victimNS string

	BeforeAll(func() {
		f.Setup()
		victimNS = f.Namespace() + "-victim"
		Expect(kubectl.CreateNamespace(victimNS)).To(Succeed())
	})

	AfterAll(func() {
		_ = kubectl.DeleteNamespace(victimNS, config.NamespaceDeleteTimeout.String())
		f.Teardown()
		f.PrintMetrics()
	})

	Context("cross-tenant secret-assets size overflow", func() {
		It("excludes only the younger pipeline when the combined values overflow the shared limit, keeping the older one intact", func() {
			By("creating the agent and the older (survivor) pipeline with its own individually-valid secret, in the framework's own namespace")
			f.ApplyTestData("pipeline-secrets-limits/agent.yaml")
			applyBigObject(f.Namespace(), "secret", "survivor-creds", bigSecretYAML(f.Namespace(), "survivor-creds", certKey, 'a', limitsSecretValueSize))
			f.ApplyTestData("pipeline-secrets-limits/pipeline-survivor.yaml")
			f.WaitForPipelineValid(survivorName)

			survivorFlat := flatSecretAssetKey(f.Namespace(), survivorName, survivorAlias, certKey)
			assetsSecret := vectorName + "-agent-secret-assets"

			Eventually(func() (map[string][]byte, error) {
				return f.GetSecret(assetsSecret)
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(HaveKey(survivorFlat))
			assertConfigOnlyReferencesMountedKeys(f, vectorName)

			// CreationTimestamp is second-precision in the Kubernetes API, so the two
			// pipelines need a real gap for attribution ("oldest survives") to be
			// unambiguous - see the identical wait in
			// vector_controller_secret_size_test.go.
			time.Sleep(1100 * time.Millisecond)

			By(fmt.Sprintf("creating the younger (victim) pipeline in a DIFFERENT tenant namespace (%s) - individually valid, but combined with the survivor it overflows corev1.MaxSecretSize", victimNS))
			applyBigObject(victimNS, "secret", "victim-creds", bigSecretYAML(victimNS, "victim-creds", certKey, 'b', limitsSecretValueSize))
			applyToNamespace(victimPipelineYAML(victimNS, victimName))

			By("verifying only the victim is excluded, with a reason naming the Kubernetes Secret limit")
			victimKubectl := kubectl.NewClient(victimNS)
			Eventually(func() (string, error) {
				return victimKubectl.GetWithJsonPath("vectorpipeline", victimName, ".status.configCheckResult")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Equal("false"))
			victimReason, err := victimKubectl.GetWithJsonPath("vectorpipeline", victimName, ".status.reason")
			Expect(err).NotTo(HaveOccurred())
			Expect(victimReason).To(ContainSubstring("secret assets size limit"))
			Expect(victimReason).To(ContainSubstring("1048576"), "the reason must name the Kubernetes Secret limit")

			By("verifying the survivor is unaffected")
			Expect(f.GetPipelineStatus(survivorName, "configCheckResult")).To(Equal("true"))
			Expect(f.GetPipelineStatus(survivorName, "reason")).To(BeEmpty())

			victimFlat := flatSecretAssetKey(victimNS, victimName, victimAlias, certKey)

			By("verifying the victim's value is absent from both the agent config and the mounted secret-assets Secret")
			Expect(f.VerifyAgentConfigNotContains(vectorName, victimFlat)).To(Succeed())
			assets, err := f.GetSecret(assetsSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(assets).To(HaveKey(survivorFlat))
			Expect(assets).NotTo(HaveKey(victimFlat))
			assertConfigOnlyReferencesMountedKeys(f, vectorName)
		})

		It("keeps accepting spec changes for the surviving pipeline while the excluded tenant waits", func() {
			By("editing the survivor pipeline's source selector")
			f.ApplyTestData("pipeline-secrets-limits/pipeline-survivor-updated.yaml")

			By("verifying the change reaches the agent config")
			Eventually(func() error {
				return f.VerifyAgentConfigContains(vectorName, "survivor-update-marker")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying the survivor is still valid and the victim tenant is still excluded")
			Expect(f.GetPipelineStatus(survivorName, "configCheckResult")).To(Equal("true"))
			victimResult, err := kubectl.NewClient(victimNS).GetWithJsonPath("vectorpipeline", victimName, ".status.configCheckResult")
			Expect(err).NotTo(HaveOccurred())
			Expect(victimResult).To(Equal("false"))
			assertConfigOnlyReferencesMountedKeys(f, vectorName)
		})

		It("does not crash-loop or fail to mount when the agent pod is forcibly restarted WHILE the victim tenant is mid-recovery", func() {
			By("shrinking the victim's backing Secret well under the limit - this starts the pipeline's transition back into the shared config")
			applyBigObject(victimNS, "secret", "victim-creds", smallSecretYAML(victimNS, "victim-creds", certKey, "victim-shrunk"))

			// The restart happens HERE, immediately after triggering the transition
			// and BEFORE waiting for it to finish converging - inside the window a
			// stale reference could actually reach a live pod's disk, not in an
			// already-settled state (which was never at risk and would barely test
			// anything).
			By("restarting the agent pod immediately, before the reinstatement has had a chance to fully converge")
			restartAgentPodAndVerifyClean(f, vectorName)
			assertConfigOnlyReferencesMountedKeys(f, vectorName)

			By("verifying the victim tenant still becomes valid again with no manual intervention beyond the Secret edit and the forced restart")
			Eventually(func() (string, error) {
				return kubectl.NewClient(victimNS).GetWithJsonPath("vectorpipeline", victimName, ".status.configCheckResult")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Equal("true"))
			victimReasonAfter, err := kubectl.NewClient(victimNS).GetWithJsonPath("vectorpipeline", victimName, ".status.reason")
			Expect(err).NotTo(HaveOccurred())
			Expect(victimReasonAfter).To(BeEmpty())

			victimFlat := flatSecretAssetKey(victimNS, victimName, victimAlias, certKey)
			assetsSecret := vectorName + "-agent-secret-assets"

			By("verifying the shrunk value now appears in both the agent config and the mounted secret-assets Secret")
			Eventually(func() error {
				return f.VerifyAgentConfigContains(vectorName, victimFlat)
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(Succeed())
			Eventually(func() (map[string][]byte, error) {
				return f.GetSecret(assetsSecret)
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(HaveKeyWithValue(victimFlat, []byte("victim-shrunk")))
			assertConfigOnlyReferencesMountedKeys(f, vectorName)
		})
	})

	Context("bridge convergence with configCheck disabled, restarting mid-window", func() {
		It("admits the pipeline with its initial value", func() {
			By("creating a dedicated agent (its own, empty secret-assets budget, configCheck disabled) and the pipeline's initial secret")
			f.ApplyTestData("pipeline-secrets-limits/agent-swap.yaml")
			applyBigObject(f.Namespace(), "secret", "swap-creds", bigSecretYAML(f.Namespace(), "swap-creds", oldKey, 'o', limitsSecretValueSize))
			f.ApplyTestData("pipeline-secrets-limits/pipeline-swap.yaml")
			f.WaitForPipelineValid(swapName)

			oldFlat := flatSecretAssetKey(f.Namespace(), swapName, swapAlias, oldKey)
			assetsSecret := swapVectorName + "-agent-secret-assets"
			Eventually(func() (map[string][]byte, error) {
				return f.GetSecret(assetsSecret)
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(HaveKey(oldFlat))
			assertConfigOnlyReferencesMountedKeys(f, swapVectorName)
		})

		It("converges to the new value across a few reconciles, restarting the agent pod mid-window, and never publishing a config that references a missing key", func() {
			oldFlat := flatSecretAssetKey(f.Namespace(), swapName, swapAlias, oldKey)
			newFlat := flatSecretAssetKey(f.Namespace(), swapName, swapAlias, newKey)
			assetsSecret := swapVectorName + "-agent-secret-assets"

			By("replacing the backing Secret's key and pointing the pipeline at the new one - old and new individually fit, together they do not")
			applyBigObject(f.Namespace(), "secret", "swap-creds", bigSecretYAML(f.Namespace(), "swap-creds", newKey, 'n', limitsSecretValueSize))
			f.ApplyTestData("pipeline-secrets-limits/pipeline-swap-newkey.yaml")

			By("verifying the pipeline is held back waiting for room rather than failed outright, and the stale key is not dropped prematurely")
			Eventually(func() string {
				return f.GetPipelineStatus(swapName, "reason")
			}, config.PipelineValidTimeout, config.DefaultPollInterval).Should(ContainSubstring("secret assets waiting for room"))
			assets, err := f.GetSecret(assetsSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(assets).To(HaveKey(oldFlat), "the old key must not be pruned in the same round the config that stopped referencing it was published")

			By("verifying the published config never references the new key before the old one has actually been freed")
			Expect(f.VerifyAgentConfigNotContains(swapVectorName, newFlat)).To(Succeed())
			assertConfigOnlyReferencesMountedKeys(f, swapVectorName)

			// This is the window a stale reference could actually reach a live
			// pod's disk: the config has already changed (the swap pipeline is
			// excluded from this round's build entirely, per the bridge), the old
			// key has NOT been pruned yet, and configCheck is disabled for this
			// agent - the exact combination that lets a stale reference reach a
			// live pod's disk uncaught. Restarting HERE, not after convergence, is
			// the point.
			By("restarting the agent pod WHILE the transition is still in flight, before any pruning has happened")
			restartAgentPodAndVerifyClean(f, swapVectorName)
			assertConfigOnlyReferencesMountedKeys(f, swapVectorName)

			By("waiting for convergence: SecretAssetsPruneGracePeriod elapses, the old key is pruned, and the pipeline is then readmitted with the new one")
			Eventually(func() (map[string][]byte, error) {
				return f.GetSecret(assetsSecret)
			}, limitsBridgeTimeout, limitsBridgePoll).Should(SatisfyAll(
				HaveKey(newFlat),
				Not(HaveKey(oldFlat)),
			))
			Eventually(func() string {
				return f.GetPipelineStatus(swapName, "configCheckResult")
			}, limitsBridgeTimeout, limitsBridgePoll).Should(Equal("true"))
			Expect(f.GetPipelineStatus(swapName, "reason")).To(BeEmpty())

			By("verifying the converged config references only the new key")
			Expect(f.VerifyAgentConfigContains(swapVectorName, newFlat)).To(Succeed())
			Expect(f.VerifyAgentConfigNotContains(swapVectorName, oldFlat)).To(Succeed())
			assertConfigOnlyReferencesMountedKeys(f, swapVectorName)
		})
	})
})
