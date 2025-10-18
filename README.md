# MCP Operator

[![Lint](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml)
[![Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml)
[![E2E Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml)
[![Release](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml)

> **⚠️ Alpha Software - Not Production Ready**
>
> This project is in early development and should be considered **experimental**. While we encourage you to try it out and provide feedback, please do not use it in production environments yet. APIs may change, features may be incomplete, and bugs are expected.
>
> **We welcome your feedback!** Please open issues for bugs, feature requests, or questions.

A Kubernetes operator for managing Model Context Protocol (MCP) servers with features including HTTP/SSE transport, horizontal pod autoscaling, ingress support, and observability.

## Description

The MCP Operator simplifies deploying and managing MCP servers on Kubernetes. Define your MCP servers using a declarative API that handles deployments, services, autoscaling, and monitoring.

**Key Features:**
- **Declarative Management**: Define MCP servers using custom resources
- **HTTP/SSE Transport**: Full support for HTTP and Server-Sent Events
- **Horizontal Pod Autoscaling**: CPU and memory-based autoscaling
- **Pod Security**: Built-in Pod Security Standards compliance
- **Ingress Support**: External access with session management
- **Observability**: Prometheus metrics and Grafana dashboards

## Architecture

The `MCPServer` custom resource manages:

- **Deployments**: Container orchestration with configurable replicas
- **Services**: Network exposure with protocol-specific configuration
- **Horizontal Pod Autoscalers**: Automatic scaling based on resource utilization
- **Ingress Resources**: External access with transport-aware routing
- **Monitoring**: Prometheus metrics and Grafana dashboards

## Quick Start

> 📖 **New to MCP Operator?** Check out the [Getting Started Guide](GETTING_STARTED.md) for a complete tutorial.

### Installation

Install the MCP Operator using the pre-built installer:

```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

### Optional: Enable Monitoring

If you have [Prometheus Operator](https://prometheus-operator.dev/) installed in your cluster, you can enable metrics collection and Grafana dashboards:

```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

This adds:
- **ServiceMonitor**: Automatic Prometheus metrics scraping
- **Grafana Dashboard**: Pre-configured dashboard for operator observability

**Don't have Prometheus Operator?** The operator works fine without it - monitoring is completely optional.

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
    runAsUser: 1000
    runAsGroup: 1000
    readOnlyRootFilesystem: true
    runAsNonRoot: true

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

## Monitoring

The operator exports Prometheus metrics and includes a Grafana dashboard. Install with:

```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

**Key Metrics:**
- `mcpserver_ready_total` - Ready MCP servers
- `mcpserver_replicas` - Replica counts by transport type
- `mcpserver_phase` - Current phase tracking
- `mcpserver_reconcile_duration_seconds` - Controller performance

## API Reference

### MCPServer Spec

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | **Required.** Container image for the MCP server |
| `replicas` | int32 | Number of desired replicas (default: 1) |
| `transport` | object | Transport configuration (HTTP or custom) |
| `resources` | object | CPU and memory resource requirements |
| `hpa` | object | Horizontal Pod Autoscaler configuration |
| `security` | object | Pod security context settings |
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
      path: "/mcp"                  # HTTP endpoint path (default: "/mcp")
      sessionManagement: true       # Enable session affinity for SSE
```

The HTTP transport supports both:
- **SSE (Server-Sent Events)** - For real-time streaming (configured via container args: `--transport sse`)
- **Standard HTTP** - For request/response patterns

## Examples and Samples

The `config/samples/` directory contains examples for different use cases:

- `wikipedia-http.yaml` - Minimal example using Wikipedia MCP server with SSE
- `mcp-basic-example.yaml` - Common production setup with HPA and monitoring
- `mcp-complete-example.yaml` - Complete example showing all available CRD fields

Apply sample configurations:

```sh
kubectl apply -k config/samples/
```

## Security

The operator enforces Pod Security Standards by default:
- **runAsNonRoot**: Containers must run as non-root users
- **No Privilege Escalation**: Blocks privilege escalation
- **Capabilities Dropped**: All Linux capabilities dropped by default

Configure security context per MCPServer:

```yaml
spec:
  security:
    runAsUser: 1000
    runAsGroup: 1000
    readOnlyRootFilesystem: true
```

## Documentation

### User Guides
- **[Getting Started](GETTING_STARTED.md)** - 5-minute quickstart tutorial
- **[Installation Guide](docs/installation.md)** - Detailed installation instructions
- **[Configuration Examples](config/samples/)** - Real-world MCPServer configurations

### Developer Resources
- **[Contributing Guide](CONTRIBUTING.md)** - How to contribute to the project
- **[Release Process](docs/release-process.md)** - Creating new releases
- **[Development Guide](development.md)** - Local development setup

## Support

For bugs, feature requests, or questions, please [open an issue](https://github.com/vitorbari/mcp-operator/issues).

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
