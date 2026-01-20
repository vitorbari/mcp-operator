# Sidecar Architecture

Technical deep-dive into the mcp-proxy sidecar for MCP server metrics collection and TLS termination.

## Overview

The mcp-proxy sidecar is a lightweight reverse proxy that sits in front of your MCP server, providing:

- **Prometheus Metrics** - Request counts, latencies, sizes, and MCP-specific metrics
- **Protocol Parsing** - Understands JSON-RPC and MCP methods for detailed observability
- **TLS Termination** - Optional HTTPS support without modifying your server
- **Health Endpoints** - `/healthz` and `/readyz` for Kubernetes probes

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ MCPServer Pod                                                               │
│                                                                             │
│  ┌───────────────────────────────┐    ┌───────────────────────────────┐    │
│  │        mcp-proxy              │    │        MCP Server             │    │
│  │        (sidecar)              │    │        (your image)           │    │
│  │                               │    │                               │    │
│  │  ┌─────────────────────────┐  │    │                               │    │
│  │  │   Reverse Proxy         │──┼───▶│  Port: 3001                   │    │
│  │  │   Port: 8080            │  │    │                               │    │
│  │  └─────────────────────────┘  │    │  - Streamable HTTP            │    │
│  │                               │    │  - SSE                        │    │
│  │  ┌─────────────────────────┐  │    │                               │    │
│  │  │   Metrics Server        │  │    └───────────────────────────────┘    │
│  │  │   Port: 9090            │  │                                         │
│  │  │   - /metrics            │  │                                         │
│  │  │   - /healthz            │  │                                         │
│  │  │   - /readyz             │  │                                         │
│  │  └─────────────────────────┘  │                                         │
│  │                               │                                         │
│  │  ┌─────────────────────────┐  │                                         │
│  │  │   JSON-RPC Parser       │  │                                         │
│  │  │   - Method extraction   │  │                                         │
│  │  │   - Tool call tracking  │  │                                         │
│  │  │   - Error classification│  │                                         │
│  │  └─────────────────────────┘  │                                         │
│  └───────────────────────────────┘                                         │
│                 ▲                                                           │
└─────────────────│───────────────────────────────────────────────────────────┘
                  │
          K8s Service
          ├── Port 8080 (http)  ─────▶ Clients (MCP traffic)
          └── Port 9090 (metrics) ───▶ Prometheus
```

## Layered API Design

The operator uses a layered API that provides simple defaults with advanced customization options.

### Layer 1: Simple Toggle

Enable metrics with a single field:

```yaml
spec:
  metrics:
    enabled: true
```

This uses all defaults:
- Sidecar image: `ghcr.io/vitorbari/mcp-proxy:latest`
- Metrics port: 9090
- Default resource requests/limits

### Layer 2: Metrics Customization

Customize the metrics configuration:

```yaml
spec:
  metrics:
    enabled: true
    port: 9091  # Custom metrics port
```

### Layer 3: Full Sidecar Control

Advanced users can customize everything:

```yaml
spec:
  metrics:
    enabled: true
    port: 9091
  sidecar:
    image: ghcr.io/vitorbari/mcp-proxy:v0.2.0  # Pin version
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 256Mi
    tls:
      enabled: true
      secretName: mcp-tls-secret
      minVersion: "1.3"
```

## Traffic Flow

### HTTP Traffic (Streamable HTTP)

```
Client Request
    │
    ▼
┌─────────────────┐
│  K8s Service    │
│  Port: 8080     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  mcp-proxy      │  ◄── Records: method, status, duration, size
│  :8080          │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  MCP Server     │
│  :3001          │
└─────────────────┘
```

### SSE Traffic

For SSE connections, the sidecar:
1. Proxies the initial connection
2. Streams events back to the client
3. Tracks connection duration
4. Records event types

```
Client
    │
    ▼ SSE Connection (/sse)
┌─────────────────┐
│  mcp-proxy      │  ◄── Records: connection open/close, events
│                 │
└────────┬────────┘
         │ Proxy
         ▼
┌─────────────────┐
│  MCP Server     │  ◄── Sends: message, endpoint events
│                 │
└─────────────────┘
```

## Configuration Options

### MCPServer Spec

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `metrics.enabled` | bool | `false` | Enable metrics sidecar injection |
| `metrics.port` | int32 | `9090` | Port for metrics endpoint |
| `sidecar.image` | string | `ghcr.io/vitorbari/mcp-proxy:latest` | Sidecar container image |
| `sidecar.resources` | ResourceRequirements | See below | Container resource limits |
| `sidecar.tls.enabled` | bool | `false` | Enable TLS termination |
| `sidecar.tls.secretName` | string | - | Name of TLS secret |
| `sidecar.tls.minVersion` | string | `"1.2"` | Minimum TLS version |

### Default Resources

```yaml
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 200m
    memory: 128Mi
