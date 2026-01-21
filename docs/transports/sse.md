# SSE Transport (Server-Sent Events)

Server-Sent Events (SSE) is the legacy transport protocol for MCP, defined in the MCP 2024-11-05 specification. While still widely supported, new servers should consider using [Streamable HTTP](streamable-http.md) instead.

## Overview

SSE transport uses long-lived HTTP connections for server-to-client streaming, with a separate POST endpoint for client-to-server messages.

```
┌─────────────────────────────────────────────────────────────┐
│                    SSE Transport Flow                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Client                              MCP Server             │
│    │                                     │                  │
│    │ ─────── GET /sse ─────────────────▶ │                  │
│    │ ◀═══════ SSE Stream ═══════════════ │                  │
│    │                                     │                  │
│    │ ─────── POST /messages ───────────▶ │                  │
│    │ ◀────── JSON-RPC Response ──────── │                  │
│    │                                     │                  │
└─────────────────────────────────────────────────────────────┘
```

## Configuration

### Basic SSE Configuration

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-sse-server
spec:
  image: "mcp/wikipedia-mcp:latest"
  command: ["python", "-m", "wikipedia_mcp"]
  args: ["--transport", "sse", "--port", "3001", "--host", "0.0.0.0"]

  transport:
    type: http
    protocol: sse
    config:
      http:
        port: 3001
        path: "/sse"
```

### Optimized SSE for Production

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-sse-optimized
spec:
  image: "my-registry/mcp-server:latest"

  transport:
    type: http
    protocol: sse
    config:
      http:
        port: 8080
        path: "/sse"
        sse:
          # Enable session affinity for sticky connections
          enableSessionAffinity: true
          # Grace period for connection draining during rollouts
          terminationGracePeriodSeconds: 120
```

### SSE with Session Affinity

Session affinity ensures clients reconnect to the same pod, preserving session state:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-sse-sticky
spec:
  image: "my-registry/mcp-server:latest"

  transport:
    type: http
    protocol: sse
    config:
      http:
        port: 8080
        path: "/sse"
        sse:
          enableSessionAffinity: true
```

When enabled, the Service is configured with `sessionAffinity: ClientIP`.

## Kubernetes Considerations

SSE connections are long-lived, which requires special handling in Kubernetes.

### Rolling Update Strategy

When SSE is detected, the operator automatically configures:

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 25%
    maxUnavailable: 0  # Never terminate pods until new ones are ready
```

This ensures no active connections are dropped during deployments.

### Termination Grace Period

Allow time for SSE connections to complete during pod termination:

```yaml
spec:
  transport:
    config:
      http:
        sse:
          terminationGracePeriodSeconds: 120
```

### PodDisruptionBudget

For SSE servers, use stricter availability:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: mcp-sse-pdb
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: mcp-sse-server
      app.kubernetes.io/name: mcpserver
      app.kubernetes.io/component: mcp-server
```

## Protocol Detection

When `protocol: auto` is configured, the operator detects SSE by:

1. Attempting Streamable HTTP connection first
2. Falling back to SSE if Streamable HTTP fails
3. Checking for `text/event-stream` content type
4. Validating SSE event format

The detected protocol is recorded in status:

```bash
kubectl get mcpserver my-server -o jsonpath='{.status.validation.protocol}'
# Output: sse
```

## Troubleshooting

### SSE Connections Dropping During Rollout

**Symptom:** Active SSE connections are terminated during deployment updates.

**Check deployment strategy:**

```bash
kubectl get deployment <mcpserver-name> -o jsonpath='{.spec.strategy}' | jq
```

**Expected output:**

```json
{
  "type": "RollingUpdate",
  "rollingUpdate": {
    "maxSurge": "25%",
    "maxUnavailable": 0
  }
}
```

**Solutions:**

1. Ensure `protocol: sse` is configured (auto-detected or explicit)
2. Configure termination grace period:
   ```yaml
   transport:
     config:
       http:
         sse:
           terminationGracePeriodSeconds: 120
   ```

### Session Affinity Not Working

**Symptom:** SSE clients reconnect to different backend pods.

**Verify configuration:**

```bash
# Check MCPServer spec
kubectl get mcpserver <name> -o jsonpath='{.spec.transport.config.http.sse.enableSessionAffinity}'

