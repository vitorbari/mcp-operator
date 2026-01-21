# Streamable HTTP Transport

Streamable HTTP is the modern transport protocol for MCP, introduced in the MCP 2025-03-26 specification. It's the recommended transport for new server implementations.

## Overview

Streamable HTTP uses standard HTTP requests with optional streaming for responses. It provides better compatibility with standard HTTP infrastructure compared to SSE.

```
┌─────────────────────────────────────────────────────────────┐
│                 Streamable HTTP Transport                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Client                              MCP Server             │
│    │                                     │                  │
│    │ ─────── POST /mcp ────────────────▶ │                  │
│    │ ◀═══════ Streaming Response ═══════ │                  │
│    │                                     │                  │
│    │         (or standard response)      │                  │
│    │ ◀────── JSON-RPC Response ──────── │                  │
│    │                                     │                  │
└─────────────────────────────────────────────────────────────┘
```

## Key Advantages

| Feature | Benefit |
|---------|---------|
| Standard HTTP | Works with any HTTP infrastructure |
| Session management | Built-in session handling |
| Bidirectional | Request/response with streaming support |
| Load balancer | Standard load balancer configurations work |
| Rolling updates | Standard Kubernetes deployments work |

## Configuration

### Basic Configuration

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-streamable
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

### With Session Management

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-streamable-sessions
spec:
  image: "my-registry/mcp-server:latest"

  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 8080
        path: "/mcp"
        sessionManagement: true
```

### Production Configuration

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-streamable-prod
spec:
  image: "my-registry/mcp-server:v1.2.0"

  replicas: 3

  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 8080
        path: "/mcp"
        sessionManagement: true
        security:
          validateOrigin: true
          allowedOrigins:
            - "https://myapp.example.com"

  hpa:
    enabled: true
    minReplicas: 3
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70

  metrics:
    enabled: true
```

## Protocol Detection

When using `protocol: auto`, the operator tries Streamable HTTP first:

1. Sends POST request to `/mcp` (or configured path)
2. Checks for `Mcp-Protocol-Version` header in response
3. Validates JSON-RPC response format
4. Falls back to SSE if Streamable HTTP fails

The detected protocol appears in status:

```bash
kubectl get mcpserver my-server -o jsonpath='{.status.validation.protocol}'
# Output: streamable-http
```

## Building Streamable HTTP Servers

### Python (FastMCP)

```python
from fastmcp import FastMCP

mcp = FastMCP("my-server")

@mcp.tool()
def hello(name: str) -> str:
    """Say hello."""
    return f"Hello, {name}!"

if __name__ == "__main__":
    # Use HTTP transport for Streamable HTTP
    mcp.run(transport="http", host="0.0.0.0", port=8080)
```

### TypeScript

```typescript
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamable-http.js";

const server = new McpServer({
  name: "my-server",
  version: "1.0.0",
});

server.tool("hello", { name: { type: "string" } }, async ({ name }) => {
  return { content: [{ type: "text", text: `Hello, ${name}!` }] };
});

const transport = new StreamableHTTPServerTransport({
  port: 8080,
  path: "/mcp",
});

await server.connect(transport);
```

## Testing

### curl

```bash
# Initialize connection
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-03-26",
      "capabilities": {},
      "clientInfo": {"name": "test", "version": "1.0.0"}
    }
  }'

# List tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}'

# Call a tool
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {"name": "hello", "arguments": {"name": "World"}}
  }'
```

### MCP Inspector

```bash
npm install -g @modelcontextprotocol/inspector
mcp-inspector --http http://localhost:8080/mcp
```

## Comparison with SSE

| Feature | Streamable HTTP | SSE |
|---------|-----------------|-----|
| MCP Spec Version | 2025-03-26+ | 2024-11-05 |
| Connection model | Request/Response | Long-lived |
| Load balancer | Standard | Needs sticky sessions |
| Rolling updates | Standard | Needs maxUnavailable: 0 |
| Session management | Built-in | Requires configuration |
| Infrastructure | Standard HTTP | SSE-specific |

## When to Use Streamable HTTP

**Recommended for:**
- New MCP server implementations
- Kubernetes deployments
- Environments with standard HTTP load balancers
- When session management is needed

**Consider SSE instead if:**
- Working with legacy servers that only support SSE
- Clients don't support Streamable HTTP yet
- Infrastructure is optimized for SSE

## See Also

- [Transport Overview](README.md) - Protocol comparison
- [SSE Transport](sse.md) - Legacy transport
- [Protocol Detection](protocol-detection.md) - Auto-detection behavior
- [Containerizing MCP Servers](../advanced/containerizing.md) - Building container images
