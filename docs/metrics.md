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

### Automatic ServiceMonitor Creation

When you enable metrics on an MCPServer, the operator **automatically creates a ServiceMonitor** if Prometheus Operator is installed in your cluster. No manual configuration required!

#### How It Works

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   MCPServer     │────▶│   MCP Operator   │────▶│  ServiceMonitor │
│ metrics: true   │     │  (auto-detect)   │     │  (auto-created) │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                               │
                               ▼
                        ┌──────────────────┐
                        │ Prometheus CRD   │
                        │ check: installed?│
                        └──────────────────┘
```

#### Conditions for Auto-Creation

A ServiceMonitor is automatically created when **both** conditions are met:

1. **Metrics are enabled**: `spec.metrics.enabled: true`
2. **Prometheus Operator is installed**: The ServiceMonitor CRD exists in the cluster

The operator checks for the Prometheus Operator CRD on each reconciliation. If the CRD is not installed, the operator silently skips ServiceMonitor creation and continues normally.

#### Auto-Created ServiceMonitor Labels

The ServiceMonitor is created with these labels:

| Label | Value | Purpose |
|-------|-------|---------|
| `app` | `<mcpserver-name>` | Standard app identifier |
| `app.kubernetes.io/name` | `mcpserver` | Kubernetes recommended label |
| `app.kubernetes.io/instance` | `<mcpserver-name>` | Instance identifier |
| `app.kubernetes.io/component` | `mcp-server` | Component type |
| `app.kubernetes.io/managed-by` | `mcp-operator` | Management identifier |
| `release` | `monitoring` | **Required by kube-prometheus-stack** |

> **Note:** The `release: monitoring` label is included by default to work with kube-prometheus-stack out of the box. If your Prometheus uses different selectors, you may need to create a manual ServiceMonitor instead.

#### Auto-Created ServiceMonitor Selector

The ServiceMonitor uses this selector to find the MCPServer Service:

```yaml
selector:
  matchLabels:
    app: <mcpserver-name>
namespaceSelector:
  matchNames:
    - <mcpserver-namespace>
```

#### Endpoint Configuration

The auto-created ServiceMonitor configures scraping with:

- **Port**: `metrics` (port name, not number)
- **Path**: `/metrics`
- **Interval**: `30s`
- **TLS**: Automatically configured if `spec.sidecar.tls.enabled: true`

#### Lifecycle Management

| Action | Result |
|--------|--------|
| Create MCPServer with `metrics.enabled: true` | ServiceMonitor created automatically |
| Update MCPServer (change metrics config) | ServiceMonitor updated |
| Set `metrics.enabled: false` | **ServiceMonitor deleted automatically** |
| Delete MCPServer | ServiceMonitor deleted (owner reference) |
| Accidentally delete ServiceMonitor | ServiceMonitor recreated on next reconciliation |

#### Example: Verifying Auto-Creation

```bash
# Create MCPServer with metrics enabled
kubectl apply -f - <<EOF
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: your-registry/mcp-server:latest
  metrics:
    enabled: true
EOF

# Verify ServiceMonitor was created
kubectl get servicemonitor my-mcp-server -o yaml

# Check that Prometheus discovered the target
kubectl port-forward svc/prometheus-operated 9090:9090
# Visit http://localhost:9090/targets and look for my-mcp-server
```

### Troubleshooting ServiceMonitor Auto-Creation

#### ServiceMonitor Not Created

**Symptom:** `kubectl get servicemonitor <name>` returns "not found"

**Possible causes:**

1. **Prometheus Operator not installed**
   ```bash
   # Check if ServiceMonitor CRD exists
   kubectl get crd servicemonitors.monitoring.coreos.com
   ```
   If missing, install Prometheus Operator first.

2. **Metrics not enabled**
   ```bash
   # Check MCPServer spec
   kubectl get mcpserver <name> -o jsonpath='{.spec.metrics.enabled}'
   # Should return "true"
   ```

3. **Operator doesn't have RBAC permissions**
   ```bash
   # Check operator logs for permission errors
   kubectl logs -n mcp-operator-system deploy/mcp-operator-controller-manager | grep -i servicemonitor
   ```

#### ServiceMonitor Created But Prometheus Not Scraping

**Symptom:** ServiceMonitor exists but no targets appear in Prometheus

1. **Check ServiceMonitor labels match Prometheus selector**
   ```bash
   # View ServiceMonitor labels
   kubectl get servicemonitor <name> -o jsonpath='{.metadata.labels}'

   # Check Prometheus serviceMonitorSelector
   kubectl get prometheus -o jsonpath='{.items[0].spec.serviceMonitorSelector}'
   ```

   The auto-created ServiceMonitor uses `release: monitoring`. If your Prometheus uses a different selector, create a manual ServiceMonitor.

2. **Verify Service has metrics port**
   ```bash
   kubectl get svc <name> -o jsonpath='{.spec.ports[?(@.name=="metrics")]}'
   ```

3. **Check namespace access**
   ```bash
   # Prometheus must have access to the MCPServer namespace
   kubectl get prometheus -o jsonpath='{.items[0].spec.serviceMonitorNamespaceSelector}'
   ```

### Manual ServiceMonitor (Custom Configuration)

If the auto-created ServiceMonitor doesn't match your Prometheus configuration, you can disable auto-creation and create your own:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: mcp-server-metrics
  labels:
    release: prometheus  # Match YOUR Prometheus selector
spec:
  selector:
    matchLabels:
      app: my-mcp-server
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

> **Note:** When creating a manual ServiceMonitor, you may want to set `metrics.enabled: true` but NOT rely on the auto-created one. The operator will create a ServiceMonitor regardless - if you want full control, ensure your manual one has different labels so both can coexist, or don't install Prometheus Operator CRDs until after your manual setup.

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
