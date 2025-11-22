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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

var (
	// MCPServer readiness metric
	mcpServerReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_ready_total",
			Help: "Number of ready MCP servers",
		},
		[]string{"namespace", "name"},
	)

	// MCPServer replica count metric
	mcpServerReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_replicas",
			Help: "Current replica count per MCP server",
		},
		[]string{"namespace", "name", "transport_type"},
	)

	// MCPServer available replica count metric
	mcpServerAvailableReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_available_replicas",
			Help: "Current available replica count per MCP server",
		},
		[]string{"namespace", "name", "transport_type"},
	)

	// Transport type distribution counter
	transportTypeDistribution = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcpserver_transport_type_total",
			Help: "Total count of transport types used",
		},
		[]string{"type"},
	)

	// Ingress enablement tracker
	mcpServerIngressEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_ingress_enabled",
			Help: "Whether ingress is enabled for MCP server (1=enabled, 0=disabled)",
		},
		[]string{"namespace", "name"},
	)

	// HPA enablement tracker
	mcpServerHPAEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_hpa_enabled",
			Help: "Whether HPA is enabled for MCP server (1=enabled, 0=disabled)",
		},
		[]string{"namespace", "name"},
	)

	// Resource requests tracking
	mcpServerResourceRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_resource_requests",
			Help: "Resource requests per MCP server",
		},
		[]string{"namespace", "name", "resource"},
	)

	// Reconciliation duration histogram
	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcpserver_reconcile_duration_seconds",
			Help:    "Time spent reconciling MCPServer resources",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller"},
	)

	// Reconciliation counter
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcpserver_reconcile_total",
			Help: "Total number of reconciliations",
		},
		[]string{"controller", "result"},
	)

	// MCPServer phase tracking
	mcpServerPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_phase",
			Help: "Current phase of MCPServer (1=current phase, 0=not current phase)",
		},
		[]string{"namespace", "name", "phase"},
	)

	// Validation compliance status
	mcpServerValidationCompliant = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_validation_compliant",
			Help: "Whether the MCPServer is MCP protocol compliant (1=compliant, 0=non-compliant, -1=not validated)",
		},
		[]string{"namespace", "name"},
	)

	// Validation duration histogram
	validationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcpserver_validation_duration_seconds",
			Help:    "Time spent validating MCP protocol compliance",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"namespace", "name"},
	)

	// Validation total counter
	validationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcpserver_validation_total",
			Help: "Total number of MCP protocol validations",
		},
		[]string{"namespace", "name", "result"},
	)

	// Validation issues counter
	validationIssues = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcpserver_validation_issues_total",
			Help: "Total number of validation issues found",
		},
		[]string{"namespace", "name", "level", "code"},
	)

	// Validation protocol version
	mcpServerProtocolVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_protocol_version",
			Help: "Detected MCP protocol version (1=current version, 0=not current version)",
		},
		[]string{"namespace", "name", "version"},
	)

	// Validation capabilities
	mcpServerCapabilities = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_capabilities",
			Help: "Discovered MCP capabilities (1=has capability, 0=does not have capability)",
		},
		[]string{"namespace", "name", "capability"},
	)

	// Time since last validation
	mcpServerLastValidation = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcpserver_last_validation_timestamp",
			Help: "Unix timestamp of the last validation check",
		},
		[]string{"namespace", "name"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		mcpServerReady,
		mcpServerReplicas,
		mcpServerAvailableReplicas,
		transportTypeDistribution,
		mcpServerIngressEnabled,
		mcpServerHPAEnabled,
		mcpServerResourceRequests,
		reconcileDuration,
		reconcileTotal,
		mcpServerPhase,
		mcpServerValidationCompliant,
		validationDuration,
		validationTotal,
		validationIssues,
		mcpServerProtocolVersion,
		mcpServerCapabilities,
		mcpServerLastValidation,
	)
}

