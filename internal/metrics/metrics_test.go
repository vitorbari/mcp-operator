/*
Copyright 2025 Vitor Bari.

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

package metrics

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

var _ = Describe("Metrics", func() {
	var (
		mcpServer *mcpv1.MCPServer
	)

	BeforeEach(func() {
		// Reset metrics between tests
		mcpServerReady.Reset()
		mcpServerReplicas.Reset()
		mcpServerAvailableReplicas.Reset()
		transportTypeDistribution.Reset()
		mcpServerHPAEnabled.Reset()
		mcpServerResourceRequests.Reset()
		reconcileDuration.Reset()
		reconcileTotal.Reset()
		mcpServerPhase.Reset()

		// Create test MCPServer
		mcpServer = &mcpv1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server",
				Namespace: "default",
			},
			Spec: mcpv1.MCPServerSpec{
				Image: "nginx:1.21",
				Transport: &mcpv1.MCPServerTransport{
					Type: mcpv1.MCPTransportHTTP,
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				HPA: &mcpv1.MCPServerHPA{
					Enabled: ptr(true),
				},
			},
			Status: mcpv1.MCPServerStatus{
				Phase:             mcpv1.MCPServerPhaseRunning,
				Replicas:          3,
				ReadyReplicas:     2,
				AvailableReplicas: 2,
			},
		}
	})

	Describe("UpdateMCPServerMetrics", func() {
		It("should update ready metrics correctly", func() {
			UpdateMCPServerMetrics(mcpServer)

			// Check ready metric
			metric := &dto.Metric{}
			Expect(mcpServerReady.WithLabelValues("default", "test-server").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1))) // ReadyReplicas > 0
		})

		It("should set ready to 0 when no ready replicas", func() {
			mcpServer.Status.ReadyReplicas = 0
			UpdateMCPServerMetrics(mcpServer)

			metric := &dto.Metric{}
			Expect(mcpServerReady.WithLabelValues("default", "test-server").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))
		})

		It("should update replica count metrics", func() {
			UpdateMCPServerMetrics(mcpServer)

			// Check replicas metric
			metric := &dto.Metric{}
			Expect(mcpServerReplicas.WithLabelValues("default", "test-server", "http").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			// Check available replicas metric
			metric = &dto.Metric{}
			Expect(mcpServerAvailableReplicas.WithLabelValues("default", "test-server", "http").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(2)))
		})

		It("should track transport type distribution", func() {
			UpdateMCPServerMetrics(mcpServer)

			// Check transport type counter
			metric := &dto.Metric{}
			Expect(transportTypeDistribution.WithLabelValues("http").Write(metric)).To(Succeed())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)))
		})

		It("should track HPA enablement", func() {
			UpdateMCPServerMetrics(mcpServer)

			// Check HPA enabled metric
			metric := &dto.Metric{}
			Expect(mcpServerHPAEnabled.WithLabelValues("default", "test-server").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))
		})

		It("should track resource requests", func() {
			UpdateMCPServerMetrics(mcpServer)

			// Check CPU resource requests
			metric := &dto.Metric{}
			Expect(mcpServerResourceRequests.WithLabelValues("default", "test-server", "cpu").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(100))) // 100m = 100 milliCores

			// Check Memory resource requests
			metric = &dto.Metric{}
			Expect(mcpServerResourceRequests.WithLabelValues("default", "test-server", "memory").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(134217728))) // 128Mi in bytes
		})

		It("should track MCPServer phase", func() {
			UpdateMCPServerMetrics(mcpServer)

			// Check running phase is set to 1
			metric := &dto.Metric{}
			Expect(mcpServerPhase.WithLabelValues("default", "test-server", "Running").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			// Check other phases are set to 0
			metric = &dto.Metric{}
			Expect(mcpServerPhase.WithLabelValues("default", "test-server", "Creating").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			metric = &dto.Metric{}
			Expect(mcpServerPhase.WithLabelValues("default", "test-server", "Failed").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))
		})

		It("should handle missing transport configuration", func() {
			mcpServer.Spec.Transport = nil
			UpdateMCPServerMetrics(mcpServer)

			// Should default to http
			metric := &dto.Metric{}
			Expect(mcpServerReplicas.WithLabelValues("default", "test-server", "http").Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))
		})
	})

	Describe("RecordReconcileMetrics", func() {
		It("should record reconciliation duration", func() {
			RecordReconcileMetrics("mcpserver", 0.150, "success")

			// For histograms, we can't easily test the exact value without
			// gathering all metrics. We just verify the call doesn't error.
			// The histogram will increment the appropriate buckets internally.
		})

		It("should record reconciliation count", func() {
			RecordReconcileMetrics("mcpserver", 0.100, "success")

			metric := &dto.Metric{}
			Expect(reconcileTotal.WithLabelValues("mcpserver", "success").Write(metric)).To(Succeed())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)))
		})

		It("should record error reconciliations", func() {
			RecordReconcileMetrics("mcpserver", 0.200, "error")

			metric := &dto.Metric{}
			Expect(reconcileTotal.WithLabelValues("mcpserver", "error").Write(metric)).To(Succeed())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)))
		})
	})

	Describe("DeleteMCPServerMetrics", func() {
		BeforeEach(func() {
			// First update metrics to have something to delete
			UpdateMCPServerMetrics(mcpServer)
		})

		It("should delete all metrics for an MCPServer", func() {
			DeleteMCPServerMetrics(mcpServer)

			// After deletion, metrics should be reset to 0 or not found
			// This is a bit tricky to test with Prometheus client as deleted metrics
			// may still return 0 values. The important thing is that the metrics
			// are removed from the registry so new instances don't get confused

			// We can verify by trying to get metrics and checking they're reset
			metric := &dto.Metric{}

			// The metric should still exist but be reset to 0
			err := mcpServerReady.WithLabelValues("default", "test-server").Write(metric)
			if err == nil {
				// If we can still get the metric, it should be 0 (deleted/reset)
				Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))
			}
		})

	})

	Describe("Registry integration", func() {
		It("should have all metrics registered", func() {
			// Create a new registry to test registration
			registry := prometheus.NewRegistry()

			// Register all our metrics - this will panic if there's an issue
			Expect(func() {
				registry.MustRegister(
					mcpServerReady,
					mcpServerReplicas,
					mcpServerAvailableReplicas,
					transportTypeDistribution,
					mcpServerHPAEnabled,
					mcpServerResourceRequests,
					reconcileDuration,
					reconcileTotal,
					mcpServerPhase,
				)
			}).NotTo(Panic())

			// Add some test data to ensure metrics appear in gather
			UpdateMCPServerMetrics(mcpServer)
			RecordReconcileMetrics("test", 0.1, "success")

			// Gather metrics to ensure they're properly registered and have data
			metricFamilies, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(metricFamilies)).To(BeNumerically(">", 5)) // Should have multiple metrics
		})
	})
})

// Helper function to create pointer to values
func ptr[T any](v T) *T {
	return &v
}
