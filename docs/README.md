# MCP Operator Documentation

Welcome to the MCP Operator documentation. This operator manages Model Context Protocol (MCP) servers on Kubernetes.

## Getting Started

| Document | Description |
|----------|-------------|
| [Installation](installation.md) | Install the operator in your cluster |
| [Quick Start](quick-start.md) | Get your first MCPServer running |
| [Configuration](configuration.md) | Basic configuration options |
| [API Reference](api-reference.md) | Complete CRD field reference |

## Transport Protocols

The MCP specification supports multiple transport protocols for client-server communication.

| Document | Description |
|----------|-------------|
| [Transport Overview](transports/README.md) | Understanding MCP transports |
| [Streamable HTTP](transports/streamable-http.md) | Modern transport (MCP 2025-03-26+) |
| [SSE](transports/sse.md) | Server-Sent Events transport (MCP 2024-11-05) |
| [Protocol Detection](transports/protocol-detection.md) | Auto-detection behavior |

## Advanced Topics

| Document | Description |
|----------|-------------|
| [Advanced Configuration](advanced/README.md) | HPA, security, affinity, and more |
| [Validation Behavior](advanced/validation-behavior.md) | Protocol validation details |
| [Containerizing MCP Servers](advanced/containerizing.md) | Build container images for MCP servers |
| [Sidecar Architecture](advanced/sidecar-architecture.md) | Metrics sidecar deep-dive |
| [Kustomize Patterns](advanced/kustomize.md) | Multi-environment deployments |

## Operations

| Document | Description |
|----------|-------------|
| [Operations Overview](operations/README.md) | Running in production |
| [Monitoring](operations/monitoring.md) | Prometheus metrics and Grafana dashboards |
| [MCP Server Metrics](operations/metrics.md) | Sidecar metrics collection |
| [Troubleshooting](operations/troubleshooting.md) | Common issues and solutions |
| [Environment Variables](environment-variables.md) | Environment variable configuration |

## Development

| Document | Description |
|----------|-------------|
| [Release Process](development/release-process.md) | Creating new releases |

## Examples

See the [config/samples/](../config/samples/) directory for complete MCPServer examples, including:

- Basic SSE and Streamable HTTP configurations
- Metrics collection setup
- Complete reference with all available options

## Quick Links

- **GitHub Repository:** [vitorbari/mcp-operator](https://github.com/vitorbari/mcp-operator)
- **Issues:** [Report bugs or request features](https://github.com/vitorbari/mcp-operator/issues)
- **Discussions:** [Ask questions](https://github.com/vitorbari/mcp-operator/discussions)