// UpdateMCPServerMetrics updates all MCPServer-related metrics
func UpdateMCPServerMetrics(mcpServer *mcpv1.MCPServer) {
	labels := []string{mcpServer.Namespace, mcpServer.Name}

	// Track ready status
	if mcpServer.Status.ReadyReplicas > 0 {
		mcpServerReady.WithLabelValues(labels...).Set(1)
	} else {
		mcpServerReady.WithLabelValues(labels...).Set(0)
	}

	// Determine transport type
	transportType := "http" // default
	if mcpServer.Spec.Transport != nil {
		transportType = string(mcpServer.Spec.Transport.Type)
	}

	// Track replica counts with transport type
	replicaLabels := []string{mcpServer.Namespace, mcpServer.Name, transportType}
	mcpServerReplicas.WithLabelValues(replicaLabels...).Set(float64(mcpServer.Status.Replicas))
	mcpServerAvailableReplicas.WithLabelValues(replicaLabels...).Set(float64(mcpServer.Status.AvailableReplicas))

	// Count transport type usage
	transportTypeDistribution.WithLabelValues(transportType).Inc()

	// Track ingress enablement
	ingressEnabled := 0.0
	if mcpServer.Spec.Ingress != nil && mcpServer.Spec.Ingress.Enabled != nil && *mcpServer.Spec.Ingress.Enabled {
		ingressEnabled = 1.0
	}
	mcpServerIngressEnabled.WithLabelValues(labels...).Set(ingressEnabled)

	// Track HPA enablement
	hpaEnabled := 0.0
	if mcpServer.Spec.HPA != nil && mcpServer.Spec.HPA.Enabled != nil && *mcpServer.Spec.HPA.Enabled {
		hpaEnabled = 1.0
	}
	mcpServerHPAEnabled.WithLabelValues(labels...).Set(hpaEnabled)

	// Track resource requests
	if mcpServer.Spec.Resources != nil && mcpServer.Spec.Resources.Requests != nil {
		if cpu := mcpServer.Spec.Resources.Requests.Cpu(); cpu != nil {
			mcpServerResourceRequests.WithLabelValues(mcpServer.Namespace, mcpServer.Name, "cpu").Set(float64(cpu.MilliValue()))
		}
		if memory := mcpServer.Spec.Resources.Requests.Memory(); memory != nil {
			mcpServerResourceRequests.WithLabelValues(mcpServer.Namespace, mcpServer.Name, "memory").Set(float64(memory.Value()))
		}
	}

	// Track MCPServer phase
	phases := []string{"Pending", "Creating", "Running", "Updating", "Scaling", "Failed", "ValidationFailed", "Terminating"}
	for _, phase := range phases {
		value := 0.0
		if string(mcpServer.Status.Phase) == phase {
			value = 1.0
		}
		mcpServerPhase.WithLabelValues(mcpServer.Namespace, mcpServer.Name, phase).Set(value)
	}

	// Track validation status
	if mcpServer.Status.Validation != nil {
		// Track compliance status
		complianceValue := 0.0
		if mcpServer.Status.Validation.Compliant {
			complianceValue = 1.0
		}
		mcpServerValidationCompliant.WithLabelValues(labels...).Set(complianceValue)

		// Track protocol version
		supportedVersions := []string{"2024-11-05", "2025-03-26"}
		for _, version := range supportedVersions {
			versionValue := 0.0
			if mcpServer.Status.Validation.ProtocolVersion == version {
				versionValue = 1.0
			}
			mcpServerProtocolVersion.WithLabelValues(mcpServer.Namespace, mcpServer.Name, version).Set(versionValue)
		}

		// Track capabilities
		allCapabilities := []string{"tools", "resources", "prompts", "logging"}
		for _, capability := range allCapabilities {
			hasCapability := 0.0
			for _, discovered := range mcpServer.Status.Validation.Capabilities {
				if discovered == capability {
					hasCapability = 1.0
					break
				}
			}
			mcpServerCapabilities.WithLabelValues(mcpServer.Namespace, mcpServer.Name, capability).Set(hasCapability)
		}

		// Track last validation timestamp
		if mcpServer.Status.Validation.LastValidated != nil {
			mcpServerLastValidation.WithLabelValues(labels...).Set(float64(mcpServer.Status.Validation.LastValidated.Unix()))
		}
	} else {
		// No validation status - set to -1 to indicate not validated
		mcpServerValidationCompliant.WithLabelValues(labels...).Set(-1)
	}
}

// RecordReconcileMetrics records reconciliation timing and results
func RecordReconcileMetrics(controller string, duration float64, result string) {
	reconcileDuration.WithLabelValues(controller).Observe(duration)
	reconcileTotal.WithLabelValues(controller, result).Inc()
}

// RecordValidationMetrics records validation execution metrics
func RecordValidationMetrics(mcpServer *mcpv1.MCPServer, duration float64, success bool) {
	labels := []string{mcpServer.Namespace, mcpServer.Name}

	// Record validation duration
	validationDuration.WithLabelValues(labels...).Observe(duration)

	// Record validation result
	result := "success"
	if !success {
		result = "failure"
	}
	validationTotal.WithLabelValues(mcpServer.Namespace, mcpServer.Name, result).Inc()

	// Record validation issues if validation status is available
	if mcpServer.Status.Validation != nil {
		for _, issue := range mcpServer.Status.Validation.Issues {
			validationIssues.WithLabelValues(
				mcpServer.Namespace,
				mcpServer.Name,
				issue.Level,
				issue.Code,
			).Inc()
		}
	}
}

// DeleteMCPServerMetrics removes metrics for a deleted MCPServer
func DeleteMCPServerMetrics(mcpServer *mcpv1.MCPServer) {
	labels := []string{mcpServer.Namespace, mcpServer.Name}

	// Remove basic metrics
	mcpServerReady.DeleteLabelValues(labels...)
	mcpServerIngressEnabled.DeleteLabelValues(labels...)
	mcpServerHPAEnabled.DeleteLabelValues(labels...)

	// Remove resource metrics
	mcpServerResourceRequests.DeleteLabelValues(mcpServer.Namespace, mcpServer.Name, "cpu")
	mcpServerResourceRequests.DeleteLabelValues(mcpServer.Namespace, mcpServer.Name, "memory")

	// Remove replica metrics (need transport type)
	transportType := "http"
	if mcpServer.Spec.Transport != nil {
		transportType = string(mcpServer.Spec.Transport.Type)
	}
	replicaLabels := []string{mcpServer.Namespace, mcpServer.Name, transportType}
	mcpServerReplicas.DeleteLabelValues(replicaLabels...)
	mcpServerAvailableReplicas.DeleteLabelValues(replicaLabels...)

	// Remove phase metrics
	phases := []string{"Creating", "Running", "Updating", "Scaling", "Failed", "Terminating"}
	for _, phase := range phases {
		mcpServerPhase.DeleteLabelValues(mcpServer.Namespace, mcpServer.Name, phase)
	}

	// Remove validation metrics
	mcpServerValidationCompliant.DeleteLabelValues(labels...)
	mcpServerLastValidation.DeleteLabelValues(labels...)

	// Remove protocol version metrics
	supportedVersions := []string{"2024-11-05", "2025-03-26"}
	for _, version := range supportedVersions {
		mcpServerProtocolVersion.DeleteLabelValues(mcpServer.Namespace, mcpServer.Name, version)
	}

	// Remove capability metrics
	allCapabilities := []string{"tools", "resources", "prompts", "logging"}
	for _, capability := range allCapabilities {
		mcpServerCapabilities.DeleteLabelValues(mcpServer.Namespace, mcpServer.Name, capability)
	}
}
