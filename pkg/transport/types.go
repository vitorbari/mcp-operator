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

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

// ResourceManager defines the interface for transport-specific resource management
type ResourceManager interface {
	// CreateResources creates the transport-specific Kubernetes resources
	CreateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error

	// UpdateResources updates existing transport-specific resources
	UpdateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error

	// DeleteResources cleans up transport-specific resources
	DeleteResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error

	// GetTransportType returns the transport type this manager handles
	GetTransportType() mcpv1.MCPTransportType

	// RequiresService returns whether this transport needs a Service resource
	RequiresService() bool

	// RequiresIngress returns whether this transport can use an Ingress resource
	RequiresIngress() bool
}

// ResourceManagerConfig contains common configuration for resource managers
type ResourceManagerConfig struct {
	Client client.Client
	Logger interface{}
}

// TransportResources holds references to created resources for a specific transport
type TransportResources struct {
	Deployment *appsv1.Deployment
	Job        *batchv1.Job
	Service    *corev1.Service
}

// IsNetworkTransport returns true if the transport type requires network resources
func IsNetworkTransport(transportType mcpv1.MCPTransportType) bool {
	switch transportType {
	case mcpv1.MCPTransportHTTP, mcpv1.MCPTransportCustom:
		return true
	default:
		return true // Default to network transport
	}
}

// GetDefaultTransportType returns the default transport type
func GetDefaultTransportType() mcpv1.MCPTransportType {
	return mcpv1.MCPTransportHTTP
}

// GetTransportPort returns the port for the given transport configuration
func GetTransportPort(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Transport == nil || mcpServer.Spec.Transport.Config == nil {
		return 8080 // default
	}

	config := mcpServer.Spec.Transport.Config
	transportType := mcpServer.Spec.Transport.Type

	switch transportType {
	case mcpv1.MCPTransportHTTP:
		if config.HTTP != nil && config.HTTP.Port != 0 {
			return config.HTTP.Port
		}
		return 8080
	case mcpv1.MCPTransportCustom:
		if config.Custom != nil && config.Custom.Port != 0 {
			return config.Custom.Port
		}
		return 8080
	default:
		return 8080
	}
}
