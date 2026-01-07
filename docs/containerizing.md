# Containerizing MCP Servers for Kubernetes

This guide covers how to package MCP (Model Context Protocol) servers as container images for deployment on Kubernetes with the MCP Operator.

## Overview

Most MCP servers are designed for local use with Claude Desktop via stdio transport. Running them on Kubernetes requires either:

1. **Native HTTP/SSE transport** - Server directly exposes HTTP endpoints (recommended)
2. **stdio with adapter** - Wrap stdio servers with a proxy that exposes HTTP

```
┌─────────────────────────────────────────────────────────────┐
│                    Transport Options                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Option A: Native HTTP/SSE          Option B: stdio + Proxy │
│  ┌──────────────────────┐          ┌──────────────────────┐ │
│  │   MCP Server         │          │   Pod                │ │
│  │   (HTTP/SSE)         │          │  ┌────────┐ ┌─────┐  │ │
│  │        ↑             │          │  │ Server │↔│Proxy│  │ │
│  │     Port 8000        │          │  │ stdio  │ │ HTTP│  │ │
│  └──────────────────────┘          │  └────────┘ └──┬──┘  │ │
│           ↑                        │                ↑     │ │
│      Direct HTTP                   │           Port 8000  │ │
│                                    └──────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Transport Types Explained

| Transport | Protocol | Use Case | Kubernetes Ready |
|-----------|----------|----------|------------------|
| **Streamable HTTP** | HTTP POST with streaming | Modern, recommended | ✅ Yes |
| **SSE** | Server-Sent Events | Legacy, widely supported | ✅ Yes |
| **stdio** | stdin/stdout | Local development | ⚠️ Needs adapter |

**Recommendation:** Use Streamable HTTP for new servers. It's the direction the MCP spec is heading and works naturally with Kubernetes Services and Ingress.

---

## Python: FastMCP

[FastMCP](https://github.com/jlowin/fastmcp) is the most popular Python framework for building MCP servers.

### Basic Server Example

```python
# server.py
from fastmcp import FastMCP

mcp = FastMCP("my-tools")

@mcp.tool()
def greet(name: str) -> str:
    """Greet someone by name."""
    return f"Hello, {name}!"

@mcp.tool()
def add(a: int, b: int) -> int:
    """Add two numbers."""
    return a + b

if __name__ == "__main__":
    mcp.run()
```

### Dockerfile for FastMCP (HTTP Transport)

```dockerfile
# Dockerfile
FROM python:3.12-slim

# Set environment variables
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install -r requirements.txt

# Copy application code
COPY . .

# Expose the MCP server port
EXPOSE 8000

# Run with HTTP transport (Streamable HTTP)
CMD ["python", "server.py", "--transport", "http", "--host", "0.0.0.0", "--port", "8000"]
```

```text
# requirements.txt
fastmcp>=0.1.0
```

### Running with Different Transports

FastMCP supports multiple transports via command line or environment variables:

```bash
# Streamable HTTP (recommended for Kubernetes)
python server.py --transport http --host 0.0.0.0 --port 8000

# SSE transport
python server.py --transport sse --host 0.0.0.0 --port 8000

# stdio (for local development)
python server.py --transport stdio
```

### Environment Variable Configuration

```dockerfile
# Alternative: Configure via environment variables
ENV FASTMCP_TRANSPORT=http
ENV FASTMCP_HOST=0.0.0.0
ENV FASTMCP_PORT=8000

CMD ["python", "server.py"]
```

### Multi-stage Build (Production)

```dockerfile
# Dockerfile.production
FROM python:3.12-slim AS builder

WORKDIR /app
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

FROM python:3.12-slim

# Create non-root user
RUN useradd --create-home --shell /bin/bash mcp
USER mcp

WORKDIR /app

# Copy virtual environment from builder
COPY --from=builder /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

COPY --chown=mcp:mcp . .

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')" || exit 1

CMD ["python", "server.py", "--transport", "http", "--host", "0.0.0.0", "--port", "8000"]
```

---

## TypeScript: Official MCP SDK

The [official MCP TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk) is commonly used for Node.js servers.

### Basic Server Example

```typescript
// src/index.ts
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { SSEServerTransport } from "@modelcontextprotocol/sdk/server/sse.js";
import express from "express";

const server = new McpServer({
  name: "my-tools",
  version: "1.0.0",
});

// Register a tool
server.tool("greet", { name: { type: "string" } }, async ({ name }) => {
  return { content: [{ type: "text", text: `Hello, ${name}!` }] };
});

// Transport selection based on environment
const transport = process.env.MCP_TRANSPORT || "stdio";

