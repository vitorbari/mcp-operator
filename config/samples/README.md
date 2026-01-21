# MCPServer Samples

Example configurations for deploying MCP servers with the operator.

## Quick Reference

| # | Sample | Transport | Metrics | Use Case |
|---|--------|-----------|---------|----------|
| 01 | [wikipedia-sse](01-wikipedia-sse.yaml) | SSE | No | Minimal example using Wikipedia MCP |
| 02 | [streamable-http-basic](02-streamable-http-basic.yaml) | Streamable HTTP | No | Modern transport, production-ready |
| 03 | [sse-optimized](03-sse-optimized.yaml) | SSE | No | SSE with session affinity |
| 04 | [metrics-basic](04-metrics-basic.yaml) | Streamable HTTP | Yes | Basic metrics collection |
| 05 | [metrics-advanced](05-metrics-advanced.yaml) | Streamable HTTP | Yes | Custom sidecar configuration |
| 06 | [metrics-sse](06-metrics-sse.yaml) | SSE | Yes | SSE with metrics |
| 10 | [complete-reference](10-complete-reference.yaml) | Auto | Yes | All available options |

## Decision Tree

```
Which sample should I use?
│
├── Just getting started?
│   └── 01-wikipedia-sse.yaml (minimal, works out of the box)
│
├── Building a new server?
│   └── 02-streamable-http-basic.yaml (modern transport)
│
├── Using legacy SSE server?
│   ├── Simple deployment → 01-wikipedia-sse.yaml
│   └── Need optimizations → 03-sse-optimized.yaml
│
├── Need metrics?
│   ├── Streamable HTTP → 04-metrics-basic.yaml
│   ├── Custom config → 05-metrics-advanced.yaml
│   └── SSE + metrics → 06-metrics-sse.yaml
│
└── Reference for all options?
    └── 10-complete-reference.yaml
```

## Sample Descriptions

### 01-wikipedia-sse.yaml

Minimal example using the public Wikipedia MCP server image. Great for:
- Quick testing
- Learning the basics
- Verifying operator installation

```bash
kubectl apply -f 01-wikipedia-sse.yaml
kubectl port-forward svc/wikipedia-mcp 3001:3001
# Test: curl -N http://localhost:3001/sse
```

### 02-streamable-http-basic.yaml

Production-ready configuration using modern Streamable HTTP transport:
- Explicit protocol configuration
- HPA enabled
- Metrics collection
- Health checks

### 03-sse-optimized.yaml

SSE configuration with optimizations for production:
- Session affinity for sticky connections
- Termination grace period for graceful shutdowns
- Suitable for stateful SSE connections

### 04-metrics-basic.yaml

Basic metrics collection with mcp-proxy sidecar:
- Prometheus metrics on port 9090
- Auto-creates ServiceMonitor if Prometheus Operator is installed
- Minimal configuration

### 05-metrics-advanced.yaml

Advanced metrics with custom sidecar configuration:
- Custom resource limits for sidecar
- TLS termination example
- Extended configuration options

### 06-metrics-sse.yaml

SSE transport with metrics collection:
- Combines SSE optimizations with metrics
- Session affinity + metrics sidecar
- Production SSE deployment

### 10-complete-reference.yaml

Reference example showing all available CRD fields:
- Every configuration option documented
- Not meant for direct use
- Copy sections as needed

## Usage

### Apply a sample

```bash
kubectl apply -f 01-wikipedia-sse.yaml
```

### Watch status

```bash
kubectl get mcpserver -w
```

### Test connection

```bash
# Port forward
kubectl port-forward svc/<mcpserver-name> 8080:8080

# Test (SSE)
curl -N http://localhost:8080/sse

# Test (Streamable HTTP)
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```

## Advanced Examples

For production patterns like NetworkPolicies and PodDisruptionBudgets, see:
- [docs/advanced/networkpolicy-example.yaml](../../docs/advanced/networkpolicy-example.yaml)
- [docs/advanced/poddisruptionbudget-example.yaml](../../docs/advanced/poddisruptionbudget-example.yaml)

## Documentation

- [Configuration Guide](../../docs/configuration.md) - Basic configuration
- [Advanced Configuration](../../docs/advanced/configuration-advanced.md) - HPA, security, affinity
- [Transport Protocols](../../docs/transports/README.md) - SSE vs Streamable HTTP
- [API Reference](../../docs/api-reference.md) - Complete field documentation
