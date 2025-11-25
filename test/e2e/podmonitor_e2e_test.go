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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaasops/vector-operator/test/e2e/framework"
	"github.com/kaasops/vector-operator/test/e2e/framework/config"
)

// PodMonitor tests verify the PodMonitor creation and configuration
// including scrapeInterval and scrapeTimeout settings
var _ = Describe("PodMonitor Configuration", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-podmonitor")

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	Context("Agent PodMonitor with custom scrape settings", func() {
		It("should create PodMonitor with scrapeInterval and scrapeTimeout", func() {
			By("deploying Vector Agent with custom scrape settings")
			f.ApplyTestData("podmonitor/agent-with-scrape-config.yaml")

			// Wait for agent resources to be created
			time.Sleep(5 * time.Second)

			By("verifying PodMonitor is created")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-agent-agent")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying scrapeInterval is set correctly")
			interval, err := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-agent-agent")
			Expect(err).NotTo(HaveOccurred())
			Expect(interval).To(Equal("45s"), "scrapeInterval should be 45s")

			By("verifying scrapeTimeout is set correctly")
			timeout, err := getPodMonitorScrapeTimeout(f.Namespace(), "podmonitor-agent-agent")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(Equal("15s"), "scrapeTimeout should be 15s")
		})

		It("should create PodMonitor with default settings when not specified", func() {
			By("deploying Vector Agent with default settings")
			f.ApplyTestData("podmonitor/agent-with-defaults.yaml")

			// Wait for agent resources to be created
			time.Sleep(5 * time.Second)

			By("verifying PodMonitor is created")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-agent-defaults-agent")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying scrapeInterval is not set (uses Prometheus default)")
			interval, err := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-agent-defaults-agent")
			Expect(err).NotTo(HaveOccurred())
			Expect(interval).To(BeEmpty(), "scrapeInterval should be empty when not specified")

			By("verifying scrapeTimeout is not set (uses Prometheus default)")
			timeout, err := getPodMonitorScrapeTimeout(f.Namespace(), "podmonitor-agent-defaults-agent")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(BeEmpty(), "scrapeTimeout should be empty when not specified")
		})

		It("should NOT create PodMonitor when internalMetrics is disabled", func() {
			By("deploying Vector Agent with internalMetrics disabled")
			f.ApplyTestData("podmonitor/agent-no-metrics.yaml")

			// Wait for agent resources to be created
			time.Sleep(5 * time.Second)

			By("verifying PodMonitor is NOT created")
			Consistently(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-agent-no-metrics-agent")
			}, 10*time.Second, time.Second).Should(HaveOccurred(), "PodMonitor should not exist when internalMetrics is disabled")
		})
	})

	Context("Aggregator PodMonitor with custom scrape settings", func() {
		It("should create PodMonitor with scrapeInterval and scrapeTimeout", func() {
			By("deploying VectorAggregator with custom scrape settings")
			f.ApplyTestData("podmonitor/aggregator-with-scrape-config.yaml")
			f.WaitForDeploymentReady("podmonitor-aggregator-aggregator")

			By("verifying PodMonitor is created")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-aggregator-aggregator")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying scrapeInterval is set correctly")
			interval, err := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-aggregator-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(interval).To(Equal("60s"), "scrapeInterval should be 60s")

			By("verifying scrapeTimeout is set correctly")
			timeout, err := getPodMonitorScrapeTimeout(f.Namespace(), "podmonitor-aggregator-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(Equal("20s"), "scrapeTimeout should be 20s")
		})

		It("should create PodMonitor with default settings when not specified", func() {
			By("deploying VectorAggregator with default settings")
			f.ApplyTestData("podmonitor/aggregator-with-defaults.yaml")
			f.WaitForDeploymentReady("podmonitor-aggregator-defaults-aggregator")

			By("verifying PodMonitor is created")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-aggregator-defaults-aggregator")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying scrapeInterval is not set (uses Prometheus default)")
			interval, err := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-aggregator-defaults-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(interval).To(BeEmpty(), "scrapeInterval should be empty when not specified")

			By("verifying scrapeTimeout is not set (uses Prometheus default)")
			timeout, err := getPodMonitorScrapeTimeout(f.Namespace(), "podmonitor-aggregator-defaults-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(BeEmpty(), "scrapeTimeout should be empty when not specified")
		})
	})

	Context("PodMonitor label selectors", func() {
		It("should have correct matchLabels to select only related pods", func() {
			By("verifying Agent PodMonitor selector")
			selector, err := getPodMonitorSelector(f.Namespace(), "podmonitor-agent-agent")
			Expect(err).NotTo(HaveOccurred())

			By("checking selector contains component=Agent")
			Expect(selector).To(HaveKeyWithValue("app.kubernetes.io/component", "Agent"))
			Expect(selector).To(HaveKeyWithValue("app.kubernetes.io/instance", "podmonitor-agent"))

			By("verifying Aggregator PodMonitor selector")
			selectorAgg, err := getPodMonitorSelector(f.Namespace(), "podmonitor-aggregator-aggregator")
			Expect(err).NotTo(HaveOccurred())

			By("checking selector contains component=Aggregator")
			Expect(selectorAgg).To(HaveKeyWithValue("app.kubernetes.io/component", "Aggregator"))
			Expect(selectorAgg).To(HaveKeyWithValue("app.kubernetes.io/instance", "podmonitor-aggregator"))
		})
	})

	Context("ClusterVectorAggregator PodMonitor with custom scrape settings", func() {
		It("should create PodMonitor with scrapeInterval and scrapeTimeout", func() {
			By("deploying ClusterVectorAggregator with custom scrape settings")
			f.ApplyTestData("podmonitor/cluster-aggregator-with-scrape-config.yaml")
			f.WaitForDeploymentReady("podmonitor-cluster-agg-aggregator")

			By("verifying PodMonitor is created")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-cluster-agg-aggregator")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying scrapeInterval is set correctly")
			interval, err := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-cluster-agg-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(interval).To(Equal("90s"), "scrapeInterval should be 90s")

			By("verifying scrapeTimeout is set correctly")
			timeout, err := getPodMonitorScrapeTimeout(f.Namespace(), "podmonitor-cluster-agg-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(Equal("25s"), "scrapeTimeout should be 25s")
		})

		It("should create PodMonitor with default settings when not specified", func() {
			By("deploying ClusterVectorAggregator with default settings")
			f.ApplyTestData("podmonitor/cluster-aggregator-with-defaults.yaml")
			f.WaitForDeploymentReady("podmonitor-cluster-agg-defaults-aggregator")

			By("verifying PodMonitor is created")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-cluster-agg-defaults-aggregator")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed())

			By("verifying scrapeInterval is not set (uses Prometheus default)")
			interval, err := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-cluster-agg-defaults-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(interval).To(BeEmpty(), "scrapeInterval should be empty when not specified")

			By("verifying scrapeTimeout is not set (uses Prometheus default)")
			timeout, err := getPodMonitorScrapeTimeout(f.Namespace(), "podmonitor-cluster-agg-defaults-aggregator")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(BeEmpty(), "scrapeTimeout should be empty when not specified")
		})

		It("should have correct matchLabels to select only ClusterVectorAggregator pods", func() {
			By("verifying ClusterVectorAggregator PodMonitor selector")
			selector, err := getPodMonitorSelector(f.Namespace(), "podmonitor-cluster-agg-aggregator")
			Expect(err).NotTo(HaveOccurred())

			By("checking selector contains component=Aggregator")
			Expect(selector).To(HaveKeyWithValue("app.kubernetes.io/component", "Aggregator"))
			Expect(selector).To(HaveKeyWithValue("app.kubernetes.io/instance", "podmonitor-cluster-agg"))
		})
	})
})

