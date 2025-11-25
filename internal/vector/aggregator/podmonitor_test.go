package aggregator

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

// Helper function to create a Controller for testing
func createTestController(name, namespace string, spec *vectorv1alpha1.VectorAggregatorCommon, isCluster bool) *Controller {
	agg := &vectorv1alpha1.VectorAggregator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	return &Controller{
		Name:                name,
		Namespace:           namespace,
		VectorAggregator:    agg,
		APIVersion:          "observability.kaasops.io/v1alpha1",
		Kind:                "VectorAggregator",
		Spec:                spec,
		isClusterAggregator: isCluster,
	}
}

func TestCreateVectorAggregatorPodMonitor_WithCustomSettings(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{
			VectorCommon: vectorv1alpha1.VectorCommon{
				ScrapeInterval: "60s",
				ScrapeTimeout:  "20s",
			},
		}, false)

	pm := ctrl.createVectorAggregatorPodMonitor()

	// Verify PodMonitor structure
	g.Expect(pm).NotTo(BeNil(), "PodMonitor should not be nil")
	g.Expect(pm.Spec.PodMetricsEndpoints).To(HaveLen(1), "Should have exactly one endpoint")

	// Verify scrape settings
	endpoint := pm.Spec.PodMetricsEndpoints[0]
	g.Expect(string(endpoint.Interval)).To(Equal("60s"), "scrapeInterval should be 60s")
	g.Expect(string(endpoint.ScrapeTimeout)).To(Equal("20s"), "scrapeTimeout should be 20s")

	// Verify endpoint configuration
	g.Expect(endpoint.Port).To(Equal("prom-exporter"), "Port should be prom-exporter")
	g.Expect(endpoint.Path).To(Equal("/metrics"), "Path should be /metrics")

	// Verify metadata
	g.Expect(pm.ObjectMeta.Namespace).To(Equal("default"), "Namespace should match Aggregator namespace")
}

func TestCreateVectorAggregatorPodMonitor_WithDefaults(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{
			VectorCommon: vectorv1alpha1.VectorCommon{
				// No scrape settings specified
			},
		}, false)

	pm := ctrl.createVectorAggregatorPodMonitor()

	g.Expect(pm).NotTo(BeNil())
	g.Expect(pm.Spec.PodMetricsEndpoints).To(HaveLen(1))

	endpoint := pm.Spec.PodMetricsEndpoints[0]
	// When not specified, fields should be empty (Prometheus will use defaults)
	g.Expect(string(endpoint.Interval)).To(BeEmpty(), "Interval should be empty when not specified")
	g.Expect(string(endpoint.ScrapeTimeout)).To(BeEmpty(), "ScrapeTimeout should be empty when not specified")

	// Basic endpoint config should still be set
	g.Expect(endpoint.Port).To(Equal("prom-exporter"))
	g.Expect(endpoint.Path).To(Equal("/metrics"))
}

func TestCreateVectorAggregatorPodMonitor_LabelSelector(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "test-namespace",
		&vectorv1alpha1.VectorAggregatorCommon{}, false)

	pm := ctrl.createVectorAggregatorPodMonitor()

	// Verify selector has proper labels to target only Aggregator pods
	g.Expect(pm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "Aggregator"),
		"Selector should include component=Aggregator label")
	g.Expect(pm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-aggregator"),
		"Selector should include instance label matching Aggregator name")
}

func TestCreateVectorAggregatorPodMonitor_OnlyIntervalSet(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{
			VectorCommon: vectorv1alpha1.VectorCommon{
				ScrapeInterval: "1m",
				// ScrapeTimeout not set
			},
		}, false)

	pm := ctrl.createVectorAggregatorPodMonitor()

	endpoint := pm.Spec.PodMetricsEndpoints[0]
	g.Expect(string(endpoint.Interval)).To(Equal("1m"), "Interval should be set")
	g.Expect(string(endpoint.ScrapeTimeout)).To(BeEmpty(), "Timeout should remain empty")
}

func TestCreateVectorAggregatorPodMonitor_OnlyTimeoutSet(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("test-aggregator", "default",
		&vectorv1alpha1.VectorAggregatorCommon{
			VectorCommon: vectorv1alpha1.VectorCommon{
				// ScrapeInterval not set
				ScrapeTimeout: "30s",
			},
		}, false)

	pm := ctrl.createVectorAggregatorPodMonitor()

	endpoint := pm.Spec.PodMetricsEndpoints[0]
	g.Expect(string(endpoint.Interval)).To(BeEmpty(), "Interval should remain empty")
	g.Expect(string(endpoint.ScrapeTimeout)).To(Equal("30s"), "Timeout should be set")
}

func TestCreateVectorAggregatorPodMonitor_DurationFormats(t *testing.T) {
	testCases := []struct {
		name         string
		interval     string
		timeout      string
		expectedInt  string
		expectedTime string
	}{
		{
			name:         "Seconds format",
			interval:     "60s",
			timeout:      "20s",
			expectedInt:  "60s",
			expectedTime: "20s",
		},
		{
			name:         "Minutes format",
			interval:     "10m",
			timeout:      "2m",
			expectedInt:  "10m",
			expectedTime: "2m",
		},
		{
			name:         "Mixed format",
			interval:     "2m30s",
			timeout:      "45s",
			expectedInt:  "2m30s",
			expectedTime: "45s",
		},
		{
			name:         "Milliseconds format",
			interval:     "1000ms",
			timeout:      "500ms",
			expectedInt:  "1000ms",
			expectedTime: "500ms",
		},
		{
			name:         "Hours format",
			interval:     "2h",
			timeout:      "1h",
			expectedInt:  "2h",
			expectedTime: "1h",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			ctrl := createTestController("test-aggregator", "default",
				&vectorv1alpha1.VectorAggregatorCommon{
					VectorCommon: vectorv1alpha1.VectorCommon{
						ScrapeInterval: tc.interval,
						ScrapeTimeout:  tc.timeout,
					},
				}, false)

			pm := ctrl.createVectorAggregatorPodMonitor()
			endpoint := pm.Spec.PodMetricsEndpoints[0]

			g.Expect(string(endpoint.Interval)).To(Equal(tc.expectedInt))
			g.Expect(string(endpoint.ScrapeTimeout)).To(Equal(tc.expectedTime))
		})
	}
}

func TestCreateVectorAggregatorPodMonitor_ClusterAggregator(t *testing.T) {
	g := NewWithT(t)

	ctrl := createTestController("cluster-test-aggregator", "vector-system",
		&vectorv1alpha1.VectorAggregatorCommon{
			VectorCommon: vectorv1alpha1.VectorCommon{
				ScrapeInterval: "90s",
				ScrapeTimeout:  "25s",
			},
		}, true)

	pm := ctrl.createVectorAggregatorPodMonitor()

	// Verify ClusterVectorAggregator also gets proper PodMonitor
	g.Expect(pm).NotTo(BeNil())

	endpoint := pm.Spec.PodMetricsEndpoints[0]
	g.Expect(string(endpoint.Interval)).To(Equal("90s"))
	g.Expect(string(endpoint.ScrapeTimeout)).To(Equal("25s"))

	// Verify selector for ClusterVectorAggregator
	g.Expect(pm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "Aggregator"))
	g.Expect(pm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "cluster-test-aggregator"))
}
