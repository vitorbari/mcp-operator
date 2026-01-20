# MCP Operator

[![Lint](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml)
[![Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml)
[![E2E Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml)
[![Release](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml)
[![Awesome MCP](https://awesome.re/mentioned-badge.svg)](https://github.com/punkpeye/awesome-mcp-devtools)

> **⚠️ Alpha Software**
>
> This project is in early development. APIs may change, features may be incomplete, and bugs are expected. We'd appreciate your feedback via [issues](https://github.com/vitorbari/mcp-operator/issues).

A Kubernetes operator for deploying MCP servers.

![demo](https://github.com/user-attachments/assets/81a95736-11fe-450b-bb57-ad4d2db2a9ac)

## Why use this?

**Protocol validation** - The operator connects to your MCP server after deployment and verifies it actually speaks MCP. It detects which protocol your server uses (SSE or Streamable HTTP), what capabilities it advertises, and whether it requires authentication. This catches configuration mistakes early - wrong port, wrong path, server not actually running MCP, etc.

**Correct transport configuration** - SSE and Streamable HTTP have different requirements. The operator handles the transport-specific configuration (paths, session management, keep-alive settings) so you don't have to figure out the right Service annotations or health check paths for each protocol type.

**Observability** - If you have Prometheus Operator installed, the operator creates ServiceMonitors and Grafana dashboards for your MCP servers. There's also an optional metrics sidecar that can collect MCP-specific metrics (request counts, latencies, etc.)

**Standard Kubernetes resources** - Under the hood, it creates Deployments, Services, ServiceAccounts, and HPAs. Nothing proprietary.

## What this doesn't do

- **Ingress/external exposure** - Creates a ClusterIP Service by default. You need to create your own Ingress, Gateway, or change the Service type to LoadBalancer if you want external access.
- **Authentication** - The operator detects if your server requires auth, but doesn't handle authentication itself. You need to configure auth at your server or ingress layer.
- **stdio transport** - Only supports HTTP-based transports (SSE and Streamable HTTP). If your MCP server uses stdio, you can wrap it with an adapter like [supergateway](https://github.com/supercorp-ai/supergateway) that exposes it as SSE or Streamable HTTP, then deploy that container with this operator. See the [containerizing guide](docs/containerizing.md) for details.
- **MCP client** - This deploys servers, not clients. It doesn't help you connect to MCP servers from your applications.
- **TLS termination** - Doesn't configure TLS by default. You can enable TLS termination via the metrics sidecar (`spec.sidecar.tls`) if you're using metrics, or use an ingress controller.

## Quick Start

See the [Getting Started Guide](GETTING_STARTED.md) for a complete walkthrough.

### Installation

Two options:

#### Option 1: Install via Helm (Recommended)

```sh
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Install
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace
```

#### Option 2: Install via kubectl

```sh
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name')

# Install from release assets
kubectl apply -f https://github.com/vitorbari/mcp-operator/releases/download/${VERSION}/install.yaml
```

Helm is easier to configure. kubectl has fewer dependencies.

### Your First MCPServer

Create `my-server.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "your-registry/your-mcp-server:v1.0.0"
```

Apply it:

```sh
kubectl apply -f my-server.yaml
```

Watch the status:

```sh
kubectl get mcpservers -w
```

Output:

```
NAME            PHASE     REPLICAS   READY   PROTOCOL   VALIDATION   CAPABILITIES                      AGE
my-mcp-server   Running   1          1       sse        Validated    ["tools","resources","prompts"]   109s
```

The operator created a Deployment and Service. It also connected to your server to validate it speaks MCP and detected its capabilities.

## What Gets Created

When you create an MCPServer, the operator creates:

- **Deployment** - Runs your server container with configured health checks
- **Service** - ClusterIP service for in-cluster access (configurable to NodePort or LoadBalancer)
- **ServiceAccount** - For pod identity
- **HPA** - If `hpa.enabled: true`, creates a HorizontalPodAutoscaler

The operator also runs validation against your server to check protocol compliance.

## Examples

### With Auto-Scaling

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "tzolov/mcp-everything-server:v3"
  command: ["node", "dist/index.js", "sse"]

  transport:
    type: http
    protocol: auto  # Let the operator detect the protocol
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true

  # Scale between 2-10 pods based on CPU
  hpa:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70

  # Pod security
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true

  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"
```

## Protocol Validation

The operator automatically validates your MCP servers to ensure they're protocol-compliant. It checks transport connectivity, protocol version, authentication requirements, and available capabilities.

Enable strict mode to fail deployments that don't pass validation:

```yaml
spec:
  validation:
    strictMode: true
```

For detailed validation behavior, see the [Validation Behavior Guide](docs/validation-behavior.md).

## Monitoring

Requires [Prometheus Operator](https://prometheus-operator.dev/) to be installed in your cluster.

**With Helm:**
```sh
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

**With kubectl:**
```sh
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name')

kubectl apply -f https://github.com/vitorbari/mcp-operator/releases/download/${VERSION}/monitoring.yaml
```

This creates a ServiceMonitor (so Prometheus scrapes the operator metrics) and a ConfigMap with a Grafana dashboard.

<img width="3452" height="3726" alt="Grafana dashboard showing MCP server metrics" src="https://github.com/user-attachments/assets/f81ed38e-a03d-4a3b-aa72-727487e6c2ff" />

See the [Monitoring Guide](docs/monitoring.md) for details.

## MCP Server Metrics

Enable per-server metrics collection with a single line:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: your-registry/your-mcp-server:latest
  transport:
    type: http
    config:
      http:
        port: 3001
  metrics:
    enabled: true  # That's it!
```

When enabled, the operator injects a metrics sidecar that automatically tracks:
- Request counts, latencies, and sizes
- Tool calls by tool name
- Resource reads by URI
- JSON-RPC errors by method and code
- SSE connection metrics

See the [MCP Server Metrics Guide](docs/metrics.md) for available metrics, Grafana queries, and alerting examples.

## Transport Configuration

MCP has two HTTP transport protocols: SSE (Server-Sent Events, older) and Streamable HTTP (newer). The operator can auto-detect which one your server uses, or you can specify it explicitly.

**Auto-detection:**
```yaml
transport:
  type: http
  protocol: auto
```

**Explicit SSE:**
```yaml
transport:
  type: http
  protocol: sse
```

**Explicit Streamable HTTP:**
```yaml
transport:
  type: http
  protocol: streamable-http
```

Auto-detection works by trying to connect with each protocol. If you know which protocol your server uses, specifying it explicitly is faster.

## Documentation

### Getting Started
- [Getting Started Guide](GETTING_STARTED.md) - 5-minute walkthrough
- [Installation Guide](docs/installation.md) - Detailed installation options

### Configuration
- [Configuration Guide](docs/configuration-guide.md) - Complete configuration patterns and examples
- [Environment Variables](docs/environment-variables.md) - Configuring environment variables
- [Configuration Examples](config/samples/) - Real-world YAML examples

### Reference
- [API Reference](docs/api-reference.md) - Complete CRD field documentation
- [Validation Behavior](docs/validation-behavior.md) - Protocol validation deep dive

### Operations
- [Troubleshooting Guide](docs/troubleshooting.md) - Common issues and solutions
- [Monitoring Guide](docs/monitoring.md) - Operator metrics, dashboards, and alerts
- [MCP Server Metrics](docs/metrics.md) - Per-server metrics with the sidecar proxy
- [Sidecar Architecture](docs/sidecar-architecture.md) - Technical deep-dive into the metrics sidecar

### Advanced Topics
- [Release Process](docs/release-process.md) - For maintainers
- [Contributing](CONTRIBUTING.md) - Development and contribution guidelines


## Examples and Samples

Check out the `config/samples/` directory for real-world examples:

- **`wikipedia-http.yaml`** - Simple example using the Wikipedia MCP server
- **`mcp-basic-example.yaml`** - Production setup with HPA and monitoring
- **`mcp-complete-example.yaml`** - Shows all available configuration options
- **`mcp_v1_mcpserver_metrics.yaml`** - Simple metrics sidecar example
- **`mcp_v1_mcpserver_metrics_advanced.yaml`** - Advanced sidecar configuration

Apply all samples:

```sh
kubectl apply -k config/samples/
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on bug reports, feature requests, and code contributions.

## Support

- **Found a bug?** [Open an issue](https://github.com/vitorbari/mcp-operator/issues)
- **Have questions?** [Start a discussion](https://github.com/vitorbari/mcp-operator/discussions)

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