if (transport === "sse") {
  const app = express();
  const port = parseInt(process.env.PORT || "8000");
  
  app.get("/sse", async (req, res) => {
    const transport = new SSEServerTransport("/messages", res);
    await server.connect(transport);
  });
  
  app.post("/messages", express.json(), async (req, res) => {
    // Handle incoming messages
  });
  
  app.get("/health", (req, res) => res.send("OK"));
  
  app.listen(port, "0.0.0.0", () => {
    console.log(`MCP server running on port ${port}`);
  });
} else {
  // Default to stdio
  const transport = new StdioServerTransport();
  await server.connect(transport);
}
```

### Dockerfile for TypeScript SDK

```dockerfile
# Dockerfile
FROM node:20-slim AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM node:20-slim

# Create non-root user
RUN useradd --create-home --shell /bin/bash mcp
USER mcp

WORKDIR /app

COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./

ENV MCP_TRANSPORT=sse
ENV PORT=8000

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD node -e "fetch('http://localhost:8000/health').then(r => process.exit(r.ok ? 0 : 1))" || exit 1

CMD ["node", "dist/index.js"]
```

```json
// package.json
{
  "name": "my-mcp-server",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "build": "tsc",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "@modelcontextprotocol/sdk": "^1.0.0",
    "express": "^4.18.2"
  },
  "devDependencies": {
    "@types/express": "^4.17.21",
    "@types/node": "^20.10.0",
    "typescript": "^5.3.0"
  }
}
```

---

## stdio Servers: Using a Proxy Adapter

Many existing MCP servers only support stdio transport. To run these on Kubernetes, use a proxy that converts stdio to HTTP.

### Option 1: Supergateway

[Supergateway](https://github.com/supercorp-ai/supergateway) wraps stdio MCP servers and exposes them via SSE.

```dockerfile
# Dockerfile using supergateway
FROM node:20-slim

RUN npm install -g supergateway

# Install your stdio MCP server
COPY --from=your-mcp-server /app /mcp-server

ENV PORT=8000

EXPOSE 8000

# Supergateway wraps the stdio server
CMD ["supergateway", "--stdio", "node /mcp-server/index.js", "--port", "8000"]
```

### Option 2: mcp-proxy

[mcp-proxy](https://github.com/sparfenyuk/mcp-proxy) is a Python-based proxy supporting multiple transports.

```dockerfile
# Dockerfile using mcp-proxy
FROM python:3.12-slim

RUN pip install mcp-proxy

# Copy your stdio server
COPY ./my-stdio-server /app/server

WORKDIR /app

EXPOSE 8000

# Proxy stdio server to SSE
CMD ["mcp-proxy", "--listen", "0.0.0.0:8000", "--", "python", "server/main.py"]
```

---

## Testing Your Containerized Server

### 1. Build and Run Locally

```bash
# Build the image
docker build -t my-mcp-server:test .

# Run with port mapping
docker run -p 8000:8000 my-mcp-server:test
```

### 2. Test with MCP Inspector

The [MCP Inspector](https://github.com/modelcontextprotocol/inspector) is an official tool for testing MCP servers.

```bash
# Install MCP Inspector
npm install -g @modelcontextprotocol/inspector

# Test SSE endpoint
mcp-inspector --sse http://localhost:8000/sse

# Test Streamable HTTP endpoint
mcp-inspector --http http://localhost:8000
```

### 3. Test with curl

```bash
# Health check
curl http://localhost:8000/health

# SSE connection test
curl -N http://localhost:8000/sse

# Streamable HTTP - Initialize
curl -X POST http://localhost:8000 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}'

# List tools
curl -X POST http://localhost:8000 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

### 4. Test with mcphost

