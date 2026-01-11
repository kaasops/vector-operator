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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
)

// Resource names used in namespace validation tests
const (
	nsValidationSecondNamespace      = "test-ns-validation-other"
	nsValidationAggregatorWithSecret = "aggregator-with-secret"
	nsValidationAggregatorNoSecret   = "aggregator-without-secret"
	nsValidationPipeline             = "test-pipeline"
	nsValidationSecret               = "test-credentials"
)

// Namespace Validation tests verify that VectorPipeline is validated only against
// VectorAggregator instances in the SAME namespace, not all aggregators with matching selectors.
//
// This is a regression test for the issue where VectorPipeline validation checked against
// ALL VectorAggregators with matching label selectors, regardless of namespace.
// This caused validation failures when aggregators in other namespaces were missing
// required secrets/resources that only existed in the pipeline's namespace.
//
// Related to issue #201 - the fix for ClusterVectorPipeline selector matching did not
// address the namespace isolation for namespaced VectorPipeline resources.
var _ = Describe("Namespace Validation Isolation", Label(config.LabelRegression), Ordered, func() {
	f := framework.NewUniqueFramework("test-ns-validation")

	BeforeAll(func() {
		f.Setup()

		By("creating second namespace for isolation test")
		f.ApplyTestDataWithoutNamespaceReplacement("namespace-validation/second-namespace.yaml")

		// Give the namespace time to be created
		time.Sleep(2 * time.Second)
	})

	AfterAll(func() {
		By("cleaning up VectorPipeline")
		f.DeleteResource("vectorpipeline", nsValidationPipeline)

		By("cleaning up VectorAggregators")
		f.DeleteResource("vectoraggregator", nsValidationAggregatorWithSecret)
		f.DeleteResourceInNamespace("vectoraggregator", nsValidationAggregatorNoSecret, nsValidationSecondNamespace)

		By("cleaning up secret")
		f.DeleteResource("secret", nsValidationSecret)

		By("cleaning up second namespace")
		f.DeleteClusterResource("namespace", nsValidationSecondNamespace)

		f.Teardown()
		f.PrintMetrics()
	})

	Context("VectorPipeline with VectorAggregators in different namespaces", func() {
		It("should validate VectorPipeline only against VectorAggregator in the same namespace", func() {
			By("creating secret in main namespace")
			f.ApplyTestData("namespace-validation/secret.yaml")

			By("deploying VectorAggregator WITH secret in main namespace")
			f.ApplyTestData("namespace-validation/aggregator-ns1.yaml")

			By("deploying VectorAggregator WITHOUT secret in second namespace")
			// This aggregator references the same secret that does NOT exist in the second namespace
			// If the bug exists, the pipeline will be validated against this aggregator and fail
			f.ApplyTestDataWithoutNamespaceReplacement("namespace-validation/aggregator-ns2.yaml")

			By("waiting for aggregator in main namespace to be ready")
			f.WaitForDeploymentReady(nsValidationAggregatorWithSecret + "-aggregator")

			// Note: The aggregator in the second namespace may not become ready
			// because the secret doesn't exist there, but that's expected and irrelevant
			// for this test. What matters is that the pipeline in namespace 1
			// should NOT be validated against it.

			By("creating VectorPipeline with matching labels in main namespace")
			f.ApplyTestData("namespace-validation/pipeline.yaml")

			By("waiting for VectorPipeline to become valid")
			// If the bug exists:
			//   - Pipeline is validated against BOTH aggregators (both have matching selector)
			//   - Validation against aggregator in second namespace FAILS (missing secret)
			//   - Pipeline status becomes invalid
			//
			// If the bug is fixed:
			//   - Pipeline is validated ONLY against aggregator in the same namespace
			//   - Secret exists in main namespace
			//   - Pipeline status becomes valid
			f.WaitForPipelineValid(nsValidationPipeline)

			By("verifying VectorPipeline has aggregator role")
			role := f.GetPipelineStatus(nsValidationPipeline, "role")
			Expect(role).To(Equal("aggregator"), "Pipeline with vector source should have aggregator role")

			// Note: We don't verify that the aggregator config contains the pipeline here
			// because that's tested in other tests. The key assertion is that validation
			// passes quickly (not timing out waiting for the aggregator in the other namespace).
		})
	})
})
