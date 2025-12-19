# MCP Operator Helm Chart

Deploy and manage Model Context Protocol (MCP) servers on Kubernetes with automatic protocol validation, horizontal scaling, and built-in observability.

## Features

- **Auto-detection** - Automatically detects transport type and MCP protocol version
- **Protocol Validation** - Ensures your servers are MCP-compliant
- **Horizontal Scaling** - Built-in autoscaling based on CPU and memory
- **Observability** - Prometheus metrics and Grafana dashboards out of the box
- **Production Ready** - Pod security standards and health checks

## Installation

### Prerequisites

- Kubernetes 1.24+
- Helm 3.8+

### Install the Chart

```bash
# See https://github.com/vitorbari/mcp-operator/releases for latest version
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --namespace mcp-operator-system \
  --create-namespace
```

### Verify Installation

```bash
# Check operator pods
kubectl get pods -n mcp-operator-system

# Check CRD is installed
kubectl get crd mcpservers.mcp.mcp-operator.io
```

## Configuration

### Enable Prometheus Monitoring

```bash
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --namespace mcp-operator-system \
  --create-namespace \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

### Custom Resource Limits

```bash
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --namespace mcp-operator-system \
  --create-namespace \
  --set controllerManager.container.resources.limits.cpu=1000m \
  --set controllerManager.container.resources.limits.memory=512Mi
```

### Using a Values File

Create a `values.yaml` file:

```yaml
controllerManager:
  replicas: 2
  container:
    image:
      repository: ghcr.io/vitorbari/mcp-operator
      tag: v0.1.0-alpha.13
    resources:
      limits:
        cpu: 1000m
        memory: 512Mi
      requests:
        cpu: 100m
        memory: 128Mi

prometheus:
  enable: true

grafana:
  enabled: true
```

Install with your values:

```bash
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --namespace mcp-operator-system \
  --create-namespace \
  -f values.yaml
```

## Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controllerManager.replicas` | Number of operator replicas | `1` |
| `controllerManager.container.image.repository` | Container image repository | `ghcr.io/vitorbari/mcp-operator` |
| `controllerManager.container.image.tag` | Container image tag | `v0.1.0-alpha.13` |
| `controllerManager.container.resources` | Resource limits and requests | See values.yaml |
| `prometheus.enable` | Enable ServiceMonitor for Prometheus | `true` |
| `prometheus.additionalLabels` | Additional labels for ServiceMonitor | `release: monitoring` |
| `metrics.enable` | Enable metrics endpoint | `true` |
| `grafana.enabled` | Create Grafana dashboard ConfigMap | `true` |
| `crd.enable` | Install CRDs | `true` |
| `crd.keep` | Keep CRDs on uninstall | `true` |
| `rbac.enable` | Create RBAC resources | `true` |

For all available options, see the [values.yaml](https://github.com/vitorbari/mcp-operator/blob/main/dist/chart/values.yaml) file.

## Upgrading

```bash
# Check latest version: https://github.com/vitorbari/mcp-operator/releases
helm upgrade mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.14 \
  --namespace mcp-operator-system \
  --reuse-values
```

Upgrade with new configuration:

```bash
helm upgrade mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.14 \
  --namespace mcp-operator-system \
  --reuse-values \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

## Uninstalling

```bash
# Delete all MCPServer resources first
kubectl delete mcpserver --all --all-namespaces

# Uninstall the operator
helm uninstall mcp-operator --namespace mcp-operator-system
```

**Note:** By default, CRDs are kept even after uninstalling (controlled by `crd.keep: true`). To also remove CRDs:

```bash
kubectl delete crd mcpservers.mcp.mcp-operator.io
```

## Usage

### Deploy Your First MCP Server

Create a file called `my-server.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: wikipedia
  namespace: default
spec:
  image: "mcp/wikipedia-mcp:latest"
  args: ["--transport", "sse", "--port", "3001", "--host", "0.0.0.0"]

  transport:
    type: http
    protocol: sse
    config:
      http:
        port: 3001
        path: "/sse"
```

Apply it:

```bash
kubectl apply -f my-server.yaml
```

Watch it reconcile:

```bash
kubectl get mcpservers -w
```

You should see:

```
NAME        PHASE     REPLICAS   READY   PROTOCOL   VALIDATION   CAPABILITIES                      AGE
wikipedia   Running   1          1       sse        Validated    ["tools","resources","prompts"]   30s
```

### Production Setup with Auto-Scaling

See the [examples directory](./examples/) for more configuration options.

## Monitoring

When `prometheus.enable=true` and `grafana.enabled=true`, the chart creates:

- **ServiceMonitor**: Configures Prometheus to scrape operator metrics
- **Grafana Dashboard**: Pre-configured dashboard showing:
  - MCPServer count by phase
  - Replica distribution by transport
  - Reconciliation performance
  - Protocol distribution

Requires [Prometheus Operator](https://prometheus-operator.dev/) to be installed in your cluster.

## Documentation

- [Getting Started Guide](https://github.com/vitorbari/mcp-operator/blob/main/GETTING_STARTED.md)
- [Configuration Guide](https://github.com/vitorbari/mcp-operator/blob/main/docs/configuration-guide.md)
- [API Reference](https://github.com/vitorbari/mcp-operator/blob/main/docs/api-reference.md)
- [Troubleshooting Guide](https://github.com/vitorbari/mcp-operator/blob/main/docs/troubleshooting.md)

## Support

- **Questions?** [Start a discussion](https://github.com/vitorbari/mcp-operator/discussions)
- **Found a bug?** [Open an issue](https://github.com/vitorbari/mcp-operator/issues)
- **Want to contribute?** See [CONTRIBUTING.md](https://github.com/vitorbari/mcp-operator/blob/main/CONTRIBUTING.md)

## License

Copyright 2025 Vitor Bari.

Licensed under the Apache License, Version 2.0. See [LICENSE](https://github.com/vitorbari/mcp-operator/blob/main/LICENSE) for details.
