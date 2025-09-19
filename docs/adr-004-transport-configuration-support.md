# ADR-004: Transport Configuration Support

## Status

Proposed

## Context

The current MCP Operator assumes a single deployment pattern for all MCP servers, but the Model Context Protocol specification defines multiple transport mechanisms with different operational characteristics:

1. **Streamable HTTP** - The recommended transport for production remote MCP servers (MCP spec 2025-03-26)
2. **stdio** - For local/subprocess communication (not suitable for Kubernetes production)  
3. **Custom transports** - Extensible transport layer for specialized use cases

Different MCP servers implement different transports, and each requires specific Kubernetes resource configurations:

- **Streamable HTTP**: Requires Deployment + Service + optional Ingress with HTTP endpoints
- **Custom transports**: May require different networking or protocol configurations

Our operator currently creates a generic deployment without consideration for transport-specific requirements, limiting its ability to properly deploy diverse MCP servers.

## Decision

We will extend the MCPServer CRD to support transport-specific configuration that automatically creates appropriate Kubernetes resources based on the transport type.

### API Design

```go
type TransportConfig struct {
    Type   string                 `json:"type"`
    Config TransportConfigDetails `json:"config,omitempty"`
}

type TransportConfigDetails struct {
    HTTP   *HTTPTransportConfig   `json:"http,omitempty"`
    Custom *CustomTransportConfig `json:"custom,omitempty"`
}

type HTTPTransportConfig struct {
    Port              int32                    `json:"port"`
    Path              string                   `json:"path"`
    SessionManagement bool                     `json:"sessionManagement,omitempty"`
    Security          *HTTPSecurityConfig      `json:"security,omitempty"`
}

type HTTPSecurityConfig struct {
    ValidateOrigin  bool                `json:"validateOrigin,omitempty"`
    AllowedOrigins  []string            `json:"allowedOrigins,omitempty"`
    BindLocalhost   bool                `json:"bindLocalhost,omitempty"`
    Authentication  *AuthenticationConfig `json:"authentication,omitempty"`
}

type CustomTransportConfig struct {
    Port     int32             `json:"port,omitempty"`
    Protocol string            `json:"protocol,omitempty"`
    Config   map[string]string `json:"config,omitempty"`
}
```

### Usage Examples

**Streamable HTTP Transport:**
```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
spec:
  image: "my-mcp-server:latest"
  transport:
    type: "http"
    config:
      http:
        port: 8080
        path: "/mcp"
        sessionManagement: true
        security:
          validateOrigin: true
          allowedOrigins: ["https://myapp.example.com"]
```

**Custom Transport:**
```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
spec:
  image: "my-tcp-mcp-server:latest"
  transport:
    type: "custom"
    config:
      custom:
        port: 9090
        protocol: "tcp"
        config:
          bufferSize: "4096"
```

### Controller Implementation

The controller will implement transport-specific resource creation:

```go
func (r *MCPServerReconciler) reconcileTransport(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
    switch mcpServer.Spec.Transport.Type {
    case "http":
        return r.reconcileHTTPTransport(ctx, mcpServer)
    case "custom":
        return r.reconcileCustomTransport(ctx, mcpServer)
    default:
        // Default to HTTP transport for backward compatibility
        return r.reconcileHTTPTransport(ctx, mcpServer)
    }
}
```

**HTTP Transport Resources:**
- Standard Deployment with HTTP health checks
- Service exposing HTTP port with session affinity for SSE support  
- Optional Ingress with SSE-compatible annotations
- ConfigMaps for transport-specific configuration

**Custom Transport Resources:**
- Deployment with transport-specific configuration
- Service with appropriate protocol and port configuration
- Transport-specific networking policies

## Consequences

### Positive
- **MCP Specification Compliance**: Proper support for official MCP transports
- **Flexibility**: Support for diverse MCP server implementations
- **Production Ready**: Correct networking configuration for each transport type
- **Future Proof**: Extensible design for new transport types
- **Backward Compatibility**: Existing MCPServer resources continue working with default HTTP transport

### Negative  
- **Increased Complexity**: More CRD fields and controller logic to maintain
- **Breaking Change Potential**: Future transport additions may require CRD schema updates
- **Testing Overhead**: Each transport type requires separate test scenarios

### Neutral
- **Documentation**: Requires comprehensive examples for each transport type
- **Migration**: Existing deployments will default to HTTP transport behavior

## Implementation Plan

### Phase 1: Core HTTP Transport
1. Extend MCPServer CRD with transport configuration
2. Implement HTTP transport reconciliation logic
3. Add transport-specific resource creation functions
4. Update samples and documentation

### Phase 2: Custom Transport Support  
1. Implement custom transport reconciliation
2. Add validation for transport configurations
3. Create comprehensive integration tests
4. Add transport-specific troubleshooting guides

### Phase 3: Advanced Features
1. Add transport auto-detection based on container image metadata
2. Implement transport-specific monitoring and metrics
3. Add support for transport migrations
4. Create transport compatibility matrix

## References

- [MCP Transport Specification](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports)
- [Streamable HTTP Transport Details](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#streamable-http)
- [ADR-001: MCPServer API Design](./adr-001-mcpserver-api-design.md)
- [ADR-002: MCPServer Controller Implementation](./adr-002-mcpserver-controller-implementation.md)