[mcphost](https://github.com/punkpeye/mcphost) is a CLI tool for interacting with MCP servers.

```bash
# Install
go install github.com/punkpeye/mcphost@latest

# Connect to your server
mcphost connect http://localhost:8000

# List available tools
mcphost tools

# Call a tool
mcphost call greet --name "World"
```

---

## Best Practices

### Health Checks

Always implement a health endpoint for Kubernetes probes:

```python
# FastMCP with health endpoint
from fastmcp import FastMCP
from starlette.responses import PlainTextResponse

mcp = FastMCP("my-server")

@mcp.custom_route("/health", methods=["GET"])
async def health():
    return PlainTextResponse("OK")
```

```yaml
# MCPServer with probes
apiVersion: mcp.mcp-operator.io/v1alpha1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: my-mcp-server:latest
  transport:
    type: http
    port: 8000
  probes:
    liveness:
      httpGet:
        path: /health
        port: 8000
      initialDelaySeconds: 5
      periodSeconds: 10
    readiness:
      httpGet:
        path: /health
        port: 8000
      initialDelaySeconds: 5
      periodSeconds: 5
```

### Graceful Shutdown

Handle SIGTERM for clean shutdown:

```python
# Python
import signal
import sys

def handle_shutdown(signum, frame):
    print("Shutting down gracefully...")
    # Cleanup code here
    sys.exit(0)

signal.signal(signal.SIGTERM, handle_shutdown)
```

```typescript
// TypeScript
process.on('SIGTERM', () => {
  console.log('Shutting down gracefully...');
  // Cleanup code here
  process.exit(0);
});
```

### Resource Limits

Set appropriate resource limits in your MCPServer manifest:

```yaml
apiVersion: mcp.mcp-operator.io/v1alpha1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: my-mcp-server:latest
  resources:
    requests:
      memory: "64Mi"
      cpu: "100m"
    limits:
      memory: "256Mi"
      cpu: "500m"
```

### Security

1. **Run as non-root:**
   ```dockerfile
   RUN useradd --create-home mcp
   USER mcp
   ```

2. **Use read-only filesystem where possible:**
   ```yaml
   securityContext:
     readOnlyRootFilesystem: true
   ```

3. **Don't hardcode secrets:**
   ```python
   import os
   api_key = os.environ.get("API_KEY")
   ```

### Logging

Use structured JSON logging for better observability:

```python
import logging
import json

class JSONFormatter(logging.Formatter):
    def format(self, record):
        return json.dumps({
            "timestamp": self.formatTime(record),
            "level": record.levelname,
            "message": record.getMessage(),
            "logger": record.name,
        })

handler = logging.StreamHandler()
handler.setFormatter(JSONFormatter())
logging.getLogger().addHandler(handler)
```

---

## Useful Tools

| Tool | Purpose | Link |
|------|---------|------|
| **MCP Inspector** | Official testing/debugging tool | [GitHub](https://github.com/modelcontextprotocol/inspector) |
| **Supergateway** | stdio → SSE proxy | [GitHub](https://github.com/supercorp-ai/supergateway) |
| **mcp-proxy** | Multi-transport proxy | [GitHub](https://github.com/sparfenyuk/mcp-proxy) |
| **mcphost** | CLI for MCP servers | [GitHub](https://github.com/punkpeye/mcphost) |
| **FastMCP** | Python MCP framework | [GitHub](https://github.com/jlowin/fastmcp) |
| **mcp-framework** | TypeScript framework | [GitHub](https://github.com/QuantGeekDev/mcp-framework) |
| **Dive** | Docker image analyzer | [GitHub](https://github.com/wagoodman/dive) |
| **Trivy** | Container security scanner | [GitHub](https://github.com/aquasecurity/trivy) |

---

## Troubleshooting

### Server starts but no response

1. Check the server is binding to `0.0.0.0`, not `127.0.0.1`
2. Verify the correct port is exposed in Dockerfile
3. Check logs: `docker logs <container-id>`

### Connection refused

```bash
# Check if server is listening
docker exec <container-id> netstat -tlnp

# Check if port is mapped correctly
docker port <container-id>
```

### stdio server not working with proxy

1. Ensure the stdio server writes to stdout, not stderr for responses
2. Check the server doesn't buffer stdout: `PYTHONUNBUFFERED=1`
3. Verify the proxy command is correct

### Protocol errors

1. Test with MCP Inspector to see exact request/response
2. Verify JSON-RPC format is correct
3. Check protocol version compatibility

---

## Example: Complete FastMCP Project

```
my-mcp-server/
├── Dockerfile
├── requirements.txt
├── server.py
├── tools/
│   ├── __init__.py
│   └── math_tools.py
└── tests/
    └── test_server.py
```

```python
# server.py
from fastmcp import FastMCP
from tools.math_tools import register_math_tools

mcp = FastMCP(
    name="my-tools",
    version="1.0.0",
    description="A collection of useful tools"
)

register_math_tools(mcp)

@mcp.custom_route("/health", methods=["GET"])
async def health():
    from starlette.responses import PlainTextResponse
    return PlainTextResponse("OK")

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("--transport", default="http", choices=["http", "sse", "stdio"])
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8000)
    args = parser.parse_args()
    
    mcp.run(transport=args.transport, host=args.host, port=args.port)
```

```python
# tools/math_tools.py
def register_math_tools(mcp):
    @mcp.tool()
    def add(a: float, b: float) -> float:
        """Add two numbers."""
        return a + b
    
    @mcp.tool()
    def multiply(a: float, b: float) -> float:
        """Multiply two numbers."""
        return a * b
```

```dockerfile
# Dockerfile
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

RUN useradd --create-home mcp
USER mcp

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')" || exit 1

CMD ["python", "server.py", "--transport", "http", "--host", "0.0.0.0", "--port", "8000"]
```

```text
# requirements.txt
fastmcp>=2.0.0
```

Build and deploy:

```bash
# Build
docker build -t my-mcp-server:latest .

# Test locally
docker run -p 8000:8000 my-mcp-server:latest

# Push to registry
docker tag my-mcp-server:latest ghcr.io/myorg/my-mcp-server:latest
docker push ghcr.io/myorg/my-mcp-server:latest
```

Deploy with MCP Operator:

```yaml
apiVersion: mcp.mcp-operator.io/v1alpha1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: ghcr.io/myorg/my-mcp-server:latest
  transport:
    type: http
    port: 8000
```