```

### Sidecar Command-Line Args

The operator configures the sidecar with these arguments:

| Argument | Value | Description |
|----------|-------|-------------|
| `--target-addr` | `localhost:<port>` | Backend MCP server address |
| `--listen-addr` | `:8080` | Proxy listen address |
| `--metrics-addr` | `:<metricsPort>` | Metrics server address |
| `--log-level` | `info` | Logging verbosity |
| `--tls-enabled` | (if configured) | Enable TLS |
| `--tls-cert-file` | `/etc/tls/tls.crt` | Certificate path |
| `--tls-key-file` | `/etc/tls/tls.key` | Key path |
| `--tls-min-version` | `1.2` or `1.3` | Minimum TLS version |

## TLS Setup Guide

### 1. Create a TLS Secret

Using cert-manager:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: mcp-tls
spec:
  secretName: mcp-tls-secret
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - mcp.example.com
```

Or manually:

```bash
kubectl create secret tls mcp-tls-secret \
  --cert=tls.crt \
  --key=tls.key
```

### 2. Configure the MCPServer

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: secure-mcp-server
spec:
  image: your-registry/your-mcp-server:latest
  transport:
    type: http
    config:
      http:
        port: 3001
  metrics:
    enabled: true
  sidecar:
    tls:
      enabled: true
      secretName: mcp-tls-secret
      minVersion: "1.2"  # or "1.3" for stricter security
```

### 3. Access via HTTPS

```bash
curl https://mcp.example.com:8080/mcp
```

## Troubleshooting

### Sidecar Not Injected

**Symptom:** Pod has only one container

**Check:**
1. Verify `metrics.enabled: true` is set
2. Check operator logs for errors
3. Ensure the transport type is `http`

```bash
kubectl get mcpserver my-server -o jsonpath='{.spec.metrics.enabled}'
kubectl logs -n mcp-operator-system deployment/mcp-operator-controller-manager
```

### Metrics Endpoint Not Responding

**Symptom:** Can't scrape metrics from port 9090

**Check:**
1. Verify sidecar container is running
2. Check sidecar logs for startup errors
3. Test metrics endpoint directly

```bash
kubectl get pods -l app=my-server -o jsonpath='{.items[0].status.containerStatuses[*].ready}'
kubectl logs -l app=my-server -c mcp-proxy
kubectl port-forward svc/my-server 9090:9090
curl http://localhost:9090/metrics
```

### TLS Certificate Errors

**Symptom:** Connection refused or certificate errors

**Check:**
1. Verify secret exists and has correct keys
2. Check secret is in the same namespace
3. Verify certificate is valid

```bash
kubectl get secret mcp-tls-secret -o yaml
openssl x509 -in <(kubectl get secret mcp-tls-secret -o jsonpath='{.data.tls\.crt}' | base64 -d) -text -noout
```

### High Memory Usage

**Symptom:** Sidecar OOMKilled

**Fix:** Increase memory limits

```yaml
sidecar:
  resources:
    limits:
      memory: 256Mi
```

### Slow Requests

**Symptom:** Added latency compared to direct access

The sidecar adds minimal overhead (~1ms). If you see significant latency:

1. Check sidecar CPU limits aren't being throttled
2. Review if JSON-RPC parsing is struggling with large messages
3. Consider increasing CPU limits for high-throughput scenarios

```yaml
sidecar:
  resources:
    limits:
      cpu: 500m
```

## Performance Considerations

### Resource Sizing

| Scenario | CPU Request | Memory Request | CPU Limit | Memory Limit |
|----------|-------------|----------------|-----------|--------------|
| Low traffic (<10 req/s) | 50m | 64Mi | 200m | 128Mi |
| Medium traffic (10-100 req/s) | 100m | 128Mi | 500m | 256Mi |
| High traffic (>100 req/s) | 200m | 256Mi | 1000m | 512Mi |

### Overhead

The sidecar adds approximately:
- **Latency:** < 1ms per request
- **Memory:** ~20-50MB baseline
- **CPU:** Negligible for most workloads

### Connection Handling

- Uses Go's efficient HTTP proxy implementation
- Supports HTTP/1.1 and HTTP/2
- Connection pooling to backend server
- Graceful shutdown on pod termination

## Security Considerations

### Pod Security

The sidecar container runs with:
- Non-root user (UID 65532)
- Read-only root filesystem
- No privilege escalation
- All capabilities dropped
- RuntimeDefault seccomp profile

These settings are automatically applied and comply with Kubernetes restricted pod security standards.

### Network Security

- The sidecar only listens on localhost for backend communication
- External traffic comes through the Kubernetes Service
- TLS can be enabled for encrypted client connections
- No sensitive data is logged (only method names, not payloads)

## Next Steps

- [MCP Server Metrics](metrics.md) - Available metrics and example queries
- [Monitoring Guide](monitoring.md) - Operator metrics and Grafana dashboards
- [Configuration Guide](configuration-guide.md) - All MCPServer options
