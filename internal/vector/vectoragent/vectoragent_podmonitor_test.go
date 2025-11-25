package vectoragent

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

func TestCreateVectorAgentPodMonitor_WithCustomSettings(t *testing.T) {
	g := NewWithT(t)

	ctrl := &Controller{
		Vector: &vectorv1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vector",
				Namespace: "default",
			},
			Spec: vectorv1alpha1.VectorSpec{
				Agent: &vectorv1alpha1.VectorAgent{
					VectorCommon: vectorv1alpha1.VectorCommon{
						ScrapeInterval: "45s",
						ScrapeTimeout:  "15s",
					},
				},
			},
		},
	}

	pm := ctrl.createVectorAgentPodMonitor()

	// Verify PodMonitor structure
	g.Expect(pm).NotTo(BeNil(), "PodMonitor should not be nil")
	g.Expect(pm.Spec.PodMetricsEndpoints).To(HaveLen(1), "Should have exactly one endpoint")

	// Verify scrape settings
	endpoint := pm.Spec.PodMetricsEndpoints[0]
	g.Expect(string(endpoint.Interval)).To(Equal("45s"), "scrapeInterval should be 45s")
	g.Expect(string(endpoint.ScrapeTimeout)).To(Equal("15s"), "scrapeTimeout should be 15s")

	// Verify endpoint configuration
	g.Expect(endpoint.Port).To(Equal("prom-exporter"), "Port should be prom-exporter")
	g.Expect(endpoint.Path).To(Equal("/metrics"), "Path should be /metrics")

	// Verify metadata
	g.Expect(pm.ObjectMeta.Namespace).To(Equal("default"), "Namespace should match Vector namespace")
}

func TestCreateVectorAgentPodMonitor_WithDefaults(t *testing.T) {
	g := NewWithT(t)

	ctrl := &Controller{
		Vector: &vectorv1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vector",
				Namespace: "default",
			},
			Spec: vectorv1alpha1.VectorSpec{
				Agent: &vectorv1alpha1.VectorAgent{
					VectorCommon: vectorv1alpha1.VectorCommon{
						// No scrape settings specified
					},
				},
			},
		},
	}

	pm := ctrl.createVectorAgentPodMonitor()

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

func TestCreateVectorAgentPodMonitor_LabelSelector(t *testing.T) {
	g := NewWithT(t)

	ctrl := &Controller{
		Vector: &vectorv1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vector-agent",
				Namespace: "test-namespace",
			},
			Spec: vectorv1alpha1.VectorSpec{
				Agent: &vectorv1alpha1.VectorAgent{},
			},
		},
	}

	pm := ctrl.createVectorAgentPodMonitor()

	// Verify selector has proper labels to target only Agent pods
	g.Expect(pm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "Agent"),
		"Selector should include component=Agent label")
	g.Expect(pm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-vector-agent"),
		"Selector should include instance label matching Vector name")
}

func TestCreateVectorAgentPodMonitor_OnlyIntervalSet(t *testing.T) {
	g := NewWithT(t)

	ctrl := &Controller{
		Vector: &vectorv1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vector",
				Namespace: "default",
			},
			Spec: vectorv1alpha1.VectorSpec{
				Agent: &vectorv1alpha1.VectorAgent{
					VectorCommon: vectorv1alpha1.VectorCommon{
						ScrapeInterval: "30s",
						// ScrapeTimeout not set
					},
				},
			},
		},
	}

	pm := ctrl.createVectorAgentPodMonitor()

	endpoint := pm.Spec.PodMetricsEndpoints[0]
	g.Expect(string(endpoint.Interval)).To(Equal("30s"), "Interval should be set")
	g.Expect(string(endpoint.ScrapeTimeout)).To(BeEmpty(), "Timeout should remain empty")
}

func TestCreateVectorAgentPodMonitor_OnlyTimeoutSet(t *testing.T) {
	g := NewWithT(t)

	ctrl := &Controller{
		Vector: &vectorv1alpha1.Vector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vector",
				Namespace: "default",
			},
			Spec: vectorv1alpha1.VectorSpec{
				Agent: &vectorv1alpha1.VectorAgent{
					VectorCommon: vectorv1alpha1.VectorCommon{
						// ScrapeInterval not set
						ScrapeTimeout: "10s",
					},
				},
			},
		},
	}

	pm := ctrl.createVectorAgentPodMonitor()

	endpoint := pm.Spec.PodMetricsEndpoints[0]
	g.Expect(string(endpoint.Interval)).To(BeEmpty(), "Interval should remain empty")
	g.Expect(string(endpoint.ScrapeTimeout)).To(Equal("10s"), "Timeout should be set")
}

func TestCreateVectorAgentPodMonitor_DurationFormats(t *testing.T) {
	testCases := []struct {
		name         string
		interval     string
		timeout      string
		expectedInt  string
		expectedTime string
	}{
		{
			name:         "Seconds format",
			interval:     "30s",
			timeout:      "10s",
			expectedInt:  "30s",
			expectedTime: "10s",
		},
		{
			name:         "Minutes format",
			interval:     "5m",
			timeout:      "1m",
			expectedInt:  "5m",
			expectedTime: "1m",
		},
		{
			name:         "Mixed format",
			interval:     "1m30s",
			timeout:      "30s",
			expectedInt:  "1m30s",
			expectedTime: "30s",
		},
		{
			name:         "Milliseconds format",
			interval:     "500ms",
			timeout:      "100ms",
			expectedInt:  "500ms",
			expectedTime: "100ms",
		},
		{
			name:         "Hours format",
			interval:     "1h",
			timeout:      "30m",
			expectedInt:  "1h",
			expectedTime: "30m",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			ctrl := &Controller{
				Vector: &vectorv1alpha1.Vector{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vector",
						Namespace: "default",
					},
					Spec: vectorv1alpha1.VectorSpec{
						Agent: &vectorv1alpha1.VectorAgent{
							VectorCommon: vectorv1alpha1.VectorCommon{
								ScrapeInterval: tc.interval,
								ScrapeTimeout:  tc.timeout,
							},
						},
					},
				},
			}

			pm := ctrl.createVectorAgentPodMonitor()
			endpoint := pm.Spec.PodMetricsEndpoints[0]

			g.Expect(string(endpoint.Interval)).To(Equal(tc.expectedInt))
			g.Expect(string(endpoint.ScrapeTimeout)).To(Equal(tc.expectedTime))
		})
	}
}
