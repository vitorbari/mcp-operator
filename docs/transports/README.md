# Transport Protocols

The Model Context Protocol (MCP) supports multiple transport mechanisms for client-server communication. This section covers the transport options available when deploying MCP servers with the operator.

## Overview

| Transport | Protocol Version | Use Case | Status |
|-----------|-----------------|----------|--------|
| [Streamable HTTP](streamable-http.md) | MCP 2025-03-26+ | Modern, recommended | Active |
| [SSE](sse.md) | MCP 2024-11-05 | Legacy, widely supported | Active |
| stdio | N/A | Local development only | Not supported in Kubernetes |

## Which Protocol Should I Use?

```
┌─────────────────────────────────────────────────────────────┐
│                    Protocol Decision Tree                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Is your MCP server new/modern?                             │
│      YES ──▶ Use Streamable HTTP (protocol: streamable-http)│
│      NO  ──▶ Does it only support SSE?                      │
│                  YES ──▶ Use SSE (protocol: sse)            │
│                  NO  ──▶ Use Auto (protocol: auto)          │
│                                                             │
│  Not sure which protocol your server supports?              │
│      Use auto-detection (protocol: auto)                    │
│      The operator will detect and configure automatically   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Quick Configuration

### Auto-Detection (Recommended for unknown servers)

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: "my-registry/mcp-server:latest"
  transport:
    type: http
    protocol: auto  # Tries Streamable HTTP first, falls back to SSE
    config:
      http:
        port: 8080
```

### Explicit Streamable HTTP (Recommended for new servers)

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: "my-registry/mcp-server:latest"
  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 8080
        path: "/mcp"
```

### Explicit SSE (For legacy servers)

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: "my-registry/mcp-server:latest"
  transport:
    type: http
    protocol: sse
    config:
      http:
        port: 8080
        path: "/sse"
```

## Protocol Comparison

| Feature | Streamable HTTP | SSE |
|---------|-----------------|-----|
| Bidirectional | Yes (via HTTP) | Limited (separate POST endpoint) |
| Connection model | Request/Response + Streaming | Long-lived connection |
| Session management | Built-in | Requires configuration |
| Kubernetes compatibility | Excellent | Requires special handling |
| Load balancer support | Standard | Needs sticky sessions |
| Graceful rollouts | Standard | Needs maxUnavailable: 0 |

## Further Reading

- [Streamable HTTP Transport](streamable-http.md) - Modern transport details
- [SSE Transport](sse.md) - Legacy transport and optimization
- [Protocol Detection](protocol-detection.md) - How auto-detection works
- [Validation Behavior](../advanced/validation-behavior.md) - Protocol validation
