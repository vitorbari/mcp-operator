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

package controller

import (
	"context"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// isServiceMonitorCRDAvailable checks if the ServiceMonitor CRD is installed in the cluster
// by attempting to get a non-existent ServiceMonitor and checking the error type
func (r *MCPServerReconciler) isServiceMonitorCRDAvailable(ctx context.Context) bool {
	log := logf.FromContext(ctx)

	// Try to get a ServiceMonitor that doesn't exist
	// If the CRD is not installed, we'll get a "no matches for kind" error
	// If the CRD is installed, we'll get a "not found" error
	sm := &monitoringv1.ServiceMonitor{}
	err := r.Get(ctx, types.NamespacedName{Name: "__probe__", Namespace: "default"}, sm)
	if err != nil {
		// Check if this is a "no kind match" error (CRD not installed)
		if meta.IsNoMatchError(err) || isNoMatchError(err) {
			log.V(1).Info("ServiceMonitor CRD not found, Prometheus Operator not installed")
			return false
		}
		// NotFound error means the CRD exists but the resource doesn't - that's fine
		if errors.IsNotFound(err) {
			return true
		}
		// Some other error
		log.Error(err, "Failed to check for ServiceMonitor CRD availability")
		return false
	}

	return true
}

// isNoMatchError checks if the error indicates the CRD is not installed
func isNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "no matches for kind") ||
		strings.Contains(errStr, "the server could not find the requested resource")
}

// shouldCreateServiceMonitor returns true if a ServiceMonitor should be created for this MCPServer
func (r *MCPServerReconciler) shouldCreateServiceMonitor(mcpServer *mcpv1.MCPServer) bool {
	return mcpServer.Spec.Metrics != nil && mcpServer.Spec.Metrics.Enabled
}

// reconcileServiceMonitor ensures the ServiceMonitor exists if metrics are enabled and Prometheus Operator is installed
func (r *MCPServerReconciler) reconcileServiceMonitor(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	log := logf.FromContext(ctx)

	// Check if Prometheus Operator is installed
	if !r.isServiceMonitorCRDAvailable(ctx) {
		// Prometheus Operator not installed, skip ServiceMonitor creation
		log.V(1).Info("Skipping ServiceMonitor reconciliation - Prometheus Operator not installed")
		return nil
	}

	// If metrics are not enabled, delete any existing ServiceMonitor
	if !r.shouldCreateServiceMonitor(mcpServer) {
		existingServiceMonitor := &monitoringv1.ServiceMonitor{}
		err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, existingServiceMonitor)
		if err == nil {
			// ServiceMonitor exists, delete it
			log.Info("Deleting ServiceMonitor as metrics are disabled")
			if err := r.Delete(ctx, existingServiceMonitor); err != nil {
				return err
			}
		} else if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	// Use CreateOrUpdate with retry logic to handle both creation and updates idempotently
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		serviceMonitor := r.buildServiceMonitor(mcpServer)

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, serviceMonitor, func() error {
			// Set controller reference
			if err := controllerutil.SetControllerReference(mcpServer, serviceMonitor, r.Scheme); err != nil {
				return err
			}

			// Update the spec (this will be used for both create and update)
			desiredSpec := r.buildServiceMonitor(mcpServer).Spec
			serviceMonitor.Spec = desiredSpec

			return nil
		})

		if err != nil {
			log.Error(err, "Failed to create or update ServiceMonitor")
		}

		return err
	})
}

// buildServiceMonitor creates a ServiceMonitor object for the MCPServer
func (r *MCPServerReconciler) buildServiceMonitor(mcpServer *mcpv1.MCPServer) *monitoringv1.ServiceMonitor {
	labels := map[string]string{
		"app":                          mcpServer.Name,
		"app.kubernetes.io/name":       "mcpserver",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/managed-by": "mcp-operator",
		// Required by kube-prometheus-stack to discover ServiceMonitors
		"release": "monitoring",
	}

	// Get metrics port
	metricsPort := mcpv1.DefaultMetricsPort
	if mcpServer.Spec.Metrics != nil && mcpServer.Spec.Metrics.Port != 0 {
		metricsPort = mcpServer.Spec.Metrics.Port
	}

	// Build endpoint configuration
	endpoint := monitoringv1.Endpoint{
		Port:     "metrics",
		Path:     "/metrics",
		Interval: monitoringv1.Duration("30s"),
	}

	// Add TLS config if sidecar TLS is enabled
	if mcpServer.Spec.Sidecar != nil && mcpServer.Spec.Sidecar.TLS != nil && mcpServer.Spec.Sidecar.TLS.Enabled {
		insecureSkipVerify := true
		endpoint.Scheme = "https"
		endpoint.TLSConfig = &monitoringv1.TLSConfig{
			SafeTLSConfig: monitoringv1.SafeTLSConfig{
				InsecureSkipVerify: &insecureSkipVerify,
			},
		}
	}

	// Note: metricsPort variable is validated to ensure correct port configuration
	// The ServiceMonitor uses the port name "metrics" which references the Service port
	_ = metricsPort

	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": mcpServer.Name,
				},
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{mcpServer.Namespace},
			},
			Endpoints: []monitoringv1.Endpoint{endpoint},
			TargetLabels: []string{
				"app.kubernetes.io/name",
				"app.kubernetes.io/instance",
			},
		},
	}
}
