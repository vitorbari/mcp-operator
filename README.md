# MCP Operator

A Kubernetes operator for managing Model Context Protocol (MCP) servers with enterprise-grade features including horizontal pod autoscaling, RBAC, ingress support, transport configuration, and comprehensive observability.

## Description

The MCP Operator simplifies the deployment and management of MCP servers on Kubernetes clusters. It provides a declarative API through custom resources that abstract away the complexity of managing deployments, services, RBAC, autoscaling, and monitoring configurations.

**Key Features:**
- **Declarative Management**: Define MCP servers using Kubernetes custom resources
- **Transport Support**: HTTP streamable and custom transport protocols with automatic service configuration
- **Horizontal Pod Autoscaling**: Built-in HPA support with CPU and memory metrics
- **Enterprise Security**: RBAC integration with user and group access controls
- **Ingress Support**: Automatic ingress creation with transport-specific annotations and MCP traffic analytics
- **Production Monitoring**: Prometheus metrics, Grafana dashboards, and comprehensive observability
- **Flexible Configuration**: Support for custom environments, volumes, and networking

## Architecture

The MCP Operator introduces the `MCPServer` custom resource that declaratively manages:

- **Kubernetes Deployments**: Container orchestration with configurable replicas
- **Services**: Network exposure with transport-specific ports and protocols
- **ServiceAccounts & RBAC**: Fine-grained security controls
- **Horizontal Pod Autoscalers**: Automatic scaling based on resource utilization
- **Ingress Resources**: External access with MCP-aware traffic routing
- **Monitoring**: Prometheus metrics and Grafana dashboards

## Quick Start

### Installation

Install the MCP Operator using the pre-built installer:

```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

### Basic MCP Server

Create a simple MCP server:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
  namespace: default
spec:
  image: "my-registry/mcp-server:v1.0.0"
  replicas: 2
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
```

Apply it to your cluster:

```sh
kubectl apply -f my-mcp-server.yaml
```

## MCPServer Resource Examples

### HTTP Transport with Ingress

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: http-mcp-server
spec:
  image: "mcp/wikipedia-mcp:latest"
  transport:
    type: http
    config:
      http:
        port: 8080
        sessionManagement: true

  # External access via ingress
  ingress:
    enabled: true
    host: "mcp.example.com"
    path: "/"
    className: "nginx"
    tls:
      - secretName: "mcp-tls"
        hosts:
          - "mcp.example.com"
    annotations:
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
```

### Custom Transport with TCP Protocol

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: custom-mcp-server
spec:
  image: "my-registry/custom-mcp:v1.0.0"
  transport:
    type: custom
    config:
      custom:
        protocol: "tcp"
        port: 9000
        config:
          buffer_size: "1024"
          timeout: "30s"
```

### Production Setup with HPA and Monitoring

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: production-mcp-server
spec:
  image: "my-registry/mcp-server:v2.0.0"
  replicas: 3

  # Transport configuration
  transport:
    type: http
    config:
      http:
        port: 8080
        sessionManagement: true

  # Horizontal Pod Autoscaler
  hpa:
    enabled: true
    minReplicas: 3
    maxReplicas: 20
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80
    scaleUpBehavior:
      stabilizationWindowSeconds: 60
      policies:
        - type: "Percent"
          value: 100
          periodSeconds: 15
    scaleDownBehavior:
      stabilizationWindowSeconds: 300
      policies:
        - type: "Percent"
          value: 10
          periodSeconds: 60

  # Security Configuration
  security:
    allowedUsers:
      - "admin"
      - "mcp-user"
    allowedGroups:
      - "mcp-operators"
      - "data-scientists"

  # Service Configuration
  service:
    type: "ClusterIP"
    port: 8080
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"

  # Health Checks
  healthCheck:
    enabled: true
    path: "/health"
    port: 8080
    initialDelaySeconds: 30
    periodSeconds: 10
    timeoutSeconds: 5
    failureThreshold: 3

  # Resource Requirements
  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"

  # Environment Variables
  environment:
    - name: "LOG_LEVEL"
      value: "info"
    - name: "MCP_PORT"
      value: "8080"
    - name: "METRICS_ENABLED"
      value: "true"

  # Ingress with monitoring
  ingress:
    enabled: true
    host: "api.example.com"
    path: "/mcp"
    className: "nginx"
    annotations:
      nginx.ingress.kubernetes.io/rate-limit: "100"
      nginx.ingress.kubernetes.io/rate-limit-window: "1m"
