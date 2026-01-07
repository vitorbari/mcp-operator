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

package transport

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
	"github.com/vitorbari/mcp-operator/internal/utils"
)

// HTTPResourceManager manages resources for HTTP transport (MCP streamable HTTP)
type HTTPResourceManager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewHTTPResourceManager creates a new HTTPResourceManager
func NewHTTPResourceManager(k8sClient client.Client, scheme *runtime.Scheme) *HTTPResourceManager {
	return &HTTPResourceManager{
		client: k8sClient,
		scheme: scheme,
	}
}

// CreateResources creates HTTP transport resources
func (h *HTTPResourceManager) CreateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Create deployment
	if err := h.createDeployment(ctx, mcpServer); err != nil {
		return err
	}

	// Create service
	if err := h.createService(ctx, mcpServer); err != nil {
		return err
	}

	return nil
}

// UpdateResources updates HTTP transport resources
func (h *HTTPResourceManager) UpdateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Update deployment
	if err := h.updateDeployment(ctx, mcpServer); err != nil {
		return err
	}

	// Update service
	if err := h.updateService(ctx, mcpServer); err != nil {
		return err
	}

	return nil
}

// DeleteResources cleans up HTTP transport resources
func (h *HTTPResourceManager) DeleteResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Resources will be cleaned up automatically via owner references
	return nil
}

// GetTransportType returns the transport type
func (h *HTTPResourceManager) GetTransportType() mcpv1.MCPTransportType {
	return mcpv1.MCPTransportHTTP
}

// RequiresService returns true for HTTP transport
func (h *HTTPResourceManager) RequiresService() bool {
	return true
}

// getHTTPPort returns the port for HTTP transport
func (h *HTTPResourceManager) getHTTPPort(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil &&
		mcpServer.Spec.Transport.Config.HTTP.Port != 0 {
		return mcpServer.Spec.Transport.Config.HTTP.Port
	}
	return 8080 // default
}

// getHTTPPath returns the path for HTTP transport
func (h *HTTPResourceManager) getHTTPPath(mcpServer *mcpv1.MCPServer) string {
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil &&
		mcpServer.Spec.Transport.Config.HTTP.Path != "" {
		return mcpServer.Spec.Transport.Config.HTTP.Path
	}
	return "/mcp" // default per ADR
}

// hasSessionManagement returns whether session management is enabled
func (h *HTTPResourceManager) hasSessionManagement(mcpServer *mcpv1.MCPServer) bool {
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil &&
		mcpServer.Spec.Transport.Config.HTTP.SessionManagement != nil {
		return *mcpServer.Spec.Transport.Config.HTTP.SessionManagement
	}
	return false // default
}

// createDeployment creates a deployment for HTTP transport
func (h *HTTPResourceManager) createDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := h.buildDeployment(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, deployment, h.scheme); err != nil {
		return err
	}

	found := &appsv1.Deployment{}
	err := h.client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return h.client.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	return nil
}

// updateDeployment updates the HTTP deployment using selective field comparison
// to avoid reconciliation loops when HPA is managing replicas.
//
// This implements the recommended kubebuilder pattern: only update the specific
// fields the operator manages, leaving HPA-managed fields untouched.
func (h *HTTPResourceManager) updateDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := h.buildDeployment(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, deployment, h.scheme); err != nil {
		return err
	}

	// Use retry logic for optimistic concurrency conflicts
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		found := &appsv1.Deployment{}
		err := h.client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
		if err != nil {
			return err
		}

		// Compare only the fields this operator manages, avoiding unnecessary updates
		// when HPA has modified replicas.
		//
		// Key principle: If HPA is enabled, we don't touch replicas. If not, we manage them.
		// This prevents reconciliation loops and unnecessary deployment generation increments.
		needsUpdate := false

		// 1. Update replicas only if HPA is not enabled
		// When HPA is enabled, it owns the replicas field via the scale subresource
		if !isHPAEnabled(mcpServer) {
			if deployment.Spec.Replicas != nil &&
				(found.Spec.Replicas == nil || *found.Spec.Replicas != *deployment.Spec.Replicas) {
				found.Spec.Replicas = deployment.Spec.Replicas
				needsUpdate = true
			}
		}

		// 2. Update selector (immutable after creation, but verify for safety)
		if !reflect.DeepEqual(found.Spec.Selector, deployment.Spec.Selector) {
			found.Spec.Selector = deployment.Spec.Selector
			needsUpdate = true
		}

		// 3. Update pod template spec
		// DeepEqual works here because 'found' was read from API server with all defaults applied,
		// and 'deployment' contains our desired state. If they match, nothing changed.
		if !reflect.DeepEqual(found.Spec.Template, deployment.Spec.Template) {
			found.Spec.Template = deployment.Spec.Template
			needsUpdate = true
		}

		if needsUpdate {
			return h.client.Update(ctx, found)
		}

		// No update needed - idempotent success
		return nil
	})
}

