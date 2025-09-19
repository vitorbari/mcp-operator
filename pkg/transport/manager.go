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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

// ManagerFactory creates transport-specific resource managers
type ManagerFactory struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewManagerFactory creates a new ManagerFactory
func NewManagerFactory(client client.Client, scheme *runtime.Scheme) *ManagerFactory {
	return &ManagerFactory{
		client: client,
		scheme: scheme,
	}
}

// GetManager returns the appropriate resource manager for the given transport type
func (f *ManagerFactory) GetManager(transportType mcpv1.MCPTransportType) (ResourceManager, error) {
	switch transportType {
	case mcpv1.MCPTransportHTTP:
		return NewHTTPResourceManager(f.client, f.scheme), nil
	case mcpv1.MCPTransportCustom:
		return NewCustomResourceManager(f.client, f.scheme), nil
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", transportType)
	}
}

// GetManagerForMCPServer returns the appropriate resource manager for an MCPServer
func (f *ManagerFactory) GetManagerForMCPServer(mcpServer *mcpv1.MCPServer) (ResourceManager, error) {
	transportType := GetDefaultTransportType()
	if mcpServer.Spec.Transport != nil {
		transportType = mcpServer.Spec.Transport.Type
	}
	return f.GetManager(transportType)
}
