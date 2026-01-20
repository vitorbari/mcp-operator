# MCP Proxy Sidecar

A lightweight metrics sidecar proxy for MCP (Model Context Protocol) servers. The proxy intercepts MCP traffic, parses JSON-RPC messages, and exposes Prometheus metrics.

## Features

- **Prometheus Metrics** - Request counts, latencies, sizes, and MCP-specific metrics
- **Protocol Parsing** - Understands JSON-RPC methods like `tools/call`, `resources/read`
- **SSE Support** - Handles Server-Sent Events for legacy MCP transports
- **TLS Termination** - Optional HTTPS support
- **Health Endpoints** - Kubernetes-compatible liveness and readiness probes
- **Minimal Footprint** - ~20MB memory, sub-millisecond latency overhead

## Quick Start

### Running Locally

```bash
# Build the binary
make build

# Run with an MCP server on localhost:3001
./bin/mcp-proxy --target-addr=localhost:3001
```

### With Docker

```bash
# Build the image
make docker-build IMG=mcp-proxy:dev

# Run with Docker
docker run -p 8080:8080 -p 9090:9090 mcp-proxy:dev \
  --target-addr=host.docker.internal:3001
```

## Development

### Prerequisites

- Go 1.22+
- Docker (for container builds)
- golangci-lint (for linting)

### Project Structure

```
sidecar/
├── cmd/
│   └── mcp-proxy/      # Main entrypoint
├── pkg/
│   ├── config/         # Configuration handling
│   ├── metrics/        # Prometheus metrics
│   └── proxy/          # HTTP proxy and SSE handler
├── deploy/             # Test Kubernetes manifests
├── Dockerfile          # Container build
├── Makefile            # Build commands
└── README.md           # This file
```

### Building

```bash
# Build binary
make build

# Build with race detector (for testing)
go build -race -o bin/mcp-proxy ./cmd/mcp-proxy

# Build Docker image
make docker-build IMG=ghcr.io/vitorbari/mcp-proxy:dev

# Build multi-platform image
make docker-buildx IMG=ghcr.io/vitorbari/mcp-proxy:dev
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific test
go test -v ./pkg/metrics/... -run TestRecorder
```

### Linting

```bash
# Run linter
make lint

# Auto-fix issues
make lint-fix
```

### Local Development

Start an MCP server (e.g., the everything-server):

```bash
docker run -p 3001:3001 tzolov/mcp-everything-server:v3 \
  node dist/index.js streamableHttp
```

Run the proxy pointing to it:

```bash
make run
# Or with custom flags:
go run ./cmd/mcp-proxy \
  --target-addr=localhost:3001 \
  --listen-addr=:8080 \
  --metrics-addr=:9090 \
  --log-level=debug
```

Test with curl:

```bash
# MCP initialize request
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}'

# Check metrics
curl http://localhost:9090/metrics | grep mcp_
```

## Configuration

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen-addr` | `:8080` | Address for incoming MCP requests |
| `--target-addr` | `localhost:3001` | Backend MCP server address |
| `--metrics-addr` | `:9090` | Address for metrics endpoint |
| `--log-level` | `info` | Log level (debug, info, warn, error) |
| `--health-check-interval` | `10s` | Interval for backend health checks |
| `--tls-enabled` | `false` | Enable TLS termination |
| `--tls-cert-file` | - | Path to TLS certificate |
| `--tls-key-file` | - | Path to TLS private key |
| `--tls-min-version` | `1.2` | Minimum TLS version (1.2 or 1.3) |

### Example with TLS

```bash
./bin/mcp-proxy \
  --target-addr=localhost:3001 \
  --tls-enabled \
  --tls-cert-file=/path/to/cert.pem \
  --tls-key-file=/path/to/key.pem \
  --tls-min-version=1.3
```

## Metrics

The proxy exposes these metrics at `/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `mcp_requests_total` | Counter | Total requests by status and method |
| `mcp_request_duration_seconds` | Histogram | Request latency |
| `mcp_request_size_bytes` | Histogram | Request body size |
| `mcp_response_size_bytes` | Histogram | Response body size |
| `mcp_active_connections` | Gauge | Current active connections |
| `mcp_tool_calls_total` | Counter | Tool calls by tool name |
| `mcp_resource_reads_total` | Counter | Resource reads by URI |
| `mcp_request_errors_total` | Counter | JSON-RPC errors by method and code |
| `mcp_sse_connections_total` | Counter | Total SSE connections |
| `mcp_sse_connections_active` | Gauge | Active SSE connections |
| `mcp_sse_events_total` | Counter | SSE events by type |
| `mcp_sse_connection_duration_seconds` | Histogram | SSE connection duration |
| `mcp_proxy_info` | Gauge | Static proxy info (version, target) |

## Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `/healthz` | Liveness probe - returns 200 if proxy is running |
| `/readyz` | Readiness probe - returns 200 if backend is reachable |

## Testing with Kubernetes

See [deploy/README.md](deploy/README.md) for instructions on testing in a Kubernetes cluster.

## Release Process

Releases are automated via GitHub Actions when the operator is released. The sidecar version is kept in sync with the operator version.

### Manual Release (if needed)

```bash
# Tag and push
VERSION=v0.1.0
make docker-buildx IMG=ghcr.io/vitorbari/mcp-proxy:${VERSION}
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      mcp-proxy                              │
│                                                             │
│  ┌─────────────────┐   ┌─────────────────┐                 │
│  │  HTTP Server    │   │  Metrics Server │                 │
│  │  :8080          │   │  :9090          │                 │
│  │                 │   │  /metrics       │                 │
│  │  ┌───────────┐  │   │  /healthz       │                 │
│  │  │  Proxy    │  │   │  /readyz        │                 │
│  │  │  Handler  │──┼───▶  ┌───────────┐  │                 │
│  │  └───────────┘  │   │  │ Recorder  │  │                 │
│  │        │        │   │  └───────────┘  │                 │
│  │        ▼        │   └─────────────────┘                 │
│  │  ┌───────────┐  │                                       │
│  │  │ JSON-RPC  │  │                                       │
│  │  │  Parser   │  │                                       │
│  │  └───────────┘  │                                       │
│  └────────┬────────┘                                       │
│           │                                                │
└───────────│────────────────────────────────────────────────┘
            │
            ▼
     Backend MCP Server
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint`
6. Submit a pull request

## License

Apache License 2.0 - see the main repository LICENSE file.
