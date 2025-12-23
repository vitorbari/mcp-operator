# MCP Operator

A production-ready Kubernetes operator for deploying and managing Model Context Protocol (MCP) servers with automatic protocol validation, horizontal scaling, and built-in observability.

## What is MCP Operator?

MCP Operator makes it easy to run MCP servers in Kubernetes. Just define your server using a simple YAML file, and the operator handles the deployment, scaling, monitoring, and protocol validation for you.

**Key Features:**
- **Auto-detection** - Automatically detects transport type and MCP protocol version
- **Protocol Validation** - Ensures your servers are MCP-compliant
- **Horizontal Scaling** - Built-in autoscaling based on CPU and memory
- **Observability** - Prometheus metrics and Grafana dashboards out of the box
- **Production Ready** - Pod security standards and health checks

## Quick Start

### Installation

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Install
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace
```

### Deploy Your First MCP Server

Create a file called `my-server.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "your-registry.company.com/your-mcp-server:v1.0.0"
  # Operator handles validation, scaling, monitoring
```

Apply it:

```bash
kubectl apply -f my-server.yaml
```

Watch it come up:

```bash
kubectl get mcpservers -w
```

You should see:

```
NAME            PHASE     REPLICAS   READY   PROTOCOL   VALIDATION   CAPABILITIES              AGE
my-mcp-server   Running   1          1       sse        Validated    ["tools","resources"]     45s
```

## Configuration Examples

### Production Setup with Auto-Scaling

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
    protocol: auto  # Auto-detect protocol
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

### Enable Monitoring

Install with Prometheus and Grafana dashboards:

```bash
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

This requires [Prometheus Operator](https://prometheus-operator.dev/) to be installed in your cluster.

### Custom Values

Override default settings:

```bash
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --set controllerManager.replicas=2 \
  --set controllerManager.container.resources.limits.cpu=1000m \
  --set metrics.enable=true
```

Or use a values file:

```bash
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  -f my-values.yaml
```

## Transport Configuration

The operator supports both modern and legacy MCP protocols:

**Auto-detection (recommended):**
```yaml
transport:
  type: http
  protocol: auto  # Automatically chooses the best protocol
```

**Force Streamable HTTP (modern):**
```yaml
transport:
  type: http
  protocol: streamable-http
```

**Force SSE (legacy):**
```yaml
transport:
  type: http
  protocol: sse
```

## Protocol Validation

The operator automatically validates your MCP servers to ensure they're protocol-compliant. Enable strict mode to fail deployments that don't pass validation:

```yaml
spec:
  validation:
    strictMode: true
```

## Upgrading

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Upgrade
helm upgrade mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system
```

## Uninstalling

```bash
helm uninstall mcp-operator -n mcp-operator-system
```

**Note:** By default, CRDs are kept after uninstall to prevent accidental data loss. To also remove CRDs:

```bash
kubectl delete crd mcpservers.mcp.mcp-operator.io
```

## What Gets Created

When you install this chart, it creates:

- **Deployment** - The operator controller manager
- **ServiceAccount** - For operator RBAC
- **ClusterRole & ClusterRoleBinding** - RBAC permissions
- **CRDs** - MCPServer custom resource definition
- **Service** - Metrics endpoint (if enabled)
- **ServiceMonitor** - Prometheus scraping (if enabled)
- **ConfigMap** - Grafana dashboard (if enabled)