// buildDeployment builds a deployment for HTTP transport
func (h *HTTPResourceManager) buildDeployment(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	port := h.getHTTPPort(mcpServer)

	// Create container
	container := utils.BuildBaseContainer(mcpServer, port)

	// Add HTTP-specific environment variables
	httpPath := h.getHTTPPath(mcpServer)
	container.Env = append(container.Env,
		corev1.EnvVar{
			Name:  "MCP_TRANSPORT",
			Value: "http",
		},
		corev1.EnvVar{
			Name:  "MCP_HTTP_PATH",
			Value: httpPath,
		},
	)

	// Add session management if enabled
	if h.hasSessionManagement(mcpServer) {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "MCP_SESSION_MANAGEMENT",
			Value: "true",
		})
	}

	// Add health probes for HTTP endpoints
	utils.AddHealthProbes(&container, mcpServer, port)

	containers := []corev1.Container{container}

	// Inject sidecar if metrics is enabled
	if h.shouldInjectSidecar(mcpServer) {
		sidecar := h.buildSidecarContainer(mcpServer, port)
		containers = append(containers, sidecar)
	}

	podSpec := utils.BuildBasePodSpec(mcpServer, containers)

	// Add TLS volume if sidecar TLS is configured
	if h.shouldInjectSidecar(mcpServer) && mcpServer.Spec.Sidecar != nil &&
		mcpServer.Spec.Sidecar.TLS != nil && mcpServer.Spec.Sidecar.TLS.Enabled {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: mcpServer.Spec.Sidecar.TLS.SecretName,
				},
			},
		})
	}

	return utils.BuildDeployment(mcpServer, podSpec)
}

