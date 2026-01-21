# Protocol Detection

The MCP Operator can automatically detect which transport protocol your MCP server supports.

## Overview

When `transport.protocol: auto` is configured, the operator:

1. Waits for pods to become ready
2. Probes the server to detect the protocol
3. Configures resources appropriately for the detected protocol
4. Records the result in the MCPServer status

## Configuration

### Enable Auto-Detection

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: "my-registry/mcp-server:latest"

  transport:
    type: http
    protocol: auto  # Enable auto-detection
    config:
      http:
        port: 8080
```

### Detection Behavior

The operator attempts protocols in this order:

1. **Streamable HTTP** (preferred)
   - Sends POST to configured path (default: `/mcp`)
   - Checks for `Mcp-Protocol-Version` header
   - Validates JSON-RPC response

2. **SSE** (fallback)
   - Connects to SSE endpoint (default: `/sse`)
   - Checks for `text/event-stream` content type
   - Validates SSE event format

The first successful protocol is used.

## Detection Results

### Viewing Detection Status

```bash
# Check detected protocol
kubectl get mcpserver my-server -o jsonpath='{.status.validation.protocol}'

# Check protocol version
kubectl get mcpserver my-server -o jsonpath='{.status.validation.protocolVersion}'

# View full validation status
kubectl get mcpserver my-server -o jsonpath='{.status.validation}' | jq
```

### Status Fields

| Field | Description |
|-------|-------------|
| `status.validation.protocol` | Detected protocol: `streamable-http` or `sse` |
| `status.validation.protocolVersion` | MCP spec version: `2025-03-26` or `2024-11-05` |
| `status.validation.state` | Validation state: `Validated`, `Failed`, etc. |
| `status.resolvedTransport` | Transport resolution details |

### Example Status

```yaml
status:
  validation:
    state: Validated
    compliant: true
    protocol: streamable-http
    protocolVersion: "2025-03-26"
    capabilities: ["tools", "resources"]
    endpoint: "http://my-server.default.svc:8080/mcp"
  resolvedTransport:
    protocol: streamable-http
    sseConfigApplied: false
    resolvedGeneration: 1
    lastResolvedTime: "2025-01-06T10:30:00Z"
```

## Protocol Mismatch

If you explicitly specify a protocol but the server implements a different one, the operator detects a mismatch.

### Non-Strict Mode (Default)

```yaml
spec:
  transport:
    protocol: sse  # Configured for SSE
  validation:
    strictMode: false  # Default
```

If server implements Streamable HTTP:
- Deployment continues normally
- Warning recorded in `status.validation.issues`
- `compliant: false`

### Strict Mode

```yaml
spec:
  transport:
    protocol: sse  # Configured for SSE
  validation:
    enabled: true
    strictMode: true  # Fail on mismatch
```

If server implements Streamable HTTP:
- Phase changes to `Failed`
- Replicas set to 0
- Error recorded in `status.validation.issues`

## Re-Triggering Detection

Detection runs when:
- MCPServer is created
- `spec` changes (generation increments)

To manually re-trigger:

```bash
# Add an annotation to force reconciliation
kubectl patch mcpserver my-server --type='json' \
  -p='[{"op": "add", "path": "/metadata/annotations/revalidate", "value":"'$(date +%s)'"}]'
```

## Disabling Detection

To skip detection entirely:

```yaml
spec:
  validation:
    enabled: false
```

Note: Protocol detection still runs to determine Service configuration, but full validation is skipped.

## Configuration Matrix

| `protocol` | `validation.enabled` | Behavior |
|------------|---------------------|----------|
| `auto` | `true` | Full detection + validation |
| `auto` | `false` | Detection only, no compliance check |
| `streamable-http` | `true` | Detects, warns/fails if mismatch |
| `streamable-http` | `false` | No detection, uses configured protocol |
| `sse` | `true` | Detects, warns/fails if mismatch |
| `sse` | `false` | No detection, uses configured protocol |

## Troubleshooting

### Detection Failing

**Symptom:** Protocol shows empty or validation state is `Failed`

**Check validation issues:**

```bash
kubectl get mcpserver my-server -o jsonpath='{.status.validation.issues}' | jq
```

**Common causes:**

1. **Server not ready** - Wait for pods to be ready before validation runs
2. **Wrong port/path** - Ensure `transport.config.http.port` and `path` match server
3. **Network issues** - Check pods can reach each other

### Forcing a Specific Protocol

If auto-detection isn't working correctly, specify the protocol explicitly:

```yaml
spec:
  transport:
    protocol: sse  # or streamable-http
```

## See Also

- [Transport Overview](README.md) - Protocol comparison
- [Streamable HTTP](streamable-http.md) - Modern transport details
- [SSE](sse.md) - Legacy transport details
- [Validation Behavior](../advanced/validation-behavior.md) - Full validation documentation
