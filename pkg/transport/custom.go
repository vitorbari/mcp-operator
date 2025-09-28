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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
	"github.com/vitorbari/mcp-operator/pkg/utils"
)

const (
	protocolTCP   = "tcp"
	protocolUDP   = "udp"
	protocolSCTP  = "sctp"
	protocolHTTP  = "http"
	protocolHTTPS = "https"
)

// CustomResourceManager manages resources for custom transport
type CustomResourceManager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewCustomResourceManager creates a new CustomResourceManager
func NewCustomResourceManager(k8sClient client.Client, scheme *runtime.Scheme) *CustomResourceManager {
	return &CustomResourceManager{
		client: k8sClient,
		scheme: scheme,
	}
}

// CreateResources creates custom transport resources
func (c *CustomResourceManager) CreateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Create deployment
	if err := c.createDeployment(ctx, mcpServer); err != nil {
		return err
	}

	// Create service if needed
	if c.RequiresService() {
		if err := c.createService(ctx, mcpServer); err != nil {
			return err
		}
	}

	return nil
}

// UpdateResources updates custom transport resources
func (c *CustomResourceManager) UpdateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Update deployment
	if err := c.updateDeployment(ctx, mcpServer); err != nil {
		return err
	}

	// Update service if needed
	if c.RequiresService() {
		if err := c.updateService(ctx, mcpServer); err != nil {
			return err
		}
	}

	return nil
}

// DeleteResources cleans up custom transport resources
func (c *CustomResourceManager) DeleteResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Resources will be cleaned up automatically via owner references
	return nil
}

// GetTransportType returns the transport type
func (c *CustomResourceManager) GetTransportType() mcpv1.MCPTransportType {
	return mcpv1.MCPTransportCustom
}

// RequiresService returns true for custom transport (unless protocol is local)
func (c *CustomResourceManager) RequiresService() bool {
	// Most custom transports require services, but could be configurable
	return true
}

// RequiresIngress returns false for custom transport (depends on protocol)
func (c *CustomResourceManager) RequiresIngress() bool {
	// Custom transports may or may not support ingress depending on protocol
	return false
}

// getCustomPort returns the port for custom transport
func (c *CustomResourceManager) getCustomPort(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.Custom != nil &&
		mcpServer.Spec.Transport.Config.Custom.Port != 0 {
		return mcpServer.Spec.Transport.Config.Custom.Port
	}
	return 8080 // default
}

// getCustomProtocol returns the protocol for custom transport
func (c *CustomResourceManager) getCustomProtocol(mcpServer *mcpv1.MCPServer) string {
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.Custom != nil &&
		mcpServer.Spec.Transport.Config.Custom.Protocol != "" {
		return mcpServer.Spec.Transport.Config.Custom.Protocol
	}
	return protocolTCP // default
}

// getCustomConfig returns the custom configuration map
func (c *CustomResourceManager) getCustomConfig(mcpServer *mcpv1.MCPServer) map[string]string {
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.Custom != nil &&
		mcpServer.Spec.Transport.Config.Custom.Config != nil {
		return mcpServer.Spec.Transport.Config.Custom.Config
	}
	return make(map[string]string)
}

// createDeployment creates a deployment for custom transport
func (c *CustomResourceManager) createDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := c.buildDeployment(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, deployment, c.scheme); err != nil {
		return err
	}

	found := &appsv1.Deployment{}
	err := c.client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return c.client.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	return nil
}

// updateDeployment updates the custom deployment
func (c *CustomResourceManager) updateDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := c.buildDeployment(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, deployment, c.scheme); err != nil {
		return err
	}

	// Use retry logic for optimistic concurrency conflicts
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		found := &appsv1.Deployment{}
		err := c.client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
		if err != nil {
			return err
		}

		// Update deployment if necessary
		if !reflect.DeepEqual(found.Spec, deployment.Spec) {
			found.Spec = deployment.Spec
			return c.client.Update(ctx, found)
		}

		return nil
	})
}

