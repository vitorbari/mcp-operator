# Operations Guide

This section covers running MCP servers in production, including monitoring, metrics collection, and troubleshooting.

## Guides

| Document | Description |
|----------|-------------|
| [Monitoring](monitoring.md) | Prometheus metrics and Grafana dashboards |
| [MCP Server Metrics](metrics.md) | Sidecar-based metrics collection |
| [Troubleshooting](troubleshooting.md) | Common issues and solutions |

## Quick Start: Enable Monitoring

### 1. Enable Metrics Collection

Add a single line to your MCPServer:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: "my-registry/mcp-server:latest"
  metrics:
    enabled: true  # Injects mcp-proxy sidecar
```

### 2. Install Monitoring Stack

If you have Prometheus Operator installed, ServiceMonitors are created automatically.

Otherwise, install the monitoring manifests:

```bash
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

### 3. View Metrics

Access Prometheus or Grafana to view:

- `mcpserver_ready_total` - Ready server count
- `mcpserver_phase` - Server lifecycle phases
- `mcpserver_replicas` - Replica counts
- `mcp_requests_total` - MCP request metrics (from sidecar)

## Key Metrics

### Operator Metrics

| Metric | Description |
|--------|-------------|
| `mcpserver_ready_total` | Number of ready MCPServer instances |
| `mcpserver_phase` | Current phase of MCPServer instances |
| `mcpserver_validation_compliant` | Validation compliance status |
| `mcpserver_replicas` | Replica counts by type |
| `mcpserver_reconcile_duration_seconds` | Reconciliation performance |

### Sidecar Metrics

| Metric | Description |
|--------|-------------|
| `mcp_requests_total` | Total HTTP requests by status and method |
| `mcp_request_duration_seconds` | Request latency distribution |
| `mcp_tool_calls_total` | Tool invocations by name |
| `mcp_sse_connections_active` | Active SSE connections |

## Common Issues

### Pods Not Starting

```bash
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

See [Troubleshooting - Deployment Issues](troubleshooting.md#deployment-issues)

### Validation Failing

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.validation}' | jq
```

See [Troubleshooting - Validation Issues](troubleshooting.md#validation-issues)

### Metrics Not Appearing

```bash
kubectl get servicemonitor
kubectl logs -n mcp-operator-system deployment/mcp-operator-controller-manager
```

See [Troubleshooting - Metrics Sidecar Issues](troubleshooting.md#metrics-sidecar-issues)

## See Also

- [Configuration Guide](../configuration.md) - Basic configuration
- [Advanced Configuration](../advanced/README.md) - HPA, security, and more
- [API Reference](../api-reference.md) - Complete field documentation
