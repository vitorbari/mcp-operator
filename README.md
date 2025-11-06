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

A Kubernetes operator for managing Model Context Protocol (MCP) servers with features including HTTP/SSE transport, protocol validation, horizontal pod autoscaling, ingress support, and observability.

## Description

The MCP Operator simplifies deploying and managing MCP servers on Kubernetes. Define your MCP servers using a declarative API that handles deployments, services, autoscaling, and monitoring.

**Key Features:**
- **Declarative Management**: Define MCP servers using custom resources
- **HTTP/SSE Transport**: Full support for HTTP and Server-Sent Events
- **Protocol Validation**: Built-in compliance checking with optional strict mode
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

### Grafana Dashboard

The dashboard displays real-time metrics for all MCPServers including readiness, phase distribution, replica counts, controller performance, and resource utilization.

<img width="2540" height="1324" alt="Screenshot 2025-10-19 at 20 58 00" src="https://github.com/user-attachments/assets/277ff6d9-e9ff-4fdb-ad76-b40077ae942e" />


## API Reference

### MCPServer Spec

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | **Required.** Container image for the MCP server |
| `replicas` | int32 | Number of desired replicas (default: 1) |
| `transport` | object | Transport configuration with protocol specification |
| `validation` | object | MCP protocol validation configuration |
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

**Default Behavior:** If no `transport` is specified, the operator defaults to HTTP transport with auto-detection of the MCP protocol (prefers Streamable HTTP over SSE), port 8080, and creates a ClusterIP Service automatically.

### MCP Protocol Specification

The operator supports explicit protocol specification or auto-detection:

```yaml
transport:
  type: http                        # Transport type
  protocol: auto                    # MCP protocol: auto, streamable-http, or sse (default: auto)
  config:
    http:
      port: 8080                    # HTTP port (default: 8080)
      path: "/mcp"                  # HTTP endpoint path (default: "/mcp")
      sessionManagement: true       # Enable session affinity
```

**Protocol Options:**
- **`auto`** (default) - Auto-detect and prefer Streamable HTTP over SSE
- **`streamable-http`** - Use Streamable HTTP transport (MCP 2025-03-26+)
- **`sse`** - Use Server-Sent Events transport (MCP 2024-11-05)

**Protocol Details:**
- **Streamable HTTP** - Modern MCP protocol (2025-03-26+) that supports both JSON and SSE response formats
- **SSE (Server-Sent Events)** - Legacy MCP protocol (2024-11-05) for real-time streaming

### Example: Explicit Protocol Selection

**Force Streamable HTTP:**
```yaml
transport:
  type: http
  protocol: streamable-http
  config:
    http:
      port: 8080
      path: "/mcp"
      sessionManagement: true
```

**Force SSE (Legacy):**
```yaml
transport:
  type: http
  protocol: sse
  config:
    http:
      port: 8080
      path: "/sse"
```

## Protocol Validation

The MCP Operator includes built-in validation to ensure your MCP servers are protocol-compliant and correctly configured. This helps catch configuration errors early and ensures reliable MCP deployments.

### Features

- **Automatic Protocol Detection**: Validates transport endpoints (Streamable HTTP, SSE)
- **Protocol Mismatch Detection**: Warns when configured protocol doesn't match actual server implementation
- **Authentication Detection**: Identifies servers requiring authentication
- **Retry Logic**: Retries transient failures (up to 5 attempts) before marking as failed
- **Strict Mode**: Optionally prevents non-compliant servers from running

### Basic Validation

Enable validation to verify your MCP server responds correctly:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: validated-server
spec:
  image: "my-registry/mcp-server:v1"
  transport:
    type: http
    protocol: auto
    config:
      http:
        port: 8080
        path: "/mcp"
  validation:
    enabled: true           # Enable protocol validation
    strictMode: false       # Allow non-compliant servers (default)
```

### Strict Mode

When strict mode is enabled, the operator will delete the deployment if validation fails after all retry attempts:

```yaml
validation:
  enabled: true
  strictMode: true          # Delete deployment if validation fails
