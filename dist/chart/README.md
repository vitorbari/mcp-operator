# MCP Operator

> **Alpha Software** - APIs may change, features may be incomplete.

A Kubernetes operator for deploying MCP servers.

## Why use this?

**Protocol validation** - The operator connects to your MCP server after deployment and verifies it speaks MCP correctly. It detects which protocol your server uses (SSE or Streamable HTTP), what capabilities it advertises, and catches configuration mistakes early.

**Correct transport configuration** - SSE and Streamable HTTP have different requirements. The operator handles the transport-specific configuration so you don't have to figure out the right paths and settings for each protocol type.

**Observability** - Creates ServiceMonitors and Grafana dashboards for your MCP servers if you have Prometheus Operator installed.

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
  image: "your-registry/your-mcp-server:v1.0.0"
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
    protocol: auto
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

MCP has two HTTP transport protocols: SSE (Server-Sent Events) and Streamable HTTP. The operator can auto-detect which one your server uses.

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