# Check Service session affinity
kubectl get svc <mcpserver-name> -o jsonpath='{.spec.sessionAffinity}'
```

**Common causes:**

1. **Session affinity not enabled** - Must be explicitly set:
   ```yaml
   transport:
     config:
       http:
         sse:
           enableSessionAffinity: true
   ```

2. **Load balancer overriding affinity** - Configure your ingress/load balancer for sticky sessions:
   ```yaml
   service:
     annotations:
       # AWS ALB
       alb.ingress.kubernetes.io/target-group-attributes: stickiness.enabled=true,stickiness.lb_cookie.duration_seconds=3600
       # GCP
       cloud.google.com/backend-config: '{"default": "sse-backend-config"}'
   ```

3. **Session affinity timeout** - Default is 3 hours. For shorter sessions, use application-level session management.

### Protocol Auto-Detection Failing

**Symptom:** Operator doesn't correctly detect SSE protocol.

**Check detection results:**

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.validation}' | jq
```

**Common causes:**

1. **Server not ready during validation** - Wait and trigger re-validation:
   ```bash
   kubectl patch mcpserver <name> --type='json' \
     -p='[{"op": "add", "path": "/metadata/annotations/revalidate", "value":"'$(date +%s)'"}]'
   ```

2. **Wrong path configuration** - Ensure path matches server endpoint:
   ```yaml
   transport:
     config:
       http:
         path: "/sse"  # Must match your server's SSE endpoint
   ```

3. **Use explicit protocol** - If auto-detection consistently fails:
   ```yaml
   transport:
     protocol: sse
   ```

### Understanding status.resolvedTransport

The operator tracks resolved transport configuration:

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.resolvedTransport}' | jq
```

Example output:

```json
{
  "protocol": "sse",
  "sseConfigApplied": true,
  "resolvedGeneration": 2,
  "lastResolvedTime": "2025-12-03T10:00:00Z"
}
```

| Field | Description |
|-------|-------------|
| `protocol` | Detected protocol: `"sse"` or `"streamable-http"` |
| `sseConfigApplied` | Whether SSE-specific config was applied |
| `resolvedGeneration` | MCPServer generation when resolved |
| `lastResolvedTime` | Timestamp of last resolution |

### SSE Diagnostic Commands

```bash
# 1. Check if SSE protocol is detected
kubectl get mcpserver <name> -o jsonpath='{.status.validation.protocol}'

# 2. Check resolved transport status
kubectl get mcpserver <name> -o jsonpath='{.status.resolvedTransport}' | jq

# 3. Check deployment rolling update strategy
kubectl get deployment <mcpserver-name> -o jsonpath='{.spec.strategy}' | jq

# 4. Check service session affinity
kubectl get svc <mcpserver-name> -o jsonpath='{.spec.sessionAffinity}'

# 5. Check termination grace period
kubectl get deployment <mcpserver-name> -o jsonpath='{.spec.template.spec.terminationGracePeriodSeconds}'

# 6. Check SSE configuration in spec
kubectl get mcpserver <name> -o jsonpath='{.spec.transport.config.http.sse}' | jq

# 7. Test SSE endpoint connectivity
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -N -H "Accept: text/event-stream" http://<mcpserver-name>:8080/sse

# 8. Watch for SSE events (with timeout)
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -N -m 10 -H "Accept: text/event-stream" http://<mcpserver-name>:8080/sse
```

## Comparison with Streamable HTTP

| Feature | SSE | Streamable HTTP |
|---------|-----|-----------------|
| Bidirectional | Limited (separate POST) | Yes (via HTTP) |
| Connection model | Long-lived | Request/Response + Streaming |
| Session management | Requires configuration | Built-in |
| Kubernetes compatibility | Requires special handling | Excellent |
| Load balancer support | Needs sticky sessions | Standard |
| Graceful rollouts | Needs maxUnavailable: 0 | Standard |
| MCP Spec Version | 2024-11-05 | 2025-03-26+ |

## When to Use SSE

Use SSE transport when:
- Working with legacy MCP servers that only support SSE
- Your server was built before MCP 2025-03-26
- You need compatibility with older MCP clients
- Your existing infrastructure is optimized for SSE

Consider migrating to [Streamable HTTP](streamable-http.md) for:
- New server implementations
- Simplified Kubernetes deployments
- Better load balancer compatibility

## See Also

- [Transport Overview](README.md) - Protocol comparison
- [Streamable HTTP](streamable-http.md) - Modern transport
- [Protocol Detection](protocol-detection.md) - Auto-detection behavior
- [Validation Behavior](../advanced/validation-behavior.md) - Protocol validation details
- [Troubleshooting](../operations/troubleshooting.md) - Full troubleshooting guide
