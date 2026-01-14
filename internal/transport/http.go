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

// isSSEActive returns true if SSE transport is active (explicit or auto-detected).
// SSE is considered active when:
// 1. transport.protocol is explicitly set to "sse", OR
// 2. transport.protocol is "auto" AND SSE was detected (stored in status.resolvedTransport)
func (h *HTTPResourceManager) isSSEActive(mcpServer *mcpv1.MCPServer) bool {
	// Check for explicit SSE configuration
	if mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Protocol == mcpv1.MCPProtocolSSE {
		return true
	}

	// Check for auto-detected SSE (resolved in status)
	if mcpServer.Status.ResolvedTransport != nil &&
		mcpServer.Status.ResolvedTransport.Protocol == mcpv1.MCPProtocolSSE {
		return true
	}

	// Also check validation status for detected protocol (legacy/fallback)
	if mcpServer.Status.Validation != nil &&
		mcpServer.Status.Validation.Protocol == "sse" {
		// Only use this if auto-detect mode is enabled
		if mcpServer.Spec.Transport == nil ||
			mcpServer.Spec.Transport.Protocol == "" ||
			mcpServer.Spec.Transport.Protocol == mcpv1.MCPProtocolAuto {
			return true
		}
	}

	return false
}

// isExplicitSSE returns true only when SSE is explicitly configured (not auto-detected)
func (h *HTTPResourceManager) isExplicitSSE(mcpServer *mcpv1.MCPServer) bool {
	return mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Protocol == mcpv1.MCPProtocolSSE
}

// getSSEConfig returns the SSE configuration, or nil if not specified
func (h *HTTPResourceManager) getSSEConfig(mcpServer *mcpv1.MCPServer) *mcpv1.SSEConfig {
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil {
		return mcpServer.Spec.Transport.Config.HTTP.SSE
	}
	return nil
}

// shouldEnableSSESessionAffinity returns true if SSE session affinity should be enabled.
// Session affinity is enabled when:
// 1. SSE is explicitly configured AND enableSessionAffinity is true, OR
// 2. SSE is auto-detected AND enableSessionAffinity is explicitly set to true
func (h *HTTPResourceManager) shouldEnableSSESessionAffinity(mcpServer *mcpv1.MCPServer) bool {
	if !h.isSSEActive(mcpServer) {
		return false
	}

	sseConfig := h.getSSEConfig(mcpServer)
	if sseConfig == nil || sseConfig.EnableSessionAffinity == nil {
		return false
	}

	return *sseConfig.EnableSessionAffinity
}

// getSSETerminationGracePeriod returns the termination grace period for SSE deployments.
// Returns the configured value, or a default of 60 seconds if not specified.
func (h *HTTPResourceManager) getSSETerminationGracePeriod(mcpServer *mcpv1.MCPServer) *int64 {
	if !h.isSSEActive(mcpServer) {
		return nil
	}

	sseConfig := h.getSSEConfig(mcpServer)
	if sseConfig != nil && sseConfig.TerminationGracePeriodSeconds != nil {
		return sseConfig.TerminationGracePeriodSeconds
	}

	// Default termination grace period for SSE (60 seconds)
	defaultGracePeriod := int64(60)
	return &defaultGracePeriod
}

// getSSEMaxSurge returns the maxSurge value for SSE rolling updates.
// Returns the configured value, or a default of "25%" if not specified.
func (h *HTTPResourceManager) getSSEMaxSurge(mcpServer *mcpv1.MCPServer) *intstr.IntOrString {
	if !h.isSSEActive(mcpServer) {
		return nil
	}

	sseConfig := h.getSSEConfig(mcpServer)
	if sseConfig != nil && sseConfig.MaxSurge != nil {
		return sseConfig.MaxSurge
	}

	// Default maxSurge for SSE (25%)
	defaultMaxSurge := intstr.FromString("25%")
	return &defaultMaxSurge
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

		// 4. Update deployment strategy (for SSE-specific rolling update settings)
		if !reflect.DeepEqual(found.Spec.Strategy, deployment.Spec.Strategy) {
			found.Spec.Strategy = deployment.Spec.Strategy
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

	// Apply SSE-specific termination grace period
	if gracePeriod := h.getSSETerminationGracePeriod(mcpServer); gracePeriod != nil {
		podSpec.TerminationGracePeriodSeconds = gracePeriod
	}

	deployment := utils.BuildDeployment(mcpServer, podSpec)

	// Apply SSE-specific deployment strategy and annotations
	h.applySSEDeploymentSettings(mcpServer, deployment)

	return deployment
}

// applySSEDeploymentSettings applies SSE-specific settings to the deployment.
// This includes rolling update strategy, pod annotations, and other SSE optimizations.
func (h *HTTPResourceManager) applySSEDeploymentSettings(mcpServer *mcpv1.MCPServer, deployment *appsv1.Deployment) {
	if !h.isSSEActive(mcpServer) {
		return
	}

	// Add SSE-specific Pod annotations for observability and debugging
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["mcp.transport.protocol"] = "sse"
	deployment.Spec.Template.Annotations["mcp.transport.streaming"] = "true"

	// Apply rolling update strategy optimized for SSE (long-lived connections)
	// maxUnavailable=0 ensures no pods are killed before new ones are ready
	// This prevents abrupt termination of active SSE streams
	maxUnavailable := intstr.FromInt(0)
	maxSurge := h.getSSEMaxSurge(mcpServer)
	if maxSurge == nil {
		defaultMaxSurge := intstr.FromString("25%")
		maxSurge = &defaultMaxSurge
	}

	deployment.Spec.Strategy = appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &maxUnavailable,
			MaxSurge:       maxSurge,
		},
	}
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

	// Apply SSE-specific Service annotations when SSE is active
	h.applySSEServiceAnnotations(mcpServer, annotations)

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

	// Set session affinity based on configuration
	// Priority: SSE session affinity > general session management
	if h.shouldEnableSSESessionAffinity(mcpServer) {
		service.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
		// Configure session affinity timeout (default 3 hours for SSE connections)
		timeoutSeconds := int32(10800)
		service.Spec.SessionAffinityConfig = &corev1.SessionAffinityConfig{
			ClientIP: &corev1.ClientIPConfig{
				TimeoutSeconds: &timeoutSeconds,
			},
		}
	} else if h.hasSessionManagement(mcpServer) {
		service.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
	}

	return service
}

// applySSEServiceAnnotations adds SSE-specific annotations to the service.
// These are informational annotations that help with observability and external tooling.
func (h *HTTPResourceManager) applySSEServiceAnnotations(mcpServer *mcpv1.MCPServer, annotations map[string]string) {
	if !h.isSSEActive(mcpServer) {
		return
	}

	// Add SSE-specific informational annotations
	annotations["mcp.transport.protocol"] = "sse"
	annotations["mcp.transport.streaming"] = "true"

	// Note: We do NOT apply cloud-provider-specific annotations here
	// as per the requirements (no ingress-controller-specific annotations).
	// The nginx/AWS annotations above are transport-generic streaming support,
	// not SSE-specific.
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
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
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
