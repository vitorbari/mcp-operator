# MCP Server Metrics

Collect Prometheus metrics from your MCP servers with zero code changes using the mcp-proxy sidecar.

## Quick Start (30 seconds)

Enable metrics collection by adding a single line to your MCPServer:

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

The operator automatically injects a sidecar proxy that:
- Intercepts all MCP traffic
- Parses JSON-RPC messages
- Exposes Prometheus metrics on port 9090

## How It Works

When `metrics.enabled: true`, the operator injects an `mcp-proxy` sidecar container:

```
┌─────────────────────────────────────────────────────────┐
│ MCPServer Pod                                           │
│                                                         │
│  ┌─────────────────────┐    ┌─────────────────────┐    │
│  │   mcp-proxy         │    │   MCP Server        │    │
│  │   (sidecar)         │───▶│   (your image)      │    │
│  │                     │    │                     │    │
│  │ :8080 (MCP)         │    │ :3001               │    │
│  │ :9090 (metrics)     │    └─────────────────────┘    │
│  └─────────────────────┘                               │
│           ▲                                            │
└───────────│────────────────────────────────────────────┘
            │
    K8s Service (:8080, :9090)
            │
    ┌───────┴───────┐
    │               │
 Clients       Prometheus
```

## Available Metrics

### Request Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_requests_total` | Counter | `status`, `method` | Total HTTP requests by status code and MCP method |
| `mcp_request_duration_seconds` | Histogram | - | Request latency distribution |
| `mcp_request_size_bytes` | Histogram | - | Request body size distribution |
| `mcp_response_size_bytes` | Histogram | - | Response body size distribution |

### Connection Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_active_connections` | Gauge | - | Current number of active connections |
| `mcp_sse_connections_total` | Counter | - | Total SSE connections opened |
| `mcp_sse_connections_active` | Gauge | - | Current active SSE connections |
| `mcp_sse_connection_duration_seconds` | Histogram | - | SSE connection duration |
| `mcp_sse_events_total` | Counter | `event_type` | Total SSE events by type |

### MCP-Specific Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_tool_calls_total` | Counter | `tool_name` | Tool invocations by tool name |
| `mcp_resource_reads_total` | Counter | `resource_uri` | Resource reads by URI |
| `mcp_request_errors_total` | Counter | `method`, `error_code` | JSON-RPC errors by method and code |

### Proxy Info

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_proxy_info` | Gauge | `version`, `target` | Static proxy information (always 1) |

## Scraping Metrics

### With ServiceMonitor (Prometheus Operator)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: mcp-server-metrics
  labels:
    release: prometheus  # Match your Prometheus selector
spec:
  selector:
    matchLabels:
      app: my-mcp-server
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### Direct Scraping

Add to your Prometheus configuration:

```yaml
scrape_configs:
  - job_name: 'mcp-servers'
    kubernetes_sd_configs:
      - role: service
    relabel_configs:
      - source_labels: [__meta_kubernetes_service_label_app]
        regex: .*mcp.*
        action: keep
      - source_labels: [__meta_kubernetes_service_port_name]
        regex: metrics
        action: keep
```

## Example Grafana Queries

### Request Rate by Method

```promql
sum(rate(mcp_requests_total[5m])) by (method)
```

### Average Request Latency

```promql
histogram_quantile(0.95, rate(mcp_request_duration_seconds_bucket[5m]))
```

### Tool Usage Breakdown

```promql
topk(10, sum(rate(mcp_tool_calls_total[1h])) by (tool_name))
```

### Error Rate

```promql
sum(rate(mcp_request_errors_total[5m])) / sum(rate(mcp_requests_total[5m])) * 100
```

### Active Connections Over Time

```promql
mcp_active_connections
```

### SSE Connection Duration (P99)

```promql
histogram_quantile(0.99, rate(mcp_sse_connection_duration_seconds_bucket[5m]))
```

## Example Prometheus Alerts

```yaml
groups:
- name: mcp-server-alerts
  rules:
  # High error rate
  - alert: MCPServerHighErrorRate
    expr: |
      sum(rate(mcp_request_errors_total[5m]))
      / sum(rate(mcp_requests_total[5m])) > 0.05
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "MCP server error rate above 5%"
      description: "{{ $labels.instance }} has error rate of {{ $value | humanizePercentage }}"

  # High latency
  - alert: MCPServerHighLatency
    expr: |
      histogram_quantile(0.95, rate(mcp_request_duration_seconds_bucket[5m])) > 1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "MCP server P95 latency above 1s"
      description: "{{ $labels.instance }} has P95 latency of {{ $value | humanizeDuration }}"

  # No active connections (potential issue)
  - alert: MCPServerNoConnections
    expr: mcp_active_connections == 0
    for: 10m
    labels:
      severity: info
    annotations:
      summary: "MCP server has no active connections"
      description: "{{ $labels.instance }} has had no connections for 10 minutes"

  # Sidecar down
  - alert: MCPProxySidecarDown
    expr: up{job="mcp-servers"} == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "MCP proxy sidecar is down"
      description: "Cannot scrape metrics from {{ $labels.instance }}"
```

## Advanced Configuration

For custom metrics port, resource limits, or TLS termination, see the [Sidecar Architecture Guide](sidecar-architecture.md).

```yaml
spec:
  metrics:
    enabled: true
    port: 9091  # Custom metrics port
  sidecar:
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 256Mi
```

## Troubleshooting

### Metrics not appearing

1. Verify the sidecar is running:
   ```bash
   kubectl get pods -l app=my-mcp-server -o jsonpath='{.items[0].spec.containers[*].name}'
   # Should show: mcp-server mcp-proxy
   ```

2. Check sidecar logs:
   ```bash
   kubectl logs -l app=my-mcp-server -c mcp-proxy
   ```

3. Test metrics endpoint directly:
   ```bash
   kubectl port-forward svc/my-mcp-server 9090:9090
   curl http://localhost:9090/metrics
   ```

### High cardinality warnings

If you see cardinality warnings from Prometheus, consider:
- The `tool_name` and `resource_uri` labels can have many values
- Use recording rules to pre-aggregate high-cardinality metrics
- Configure label dropping in your scrape config if needed

## Next Steps

- [Sidecar Architecture](sidecar-architecture.md) - Deep dive into how the sidecar works
- [Monitoring Guide](monitoring.md) - Operator-level metrics and dashboards
- [Configuration Guide](configuration-guide.md) - All MCPServer options