```

## Monitoring and Observability

The MCP Operator provides comprehensive monitoring capabilities:

### Prometheus Metrics

Automatic metrics collection includes:
- `mcpserver_ready_total` - Number of ready MCP servers
- `mcpserver_replicas` - Current replica count per server and transport type
- `mcpserver_transport_type_total` - Transport type distribution
- `mcpserver_reconcile_duration_seconds` - Controller reconciliation timing
- `mcpserver_phase` - Current phase tracking (Creating, Running, Scaling, Failed)
- `mcpserver_resource_requests` - CPU and memory resource allocation

### Grafana Dashboard

The operator includes a pre-built Grafana dashboard with:
- MCPServer status and health overview
- Transport type distribution analytics
- Replica count and scaling trends
- Controller performance metrics
- Resource utilization tracking

Dashboard is automatically deployed as a ConfigMap that Grafana can discover.

### Ingress Analytics

Transport-specific ingress annotations provide:
- MCP protocol version tracking
- Session management analytics
- Request routing and load balancing metrics
- Advanced structured logging for traffic analysis

## API Reference

### MCPServer Spec

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | **Required.** Container image for the MCP server |
| `replicas` | int32 | Number of desired replicas (default: 1) |
| `transport` | object | Transport configuration (HTTP or custom) |
| `resources` | object | CPU and memory resource requirements |
| `hpa` | object | Horizontal Pod Autoscaler configuration |
| `security` | object | RBAC and access control settings |
| `service` | object | Service exposure configuration |
| `ingress` | object | Ingress configuration for external access |
| `healthCheck` | object | Health check probe configuration |
| `environment` | []object | Environment variables |
| `podTemplate` | object | Additional pod template specifications |

### MCPServer Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase: Creating, Running, Updating, Scaling, Failed, Terminating |
| `replicas` | int32 | Current total replica count |
| `readyReplicas` | int32 | Number of ready replicas |
| `availableReplicas` | int32 | Number of available replicas |
| `transportType` | string | Active transport type |
| `serviceEndpoint` | string | Service endpoint URL |
| `conditions` | []object | Detailed status conditions |
| `lastReconcileTime` | timestamp | Last reconciliation timestamp |

## Transport Configuration

### HTTP Transport

```yaml
transport:
  type: http
  config:
    http:
      port: 8080                    # HTTP port (default: 8080)
      sessionManagement: true       # Enable session affinity
```

### Custom Transport

```yaml
transport:
  type: custom
  config:
    custom:
      protocol: "tcp"               # Protocol: tcp, udp, sctp
      port: 9000                    # Custom port
      config:                       # Protocol-specific configuration
        buffer_size: "1024"
        timeout: "30s"
```

## Examples and Samples

The `config/samples/` directory contains comprehensive examples:

- `basic-mcpserver.yaml` - Simple MCP server deployment
- `http-mcp-server-ingress.yaml` - HTTP transport with ingress
- `custom-http-ingress.yaml` - Custom transport configuration
- `monitoring-metrics-example.yaml` - Full monitoring setup
- `wikipedia.yaml` - Real-world Wikipedia MCP server example

Apply sample configurations:

```sh
kubectl apply -k config/samples/
```

## Advanced Topics

### Transport Manager Architecture

The operator uses a transport manager pattern to handle different MCP protocols:
- **HTTP Manager**: Optimized for MCP-over-HTTP with streaming support
- **Custom Manager**: Flexible configuration for TCP/UDP/SCTP protocols

### Resource Management

Automatic resource management includes:
- Transport-specific service creation
- Protocol-aware port allocation
- Load balancer annotations for cloud providers
- Health check configuration per transport type

### Security Model

Multi-layered security approach:
- **RBAC Integration**: Fine-grained user and group access controls
- **Network Policies**: Optional traffic isolation
- **Service Mesh**: Compatible with Istio and Linkerd
- **TLS Termination**: Automatic certificate management with cert-manager

## Documentation

- **[Development Guide](development.md)** - Building, testing, and contributing
- **[API Reference](docs/)** - Complete API documentation
- **[Architecture Decision Records](docs/)** - Design decisions and rationale
- **[Examples](config/samples/)** - Real-world configuration examples

## Support and Community

- **Issues**: [GitHub Issues](https://github.com/vitorbari/mcp-operator/issues)
- **Discussions**: [GitHub Discussions](https://github.com/vitorbari/mcp-operator/discussions)
- **Documentation**: [Project Wiki](https://github.com/vitorbari/mcp-operator/wiki)

## License

Copyright 2025 Vitor Bari.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.