// buildDeployment builds a deployment for custom transport
func (c *CustomResourceManager) buildDeployment(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	port := c.getCustomPort(mcpServer)
	protocol := c.getCustomProtocol(mcpServer)
	customConfig := c.getCustomConfig(mcpServer)

	// Create container
	container := utils.BuildBaseContainer(mcpServer, port)

	// Adjust port name based on protocol
	if len(container.Ports) > 0 {
		switch protocol {
		case protocolTCP:
			container.Ports[0].Name = protocolTCP
		case protocolUDP:
			container.Ports[0].Name = protocolUDP
			container.Ports[0].Protocol = corev1.ProtocolUDP
		case protocolSCTP:
			container.Ports[0].Name = protocolSCTP
			container.Ports[0].Protocol = corev1.ProtocolSCTP
		default:
			container.Ports[0].Name = "custom"
		}
	}

	// Add custom transport-specific environment variables
	container.Env = append(container.Env,
		corev1.EnvVar{
			Name:  "MCP_TRANSPORT",
			Value: "custom",
		},
		corev1.EnvVar{
			Name:  "MCP_CUSTOM_PROTOCOL",
			Value: protocol,
		},
		corev1.EnvVar{
			Name:  "MCP_CUSTOM_PORT",
			Value: fmt.Sprintf("%d", port),
		},
	)

	// Add custom configuration as environment variables
	for key, value := range customConfig {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  fmt.Sprintf("MCP_CUSTOM_%s", key),
			Value: value,
		})
	}

	// For HTTP-like protocols, add health probes
	if protocol == protocolHTTP || protocol == protocolHTTPS {
		utils.AddHealthProbes(&container, mcpServer, port)
	}

	containers := []corev1.Container{container}
	podSpec := utils.BuildBasePodSpec(mcpServer, containers)

	return utils.BuildDeployment(mcpServer, podSpec)
}

// createService creates a service for custom transport
func (c *CustomResourceManager) createService(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	service := c.buildService(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, service, c.scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := c.client.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return c.client.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// updateService updates the custom service
func (c *CustomResourceManager) updateService(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	service := c.buildService(mcpServer)
	return utils.UpdateService(ctx, c.client, c.scheme, mcpServer, service)
}

// buildService builds a service for custom transport
func (c *CustomResourceManager) buildService(mcpServer *mcpv1.MCPServer) *corev1.Service {
	port := c.getCustomPort(mcpServer)
	protocol := c.getCustomProtocol(mcpServer)

	// Custom transport annotations
	annotations := map[string]string{
		"mcp.transport.type":     "custom",
		"mcp.transport.protocol": protocol,
	}

	// Add custom configuration to annotations
	customConfig := c.getCustomConfig(mcpServer)
	for key, value := range customConfig {
		annotations[fmt.Sprintf("mcp.transport.config.%s", key)] = value
	}

	// Add protocol-specific annotations for load balancers
	switch protocol {
	case protocolTCP:
		annotations["service.beta.kubernetes.io/aws-load-balancer-backend-protocol"] = protocolTCP
		annotations["service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"] = "3600"
	case protocolUDP:
		annotations["service.beta.kubernetes.io/aws-load-balancer-backend-protocol"] = protocolUDP
	case protocolHTTP, protocolHTTPS:
		// HTTP-like custom protocols
		annotations["service.beta.kubernetes.io/aws-load-balancer-backend-protocol"] = protocolHTTP
		annotations["service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"] = "3600"
		// Add streaming annotations for HTTP-like protocols
		annotations["nginx.ingress.kubernetes.io/proxy-buffering"] = "off"
		annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] = "3600"
		annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = "3600"
	}

	// Determine service protocol
	serviceProtocol := corev1.ProtocolTCP
	switch protocol {
	case protocolUDP:
		serviceProtocol = corev1.ProtocolUDP
	case protocolSCTP:
		serviceProtocol = corev1.ProtocolSCTP
	}

	service := utils.BuildService(mcpServer, port, serviceProtocol, annotations)

	// Override service port name based on protocol
	if len(service.Spec.Ports) > 0 {
		service.Spec.Ports[0].Name = protocol
	}

	// For non-HTTP protocols, consider using LoadBalancer for external access
	if protocol != protocolHTTP && protocol != protocolHTTPS {
		if mcpServer.Spec.Service == nil || mcpServer.Spec.Service.Type == "" {
			service.Spec.Type = corev1.ServiceTypeLoadBalancer
		}
	}

	return service
}