// InternalMetrics tests verify the isExporterSinkExists logic
var _ = Describe("Internal Metrics Exporter Logic", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-exporter-logic")

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	Context("Auto-add prometheus_exporter when not present", func() {
		It("should add default prometheus_exporter when pipeline has no exporter sink", func() {
			By("deploying test pod with app=test label")
			f.ApplyTestData("podmonitor/test-pod.yaml")
			f.WaitForPodReady("test-app")

			By("deploying Vector Agent with internalMetrics enabled")
			f.ApplyTestData("podmonitor/agent-with-defaults.yaml")
			time.Sleep(5 * time.Second)

			By("creating pipeline without prometheus_exporter sink")
			f.ApplyTestData("podmonitor/pipeline-without-exporter.yaml")
			f.WaitForPipelineValid("no-exporter-pipeline")

			By("verifying agent config contains default prometheus_exporter")
			Eventually(func() bool {
				return checkConfigHasExporter(f.Namespace(), "podmonitor-agent-defaults-agent", "internalMetricsSink")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(BeTrue(),
				"Agent config should have auto-added prometheus_exporter")
		})
	})

	Context("Skip adding prometheus_exporter when already present", func() {
		It("should NOT add default prometheus_exporter when pipeline already has one", func() {
			By("deploying Vector Agent with internalMetrics enabled")
			// Using existing agent from previous test

			By("creating pipeline WITH custom prometheus_exporter sink")
			f.ApplyTestData("podmonitor/pipeline-with-custom-exporter.yaml")
			f.WaitForPipelineValid("custom-exporter-pipeline")

			By("verifying agent config uses custom exporter from pipeline")
			Eventually(func() bool {
				return checkConfigHasExporter(f.Namespace(), "podmonitor-agent-defaults-agent", "custom_prom_exporter")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(BeTrue(),
				"Agent config should have custom prometheus_exporter from pipeline")

			By("verifying default exporter is NOT added when custom exporter exists")
			// When user provides custom prometheus_exporter, the default should NOT be added
			// because isExporterSinkExists() detects the custom exporter
			Consistently(func() bool {
				return !checkConfigHasExporter(f.Namespace(), "podmonitor-agent-defaults-agent", "internalMetricsSink")
			}, 10*time.Second, 2*time.Second).Should(BeTrue(),
				"Default exporter should NOT be added when custom exporter exists")
		})
	})
})

// PodMonitor Update tests verify that PodMonitor updates when CRD changes
var _ = Describe("PodMonitor Update Behavior", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-podmonitor-update")

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	Context("Update scrapeInterval and scrapeTimeout", func() {
		It("should update PodMonitor when Agent scrapeInterval changes", func() {
			By("deploying Vector Agent with initial scrapeInterval=45s")
			f.ApplyTestData("podmonitor/agent-with-scrape-config.yaml")
			time.Sleep(5 * time.Second)

			By("verifying initial scrapeInterval is 45s")
			Eventually(func() string {
				interval, _ := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-agent-agent")
				return interval
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Equal("45s"),
				"Initial scrapeInterval should be 45s")

			By("updating Vector Agent with new scrapeInterval=90s")
			f.ApplyTestData("podmonitor/agent-with-updated-interval.yaml")

			By("verifying PodMonitor scrapeInterval updates to 90s")
			Eventually(func() string {
				interval, _ := getPodMonitorScrapeInterval(f.Namespace(), "podmonitor-agent-agent")
				return interval
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Equal("90s"),
				"Updated scrapeInterval should be 90s")

			By("verifying scrapeTimeout also updates to 30s")
			Eventually(func() string {
				timeout, _ := getPodMonitorScrapeTimeout(f.Namespace(), "podmonitor-agent-agent")
				return timeout
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Equal("30s"),
				"Updated scrapeTimeout should be 30s")
		})
	})
})

// PodMonitor Cleanup tests verify that PodMonitor is deleted when Vector CR is deleted
var _ = Describe("PodMonitor Cleanup Behavior", Label(config.LabelSmoke, config.LabelFast), Ordered, func() {
	f := framework.NewUniqueFramework("test-podmonitor-cleanup")

	BeforeAll(func() {
		f.Setup()
	})

	AfterAll(func() {
		f.Teardown()
		f.PrintMetrics()
	})

	Context("Delete Vector CR", func() {
		It("should delete PodMonitor when Agent is deleted", func() {
			By("deploying Vector Agent with PodMonitor")
			f.ApplyTestData("podmonitor/agent-with-scrape-config.yaml")
			time.Sleep(5 * time.Second)

			By("verifying PodMonitor exists")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-agent-agent")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed(),
				"PodMonitor should exist after Agent creation")

			By("deleting Vector Agent CR")
			f.DeleteTestData("podmonitor/agent-with-scrape-config.yaml")

			By("verifying PodMonitor is cleaned up")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-agent-agent")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(HaveOccurred(),
				"PodMonitor should be deleted when Agent is deleted")
		})

		It("should delete PodMonitor when Aggregator is deleted", func() {
			By("deploying VectorAggregator with PodMonitor")
			f.ApplyTestData("podmonitor/aggregator-with-scrape-config.yaml")
			f.WaitForDeploymentReady("podmonitor-aggregator-aggregator")

			By("verifying PodMonitor exists")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-aggregator-aggregator")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(Succeed(),
				"PodMonitor should exist after Aggregator creation")

			By("deleting VectorAggregator CR")
			f.DeleteTestData("podmonitor/aggregator-with-scrape-config.yaml")

			By("verifying PodMonitor is cleaned up")
			Eventually(func() error {
				return checkPodMonitorExists(f.Namespace(), "podmonitor-aggregator-aggregator")
			}, config.ServiceCreateTimeout, config.DefaultPollInterval).Should(HaveOccurred(),
				"PodMonitor should be deleted when Aggregator is deleted")
		})
	})
})

// Helper functions for PodMonitor verification

func checkPodMonitorExists(namespace, name string) error {
	cmd := exec.Command("kubectl", "get", "podmonitor", name, "-n", namespace)
	_, err := cmd.Output()
	return err
}

func getPodMonitorScrapeInterval(namespace, name string) (string, error) {
	cmd := exec.Command("kubectl", "get", "podmonitor", name, "-n", namespace,
		"-o", "jsonpath={.spec.podMetricsEndpoints[0].interval}")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func getPodMonitorScrapeTimeout(namespace, name string) (string, error) {
	cmd := exec.Command("kubectl", "get", "podmonitor", name, "-n", namespace,
		"-o", "jsonpath={.spec.podMetricsEndpoints[0].scrapeTimeout}")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func getPodMonitorSelector(namespace, name string) (map[string]string, error) {
	cmd := exec.Command("kubectl", "get", "podmonitor", name, "-n", namespace, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var podMonitor struct {
		Spec struct {
			Selector struct {
				MatchLabels map[string]string `json:"matchLabels"`
			} `json:"selector"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(output, &podMonitor); err != nil {
		return nil, fmt.Errorf("failed to parse PodMonitor JSON: %w", err)
	}

	return podMonitor.Spec.Selector.MatchLabels, nil
}

func checkConfigHasExporter(namespace, secretName, exporterName string) bool {
	// Get the secret containing vector config
	cmd := exec.Command("kubectl", "get", "secret", secretName, "-n", namespace,
		"-o", "jsonpath={.data['agent\\.json']}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	if err != nil {
		return false
	}

	// Check if exporter name is in the config
	return strings.Contains(string(decoded), exporterName)
}