```

**Strict Mode Behavior:**
- Deployment is created and pods start normally
- Operator validates MCP protocol compliance
- If validation fails after 5 attempts (~4.5 minutes):
  - Deployment is deleted
  - Phase transitions to `ValidationFailed`
  - No further retries until spec is updated
- Update the spec to fix configuration and trigger revalidation

### Validation Status

Check validation results in the MCPServer status:

```sh
kubectl get mcpserver my-server -o jsonpath='{.status.validation}'
```

Example output:

```json
{
  "state": "Compliant",
  "compliant": true,
  "attempts": 1,
  "lastValidated": "2025-11-03T20:00:00Z",
  "requiresAuth": false,
  "protocol": "streamable-http",
  "transport": {
    "lastDetected": "2025-11-03T20:00:00Z"
  }
}
```

**Validation States:**
- `Pending` - Initial state, validation not yet started
- `Validating` - Actively validating the server
- `Compliant` - Server is MCP protocol compliant
- `Failed` - Validation failed (see issues for details)

### Viewing Validation Issues

If validation fails, check the status for detailed error information:

```sh
kubectl get mcpserver my-server -o jsonpath='{.status.validation.issues}'
```

Example issues:

```json
[
  {
    "code": "TRANSPORT_DETECTION_FAILED",
    "level": "error",
    "message": "Failed to detect transport: could not connect to http://my-server.default.svc.cluster.local:8080/mcp"
  },
  {
    "code": "PROTOCOL_MISMATCH",
    "level": "warning",
    "message": "Protocol mismatch: configured streamable-http but detected sse"
  }
]
```

### Common Validation Scenarios

**Scenario 1: Wrong Port Configuration**
```yaml
# Server listens on 3001, but configured as 8080
transport:
  config:
    http:
      port: 8080  # Wrong port!
```
Result: `TRANSPORT_DETECTION_FAILED` - Fix the port and validation will succeed on the next attempt.

**Scenario 2: Protocol Mismatch**
```yaml
# Server implements SSE, but configured as streamable-http
transport:
  protocol: streamable-http  # Mismatch!
```
Result: `PROTOCOL_MISMATCH` warning - Server runs in non-strict mode, but update to `protocol: sse` for correct configuration.

**Scenario 3: Authentication Required**
```yaml
# Server requires auth but no credentials provided
validation:
  enabled: true
```
Result: Validation detects `requiresAuth: true` - Add authentication configuration to your server setup.

### Events and Monitoring

Monitor validation progress through Kubernetes events:

```sh
kubectl get events --field-selector involvedObject.name=my-server
```

Example events:
```
Normal   ValidationStarted   Validation started for MCP server
Warning  ValidationRetry     Validation failed (attempt 2/5), will retry
Warning  ValidationFailed    Validation failed after 5/5 attempts
Warning  DeploymentDeleted   Deleted deployment after 5 validation attempts in strict mode
Normal   ValidationSuccess   MCP server passed protocol validation
```

### Best Practices

1. **Enable Validation in Development**: Catch configuration errors early
2. **Use Strict Mode in Production**: Ensure only compliant servers run
3. **Monitor Validation Status**: Set up alerts for `ValidationFailed` phase
4. **Update Specs to Retry**: Fix issues and update spec to trigger revalidation
5. **Check Auth Requirements**: Validation will detect if authentication is needed

### Advanced Configuration

The validation retry behavior can be customized via environment variables on the controller deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-operator-controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: MCP_MAX_VALIDATION_ATTEMPTS
          value: "5"  # Default: 5 attempts for transient errors
        - name: MCP_MAX_PERMANENT_ERROR_ATTEMPTS
          value: "2"  # Default: 2 attempts for permanent errors
```

**Use cases:**
- **Testing environments**: Reduce retry counts (e.g., `"3"`) to speed up E2E tests
- **Production with slow services**: Increase retry counts (e.g., `"10"`) for services with long startup times
- **Fail-fast scenarios**: Lower both values to `"1"` for immediate failure detection

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
