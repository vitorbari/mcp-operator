# ADR-001: MCPServer API Design

## Status

Accepted

## Context

The MCP (Model Context Protocol) Operator requires a comprehensive Kubernetes Custom Resource Definition (CRD) to manage MCP servers in enterprise environments. The API design must support:

1. Container lifecycle management (deploy, scale, configure, monitor)
2. Enterprise security requirements (RBAC, TLS)
3. Kubernetes-native integrations (networking, scaling, monitoring)
4. MCP-specific configurations and capabilities
5. Operational observability and status reporting

Target audience: Enterprise platform engineering teams managing AI tool infrastructure.

## Decision

We have designed the MCPServer CRD with the following structure:

### MCPServerSpec

The spec defines the desired state with these main components:

#### Core Container Management
- **Image**: Container image specification with validation pattern
- **Replicas**: Scaling configuration with minimum validation (default: 1)
- **Resources**: Standard Kubernetes resource requirements (CPU/Memory limits)
- **Environment**: Environment variables for container configuration

#### Security Configuration (MCPServerSecurity)
- **AllowedUsers/AllowedGroups**: RBAC integration for access control
- **TLS**: Complete TLS configuration with certificate management
- **Security Contexts**: RunAsUser, RunAsGroup, ReadOnlyRootFilesystem

#### Service Configuration (MCPServerService)
- **Service Types**: ClusterIP, NodePort, LoadBalancer
- **Port Configuration**: Configurable ports with validation (1-65535)
- **Protocol Support**: TCP/UDP with annotations

#### Health Monitoring (MCPServerHealthCheck)
- **Configurable Health Checks**: HTTP path-based with customizable parameters
- **Thresholds**: Failure/success thresholds, timeouts, and intervals
- **Defaults**: Sensible defaults (/health path, 30s initial delay)

#### Enterprise Features (MCPServerPodTemplate)
- **Pod Customization**: Labels, annotations, node selectors
- **Scheduling**: Affinity/anti-affinity rules and tolerations
- **Integration**: Service accounts and image pull secrets
- **Storage**: Volume mounts and additional volumes

#### MCP-Specific Configuration (MCPServerConfiguration)
- **Connection Management**: Max connections and timeout configuration
- **Observability**: Log levels, metrics collection, custom metrics paths
- **Extensibility**: Custom configuration key-value pairs

### MCPServerStatus

The status reports observed state with:

#### State Tracking
- **Phase Enum**: Pending, Creating, Running, Scaling, Updating, Failed, Terminating
- **Replica Counts**: Current, ready, and available replicas
- **Conditions**: Standard Kubernetes condition types (Ready, Available, Progressing, Degraded, Reconciled)

#### Operational Data
- **Service Endpoints**: Accessible endpoint information
- **Reconciliation**: Last reconcile time and observed generation
- **Messages**: Human-readable status information

#### Metrics and Monitoring (MCPServerMetrics)
- **Connection Metrics**: Current connections and total requests
- **Performance**: Error rates and average response times
- **Resource Usage**: Current CPU and memory consumption

## Rationale

### Design Principles

1. **Kubernetes Native**: Follows standard Kubernetes API conventions and patterns
2. **Enterprise Ready**: Comprehensive security, RBAC, and operational features
3. **Extensible**: Custom configuration options for MCP-specific needs
4. **Observable**: Rich status reporting and metrics integration
5. **Validated**: Proper kubebuilder markers for CRD validation and defaults

### Key Design Decisions

#### Security First
- Multi-layered security approach with user/group access control
- Rate limiting to prevent abuse and ensure fair resource usage
- TLS support for secure communication
- Container security contexts following security best practices

#### Operational Excellence
- Comprehensive health checking with configurable parameters
- Rich status reporting for operational visibility
- Metrics collection for monitoring and alerting
- Phase-based lifecycle management

#### Enterprise Integration
- Pod template specifications for enterprise scheduling requirements
- Service account integration for RBAC
- Volume mounting for configuration and data persistence
- Annotation support for integration with enterprise tooling

#### MCP Protocol Support
- Capabilities array to define server functionality
- Connection management for protocol-specific requirements
- Custom configuration for MCP server parameters
- Extensibility for future MCP protocol enhancements

## Consequences

### Positive
- Comprehensive API covering all enterprise requirements
- Standard Kubernetes patterns ensure familiarity for platform teams
- Rich validation prevents misconfigurations
- Extensible design allows for future enhancements
- Strong security model suitable for enterprise environments

### Negative
- Complex API surface may require documentation and examples
- Many optional fields might overwhelm new users
- Comprehensive validation requires thorough testing

### Mitigation
- Provide comprehensive examples and documentation
- Use sensible defaults to reduce configuration burden
- Implement progressive disclosure in tooling and documentation
- Create validation tests for all configuration scenarios

## Example Usage

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: database-tools
  namespace: ai-tools
spec:
  image: "mycompany/db-mcp:v1.2.0"
  replicas: 3
  capabilities: ["database", "analytics"]
  resources:
    limits:
      cpu: "500m"
      memory: "512Mi"
    requests:
      cpu: "100m"
      memory: "128Mi"
  security:
    allowedUsers: ["data-team"]
    allowedGroups: ["analytics-users"]
    tls:
      enabled: true
      secretName: "db-mcp-tls"
  service:
    type: "ClusterIP"
    port: 8080
  healthCheck:
    path: "/health"
    initialDelaySeconds: 30
    periodSeconds: 10
  configuration:
    logLevel: "info"
    metricsEnabled: true
    maxConnections: 50
    customConfig:
      database_pool_size: "10"
      timeout: "30s"
```

## References

- Kubernetes API Conventions: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- Kubebuilder Documentation: https://book.kubebuilder.io/
- MCP Protocol Specification: https://modelcontextprotocol.io/
- Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
