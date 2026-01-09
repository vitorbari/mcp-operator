# MCP Server Metrics Guide

Per-server metrics collection for MCP servers using the mcp-proxy sidecar.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Available Metrics](#available-metrics)
- [Grafana Dashboard](#grafana-dashboard)
- [Example Queries](#example-queries)
- [Prometheus Alerts](#prometheus-alerts)
- [Troubleshooting](#troubleshooting)

## Overview

The MCP Operator can inject a metrics sidecar (mcp-proxy) into your MCP server pods. This sidecar:

- **Intercepts traffic** - Acts as a reverse proxy in front of your MCP server
- **Parses JSON-RPC** - Understands MCP protocol for detailed metrics
- **Exposes Prometheus metrics** - Tool calls, resource reads, latencies, errors
- **Minimal overhead** - Sub-millisecond latency, ~20MB memory

## Quick Start

Enable metrics on any MCPServer with a single line:

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

When `metrics.enabled: true`, the operator:

1. Injects the mcp-proxy sidecar container
2. Routes traffic through the proxy (port 8080 -> 3001)
3. Exposes metrics on port 9090

## Available Metrics

### Request Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_requests_total` | Counter | `method`, `status`, `namespace`, `mcpserver` | Total requests by JSON-RPC method and HTTP status |
| `mcp_request_duration_seconds` | Histogram | `method`, `namespace`, `mcpserver` | Request latency in seconds |
| `mcp_request_size_bytes` | Histogram | `method`, `namespace`, `mcpserver` | Request body size |
| `mcp_response_size_bytes` | Histogram | `method`, `namespace`, `mcpserver` | Response body size |
| `mcp_request_errors_total` | Counter | `method`, `error_code`, `namespace`, `mcpserver` | JSON-RPC errors by method and code |

### Connection Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_active_connections` | Gauge | `namespace`, `mcpserver` | Current active HTTP connections |
| `mcp_proxy_info` | Gauge | `version`, `target`, `namespace`, `mcpserver` | Static proxy metadata |

### MCP-Specific Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_tool_calls_total` | Counter | `tool_name`, `namespace`, `mcpserver` | Tool invocations by tool name |
| `mcp_resource_reads_total` | Counter | `resource_uri`, `namespace`, `mcpserver` | Resource reads by URI |

### SSE Metrics (Legacy Transport)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `mcp_sse_connections_total` | Counter | `namespace`, `mcpserver` | Total SSE connections established |
| `mcp_sse_connections_active` | Gauge | `namespace`, `mcpserver` | Currently active SSE streams |
| `mcp_sse_events_total` | Counter | `event_type`, `namespace`, `mcpserver` | SSE events by type |
| `mcp_sse_connection_duration_seconds` | Histogram | `namespace`, `mcpserver` | How long SSE connections stay open |

## Grafana Dashboard

The MCP Operator Grafana dashboard includes a **Sidecar Metrics** section with the following panels:

### Overview Row
- **Request Rate** - Current requests per second
- **Error Rate** - Percentage of failed requests
- **Active Connections** - HTTP connections
- **SSE Connections** - Active streaming connections
- **p50 Latency** - Median request latency
- **p95 Latency** - 95th percentile latency

### Request Traffic Row
- **Requests by Method** - Traffic breakdown by JSON-RPC method
- **Requests by Status** - Success vs errors by HTTP status code

### Latency Row
- **Latency Percentiles** - p50, p90, p95, p99 over time
- **p95 Latency by Method** - Latency breakdown by JSON-RPC method

### MCP Protocol Usage Row
- **Tool Calls Over Time** - Which tools are being called
- **Top 10 Tools (24h)** - Most used tools
- **Resource Reads Over Time** - Resource access patterns
- **Top 10 Resources (24h)** - Most accessed resources

### SSE Connections Row
- **Active SSE Connections** - Streaming connections over time
- **SSE Connection Duration** - How long connections stay open
- **SSE Events by Type** - Event throughput

### Errors Row
- **Errors by Method** - Which methods are failing
- **Errors by Code** - JSON-RPC error code distribution
- **Error Log (Last Hour)** - Detailed error breakdown table

### Using the Dashboard

1. Use the **Namespace** dropdown to filter by Kubernetes namespace
2. Use the **MCP Server** dropdown to filter by specific servers
3. Expand the "Sidecar Metrics" section (collapsed by default)

## Example Queries

### Request Rate

```promql
# Overall request rate
sum(rate(mcp_requests_total[5m]))

# Request rate by method
sum(rate(mcp_requests_total[5m])) by (method)

# Request rate for specific server
sum(rate(mcp_requests_total{mcpserver="my-server"}[5m]))
```

### Latency

```promql
# p95 latency overall
histogram_quantile(0.95, sum(rate(mcp_request_duration_seconds_bucket[5m])) by (le))

# p95 latency by method
histogram_quantile(0.95, sum(rate(mcp_request_duration_seconds_bucket[5m])) by (le, method))

# Average latency
rate(mcp_request_duration_seconds_sum[5m]) / rate(mcp_request_duration_seconds_count[5m])
```

### Error Rate

```promql
# Error rate percentage
100 * (
  sum(rate(mcp_request_errors_total[5m]))
  /
  sum(rate(mcp_requests_total[5m]))
)

# Errors by code
sum(rate(mcp_request_errors_total[5m])) by (error_code)
```

### Tool Usage

```promql
# Tool call rate by tool name
sum(rate(mcp_tool_calls_total[5m])) by (tool_name)

# Top 10 tools in last 24 hours
topk(10, sum(increase(mcp_tool_calls_total[24h])) by (tool_name))
```

### Resource Access

```promql
# Resource read rate
sum(rate(mcp_resource_reads_total[5m])) by (resource_uri)

# Top 10 resources in last 24 hours
topk(10, sum(increase(mcp_resource_reads_total[24h])) by (resource_uri))
```

### SSE Connections

```promql
# Active SSE connections
sum(mcp_sse_connections_active)

# SSE connection duration p95
histogram_quantile(0.95, sum(rate(mcp_sse_connection_duration_seconds_bucket[5m])) by (le))
```

## Prometheus Alerts

Example alert rules for MCP server metrics:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: mcp-sidecar-alerts
  namespace: mcp-operator-system
spec:
  groups:
    - name: mcp-sidecar
      interval: 30s
      rules:
        # High error rate
        - alert: MCPServerHighErrorRate
          expr: |
            100 * (
              sum(rate(mcp_request_errors_total[5m])) by (mcpserver, namespace)
              /
              sum(rate(mcp_requests_total[5m])) by (mcpserver, namespace)
            ) > 5
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "High error rate for {{ $labels.mcpserver }}"
            description: "MCPServer {{ $labels.mcpserver }} in {{ $labels.namespace }} has error rate {{ $value | humanize }}%"

        # Critical error rate
        - alert: MCPServerCriticalErrorRate
          expr: |
            100 * (
              sum(rate(mcp_request_errors_total[5m])) by (mcpserver, namespace)
              /
              sum(rate(mcp_requests_total[5m])) by (mcpserver, namespace)
            ) > 25
          for: 2m
          labels:
            severity: critical
          annotations:
            summary: "Critical error rate for {{ $labels.mcpserver }}"
            description: "MCPServer {{ $labels.mcpserver }} in {{ $labels.namespace }} has error rate {{ $value | humanize }}%"

        # High latency
        - alert: MCPServerHighLatency
          expr: |
            histogram_quantile(0.95,
              sum(rate(mcp_request_duration_seconds_bucket[5m])) by (le, mcpserver, namespace)
            ) > 2
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "High latency for {{ $labels.mcpserver }}"
            description: "MCPServer {{ $labels.mcpserver }} p95 latency is {{ $value | humanize }}s"

        # No traffic (potential issue)
        - alert: MCPServerNoTraffic
          expr: |
            sum(rate(mcp_requests_total[15m])) by (mcpserver, namespace) == 0
            and
            up{job=~".*mcp.*"} == 1
          for: 30m
          labels:
            severity: info
          annotations:
            summary: "No traffic for {{ $labels.mcpserver }}"
            description: "MCPServer {{ $labels.mcpserver }} has received no requests for 30 minutes"

        # SSE connection issues
        - alert: MCPServerSSEConnectionDrops
          expr: |
            increase(mcp_sse_connections_total[5m]) > 0
            and
            mcp_sse_connections_active == 0
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "SSE connections failing for {{ $labels.mcpserver }}"
            description: "SSE connections are being established but immediately closing"
```

## Advanced Configuration

### Custom Metrics Port

```yaml
spec:
  metrics:
    enabled: true
    port: 9091  # Custom port
```

### Custom Sidecar Resources

```yaml
spec:
  metrics:
    enabled: true
  sidecar:
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 256Mi
```

### Pin Sidecar Version

```yaml
spec:
  metrics:
    enabled: true
  sidecar:
    image: ghcr.io/vitorbari/mcp-proxy:v0.1.0
```

### TLS Termination

```yaml
spec:
  metrics:
    enabled: true
  sidecar:
    tls:
      enabled: true
      secretName: mcp-tls-secret
      minVersion: "1.3"
```

## Troubleshooting

### Metrics Not Appearing

**Check sidecar is running:**

```bash
kubectl get pods -l app=my-mcp-server -o jsonpath='{.items[0].spec.containers[*].name}'
# Should show both: mcp-server mcp-proxy
```

**Check sidecar logs:**

```bash
kubectl logs -l app=my-mcp-server -c mcp-proxy
```

**Test metrics endpoint:**

```bash
kubectl port-forward svc/my-mcp-server 9090:9090
curl http://localhost:9090/metrics | grep mcp_
```

### No Data in Dashboard

1. Verify `metrics.enabled: true` in MCPServer spec
2. Check that Prometheus is scraping the metrics endpoint
3. Ensure namespace and mcpserver filters are correct
4. Send some test traffic to the MCP server

### High Latency Added by Sidecar

The sidecar adds < 1ms overhead. If you see significant latency:

1. Check sidecar CPU limits aren't being throttled
2. Increase CPU limits for high-throughput scenarios:

```yaml
sidecar:
  resources:
    limits:
      cpu: 500m
```

### Sidecar OOMKilled

Increase memory limits:

```yaml
sidecar:
  resources:
    limits:
      memory: 256Mi
```

## See Also

- [Sidecar Architecture](sidecar-architecture.md) - Technical deep-dive
- [Monitoring Guide](monitoring.md) - Operator-level metrics
- [Configuration Guide](configuration-guide.md) - All MCPServer options
