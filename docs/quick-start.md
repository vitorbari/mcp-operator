# Quick Start

Get your first MCP server running on Kubernetes in minutes.

## Prerequisites

- Kubernetes cluster (1.19+)
- kubectl configured
- MCP Operator installed (see [Installation](installation.md))

## Deploy Your First MCPServer

### 1. Create a Minimal MCPServer

```yaml
# my-mcp-server.yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "mcp/wikipedia-mcp:latest"
  command: ["python", "-m", "wikipedia_mcp"]
  args: ["--transport", "sse", "--port", "3001", "--host", "0.0.0.0"]

  transport:
    type: http
    protocol: auto
    config:
      http:
        port: 3001
        path: "/sse"
```

Apply it:

```bash
kubectl apply -f my-mcp-server.yaml
```

### 2. Watch the Server Come Up

```bash
# Watch the MCPServer status
kubectl get mcpserver my-mcp-server -w

# Check pods
kubectl get pods -l app.kubernetes.io/instance=my-mcp-server
```

You should see the phase progress from `Creating` to `Running`.

### 3. Test the Connection

```bash
# Port forward to the service
kubectl port-forward svc/my-mcp-server 3001:3001

# Test the SSE endpoint (in another terminal)
curl -N http://localhost:3001/sse
```

## Development Configuration

For local development and testing, use a minimal configuration:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-dev
  namespace: development
spec:
  image: "myregistry/mcp-server:latest"

  # Single replica for development
  replicas: 1

  # Auto-detect protocol
  transport:
    type: http
    protocol: auto
    config:
      http:
        port: 8080
        path: "/mcp"

  # Minimal resources
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"

  # Debug logging
  environment:
    - name: LOG_LEVEL
      value: "debug"
    - name: ENVIRONMENT
      value: "development"

  # Validation enabled but not strict
  validation:
    enabled: true
    strictMode: false
```

## What's Next?

- [Configuration Guide](configuration.md) - Learn about all configuration options
- [Transport Protocols](transports/README.md) - Understand SSE vs Streamable HTTP
- [Monitoring](operations/monitoring.md) - Enable metrics collection
- [API Reference](api-reference.md) - Complete field documentation

## Troubleshooting

### Pod not starting?

```bash
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

### Server stuck in Creating phase?

Check for image pull errors or resource constraints:

```bash
kubectl get pods -l app.kubernetes.io/instance=my-mcp-server
kubectl describe pod <pod-name>
```

See the full [Troubleshooting Guide](operations/troubleshooting.md) for more help.