// createService creates a service for HTTP transport
func (h *HTTPResourceManager) createService(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	service := h.buildService(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, service, h.scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := h.client.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return h.client.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// updateService updates the HTTP service
func (h *HTTPResourceManager) updateService(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	service := h.buildService(mcpServer)
	return utils.UpdateService(ctx, h.client, h.scheme, mcpServer, service)
}

// buildService builds a service for HTTP transport
func (h *HTTPResourceManager) buildService(mcpServer *mcpv1.MCPServer) *corev1.Service {
	port := h.getHTTPPort(mcpServer)

	// HTTP transport specific annotations for streamable HTTP
	annotations := map[string]string{
		"mcp.transport.type": "http",
	}

	// Add session affinity if session management is enabled
	if h.hasSessionManagement(mcpServer) {
		annotations["nginx.ingress.kubernetes.io/affinity"] = "cookie"
		annotations["nginx.ingress.kubernetes.io/session-cookie-name"] = "mcp-session"
		annotations["nginx.ingress.kubernetes.io/session-cookie-expires"] = "86400"
	}

	// Add streaming-friendly annotations for SSE support in streamable HTTP
	annotations["nginx.ingress.kubernetes.io/proxy-buffering"] = "off"
	annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] = "86400"
	annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = "86400"

	// Add AWS Load Balancer annotations for HTTP transport
	annotations["service.beta.kubernetes.io/aws-load-balancer-backend-protocol"] = "http"
	annotations["service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"] = "3600"

	// When sidecar is injected, the service points to sidecar port (8080)
	// The sidecar then proxies to the MCP server on localhost
	servicePort := port
	if h.shouldInjectSidecar(mcpServer) {
		servicePort = mcpv1.DefaultSidecarPort
	}

	service := utils.BuildService(mcpServer, servicePort, corev1.ProtocolTCP, annotations)

	// Add metrics port if sidecar is enabled
	if h.shouldInjectSidecar(mcpServer) {
		metricsPort := h.getMetricsPort(mcpServer)
		service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
			Name:       "metrics",
			Port:       metricsPort,
			TargetPort: intstr.FromInt(int(metricsPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// Set session affinity if session management is enabled
	if h.hasSessionManagement(mcpServer) {
		service.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
	}

	return service
}

// isHPAEnabled checks if HPA is enabled for the MCPServer
func isHPAEnabled(mcpServer *mcpv1.MCPServer) bool {
	return mcpServer.Spec.HPA != nil &&
		mcpServer.Spec.HPA.Enabled != nil &&
		*mcpServer.Spec.HPA.Enabled
}

// shouldInjectSidecar returns true if the metrics sidecar should be injected
func (h *HTTPResourceManager) shouldInjectSidecar(mcpServer *mcpv1.MCPServer) bool {
	return mcpServer.Spec.Metrics != nil && mcpServer.Spec.Metrics.Enabled
}

// getMetricsPort returns the port for the Prometheus metrics endpoint
func (h *HTTPResourceManager) getMetricsPort(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Metrics != nil && mcpServer.Spec.Metrics.Port != 0 {
		return mcpServer.Spec.Metrics.Port
	}
	return mcpv1.DefaultMetricsPort
}

// getSidecarImage returns the sidecar image to use
func (h *HTTPResourceManager) getSidecarImage(mcpServer *mcpv1.MCPServer) string {
	if mcpServer.Spec.Sidecar != nil && mcpServer.Spec.Sidecar.Image != "" {
		return mcpServer.Spec.Sidecar.Image
	}
	return mcpv1.DefaultSidecarImage
}

// buildSidecarContainer builds the metrics sidecar container
func (h *HTTPResourceManager) buildSidecarContainer(mcpServer *mcpv1.MCPServer, targetPort int32) corev1.Container {
	metricsPort := h.getMetricsPort(mcpServer)
	sidecarImage := h.getSidecarImage(mcpServer)

	// Build sidecar args
	args := []string{
		fmt.Sprintf("--target-addr=localhost:%d", targetPort),
		fmt.Sprintf("--listen-addr=:%d", mcpv1.DefaultSidecarPort),
		fmt.Sprintf("--metrics-addr=:%d", metricsPort),
		"--log-level=info",
	}

	// Add TLS args if configured
	if mcpServer.Spec.Sidecar != nil && mcpServer.Spec.Sidecar.TLS != nil && mcpServer.Spec.Sidecar.TLS.Enabled {
		args = append(args,
			"--tls-enabled",
			"--tls-cert-file=/etc/tls/tls.crt",
			"--tls-key-file=/etc/tls/tls.key",
		)
		if mcpServer.Spec.Sidecar.TLS.MinVersion != "" {
			args = append(args, fmt.Sprintf("--tls-min-version=%s", mcpServer.Spec.Sidecar.TLS.MinVersion))
		}
	}

	container := corev1.Container{
		Name:  "mcp-proxy",
		Image: sidecarImage,
		Args:  args,
		Ports: []corev1.ContainerPort{
			{
				Name:          "mcp",
				ContainerPort: mcpv1.DefaultSidecarPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "metrics",
				ContainerPort: metricsPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(int(metricsPort)),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt(int(metricsPort)),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
			TimeoutSeconds:      3,
			FailureThreshold:    3,
		},
		Resources: h.getSidecarResources(mcpServer),
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             boolPtr(true),
			ReadOnlyRootFilesystem:   boolPtr(true),
			AllowPrivilegeEscalation: boolPtr(false),
		},
	}

	// Add TLS volume mount if configured
	if mcpServer.Spec.Sidecar != nil && mcpServer.Spec.Sidecar.TLS != nil && mcpServer.Spec.Sidecar.TLS.Enabled {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "tls-certs",
			MountPath: "/etc/tls",
			ReadOnly:  true,
		})
	}

	return container
}

// getSidecarResources returns the resource requirements for the sidecar
func (h *HTTPResourceManager) getSidecarResources(mcpServer *mcpv1.MCPServer) corev1.ResourceRequirements {
	// Use custom resources if specified
	if mcpServer.Spec.Sidecar != nil && mcpServer.Spec.Sidecar.Resources.Requests != nil {
		return mcpServer.Spec.Sidecar.Resources
	}

	// Return defaults
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(mcpv1.DefaultSidecarCPURequest),
			corev1.ResourceMemory: resource.MustParse(mcpv1.DefaultSidecarMemoryRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(mcpv1.DefaultSidecarCPULimit),
			corev1.ResourceMemory: resource.MustParse(mcpv1.DefaultSidecarMemoryLimit),
		},
	}
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
