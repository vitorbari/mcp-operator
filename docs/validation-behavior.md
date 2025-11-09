# MCP Protocol Validation Behavior

This document describes how the MCP operator validates protocol compliance, detects authentication, and discovers capabilities based on different configurations of `spec.validation` and `spec.transport`.

## Overview

The validator performs three key functions:
1. **Protocol Detection**: Identifies which MCP transport protocol the server implements
2. **Authentication Detection**: Determines if the server requires authentication
3. **Capabilities Discovery**: Lists the server's advertised capabilities (tools, resources, prompts)

## Default Behavior

**Validation is ENABLED by default** unless explicitly disabled with `validation.enabled: false`.

When `spec.validation` is not specified:
- Validation runs automatically when the MCPServer reaches Running phase
- Protocol is auto-detected (tries Streamable HTTP, then SSE)
- Authentication is auto-detected
- Capabilities are discovered from server response
- `strictMode: false` - deployment continues even if validation fails
- Validation results are shown in status fields

## Configuration Matrix

### Case 1: No spec.validation, No spec.transport

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: minimal-example
spec:
  image: "mcp-server:latest"
```

**Expected Behavior:**
- ✅ Validation: **ENABLED** (default)
- ✅ Protocol Detection: **AUTO** (tries streamable-http, falls back to sse)
- ✅ Auth Detection: **AUTO**
- ✅ Capabilities Discovery: **AUTO**
- ✅ StrictMode: **FALSE** (deployment continues on failure)
- ✅ Status Fields: All populated (protocol, auth, compliant, capabilities)

**Use Case:** Quick testing, minimal configuration

---

### Case 2: No spec.validation, spec.transport with protocol: "auto"

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: auto-transport-example
spec:
  image: "mcp-server:latest"
  transport:
    type: "http"
    protocol: "auto"  # Prefers streamable-http over sse
    config:
      http:
        port: 8080
        path: "/mcp"
```

**Expected Behavior:**
- ✅ Validation: **ENABLED** (default)
- ✅ Protocol Detection: **AUTO** (tries streamable-http first, then sse)
- ✅ Auth Detection: **AUTO**
- ✅ Capabilities Discovery: **AUTO**
- ✅ StrictMode: **FALSE** (deployment continues on failure)
- ⚠️ Protocol Mismatch Check: Compares detected vs configured (auto accepts both)
- ✅ Status Fields: All populated

**Use Case:** Modern MCP servers that may support either protocol

---

### Case 3: No spec.validation, spec.transport with protocol: "sse"

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: sse-example
spec:
  image: "mcp-server:latest"
  transport:
    type: "http"
    protocol: "sse"  # Explicitly specify SSE
    config:
      http:
        port: 8080
        path: "/sse"
```

**Expected Behavior:**
- ✅ Validation: **ENABLED** (default)
- ✅ Protocol Detection: **AUTO** (tries both protocols)
- ✅ Auth Detection: **AUTO**
- ✅ Capabilities Discovery: **AUTO**
- ✅ StrictMode: **FALSE**
- ⚠️ Protocol Mismatch Check:
  - If server implements **sse**: ✅ No mismatch, compliant = true
  - If server implements **streamable-http**: ❌ Mismatch detected, issue added, compliant = false
- ✅ Status Fields: All populated + protocol mismatch issue if detected

**Use Case:** Legacy MCP servers (2024-11-05 spec) or explicit SSE requirement

---

### Case 4: No spec.validation, spec.transport with protocol: "streamable-http"

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: streamable-http-example
spec:
  image: "mcp-server:latest"
  transport:
    type: "http"
    protocol: "streamable-http"
    config:
      http:
        port: 8080
        path: "/mcp"
```

**Expected Behavior:**
- ✅ Validation: **ENABLED** (default)
- ✅ Protocol Detection: **AUTO** (tries both protocols)
- ✅ Auth Detection: **AUTO**
- ✅ Capabilities Discovery: **AUTO**
- ✅ StrictMode: **FALSE**
- ⚠️ Protocol Mismatch Check:
  - If server implements **streamable-http**: ✅ No mismatch, compliant = true
  - If server implements **sse**: ❌ Mismatch detected, issue added, compliant = false
- ✅ Status Fields: All populated + protocol mismatch issue if detected

**Use Case:** Modern MCP servers (2025-03-26+ spec)

---

### Case 5: spec.validation with enabled: true, No strictMode

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: validation-enabled-example
spec:
  image: "mcp-server:latest"
  validation:
    enabled: true  # Explicit enable (same as default)
```

**Expected Behavior:**
- ✅ Validation: **ENABLED**
- ✅ Protocol Detection: **AUTO**
- ✅ Auth Detection: **AUTO**
- ✅ Capabilities Discovery: **AUTO**
- ✅ StrictMode: **FALSE** (default when not specified)
- ✅ Status Fields: All populated

**Use Case:** Explicit validation configuration, non-strict

---

### Case 6: spec.validation with strictMode: true

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: strict-validation-example
spec:
  image: "mcp-server:latest"
  validation:
    enabled: true
    strictMode: true  # Fail deployment on validation failure
```

**Expected Behavior:**
- ✅ Validation: **ENABLED**
- ✅ Protocol Detection: **AUTO**
- ✅ Auth Detection: **AUTO**
- ✅ Capabilities Discovery: **AUTO**
- ✅ StrictMode: **TRUE** - deployment FAILS if validation fails
- ⚠️ Phase: If validation fails → **Failed** phase, replicas = 0
- ✅ Status Fields: All populated + validation issues

**Use Case:** Production deployments requiring protocol compliance

---

### Case 7: spec.validation with requiredCapabilities

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: required-capabilities-example
spec:
  image: "mcp-server:latest"
  validation:
    enabled: true
    strictMode: true
    requiredCapabilities:
      - "tools"
      - "resources"
```

**Expected Behavior:**
- ✅ Validation: **ENABLED**
- ✅ Protocol Detection: **AUTO**
- ✅ Auth Detection: **AUTO**
- ✅ Capabilities Discovery: **AUTO**
- ✅ StrictMode: **TRUE**
- ⚠️ Capability Check:
  - If server has both "tools" and "resources": ✅ compliant = true
  - If server missing any required capability: ❌ compliant = false, issue added
- ✅ Status Fields: All populated + missing capability issues if any

**Use Case:** Ensuring server has required functionality

---

### Case 8: spec.validation with enabled: false (Explicit Disable)

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: no-validation-example
spec:
  image: "mcp-server:latest"
  validation:
    enabled: false  # ONLY way to disable validation
```

**Expected Behavior:**
- ❌ Validation: **DISABLED**
- ❌ Protocol Detection: **SKIPPED**
- ❌ Auth Detection: **SKIPPED**
- ❌ Capabilities Discovery: **SKIPPED**
- ❌ StrictMode: **N/A**
- ❌ Status Fields: status.validation is **nil**

**Use Case:** Development environments, servers under construction, or non-MCP HTTP servers

---

### Case 9: Protocol Mismatch with strictMode: false

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mismatch-non-strict-example
spec:
  image: "mcp-server:latest"
  transport:
    type: "http"
    protocol: "sse"  # Configured for SSE
    config:
      http:
        port: 8080
        path: "/sse"
  validation:
    strictMode: false
```

**Expected Behavior (when server actually implements streamable-http):**
- ✅ Validation: **ENABLED**
- ✅ Protocol Detection: Detects **streamable-http**
- ⚠️ Protocol Mismatch: Configured=sse, Detected=streamable-http
- ✅ Deployment: **CONTINUES RUNNING** (strictMode: false)
- ✅ Status:
  - compliant: **false**
  - issues: `[{level: "warning", code: "PROTOCOL_MISMATCH", message: "..."}]`
  - phase: **Running**
  - replicas: **> 0**

**Use Case:** Observability - detect mismatches without breaking deployments

---

### Case 10: Protocol Mismatch with strictMode: true

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mismatch-strict-example
spec:
  image: "mcp-server:latest"
  transport:
    type: "http"
    protocol: "sse"  # Configured for SSE
    config:
      http:
        port: 8080
        path: "/sse"
  validation:
    enabled: true
    strictMode: true  # Fail on mismatch
```

**Expected Behavior (when server actually implements streamable-http):**
- ✅ Validation: **ENABLED**
- ✅ Protocol Detection: Detects **streamable-http**
- ⚠️ Protocol Mismatch: Configured=sse, Detected=streamable-http
- ❌ Deployment: **FAILS** (strictMode: true)
- ✅ Status:
  - compliant: **false**
  - issues: `[{level: "error", code: "PROTOCOL_MISMATCH", message: "..."}]`
  - phase: **Failed**
  - replicas: **0** (deployment deleted)

**Use Case:** Enforce protocol compliance in production

---

## Protocol Detection Logic

The validator attempts to detect the protocol in the following order:

1. **Try Streamable HTTP** (MCP 2025-03-26+):
   - Attempts to detect HTTP-based streaming transport
   - Looks for `Mcp-Protocol-Version` header
   - Checks for proper JSON-RPC message handling

2. **Try SSE** (MCP 2024-11-05):
   - Attempts Server-Sent Events connection
   - Looks for `text/event-stream` content type
   - Validates SSE event format

3. **Result**:
   - Returns first successful protocol detected
   - If both fail, validation fails with connection/timeout errors

## Authentication Detection

The validator detects authentication requirements:

1. **During Protocol Detection**:
   - Checks for `WWW-Authenticate` headers (HTTP auth)
   - Checks for 401 Unauthorized responses
   - Checks for custom auth challenge patterns

2. **Result**:
   - `status.validation.authentication: true` if auth detected
   - `status.validation.authentication: false` if no auth required

## Capabilities Discovery

The validator discovers capabilities from the server's initialize response:

1. **Initialize Handshake**:
   - Sends MCP `initialize` request
   - Receives server capabilities in response

2. **Extraction**:
   - Parses `capabilities.tools`, `capabilities.resources`, `capabilities.prompts`
   - Stores in `status.validation.capabilities: ["tools", "resources", ...]`

3. **Validation**:
   - Compares discovered capabilities against `spec.validation.requiredCapabilities`
   - Adds issues if required capabilities are missing

## Status Field Population

All validation results are stored in `status.validation`:

```yaml
status:
  validation:
    state: "Passed" | "Failed" | "Validating" | "Pending"
    compliant: true | false
    protocol: "streamable-http" | "sse"
    authentication: true | false
    capabilities: ["tools", "resources", "prompts"]
    attempts: 3
    lastAttemptTime: "2025-01-06T10:30:00Z"
    lastSuccessTime: "2025-01-06T10:30:00Z"
    validatedGeneration: 5
    issues:
      - level: "error" | "warning"
        code: "PROTOCOL_MISMATCH" | "MISSING_CAPABILITY" | ...
        message: "Detailed error message"
```

## Test Case Summary

| Case | spec.validation | spec.transport.protocol | Validation | Protocol Detection | StrictMode | Mismatch Behavior |
|------|----------------|------------------------|------------|-------------------|------------|------------------|
| 1 | nil | nil | ENABLED | AUTO | false | N/A |
| 2 | nil | auto | ENABLED | AUTO | false | Accepts both |
| 3 | nil | sse | ENABLED | AUTO | false | Warning if mismatch |
| 4 | nil | streamable-http | ENABLED | AUTO | false | Warning if mismatch |
| 5 | enabled: true | nil | ENABLED | AUTO | false | N/A |
| 6 | strictMode: true | nil | ENABLED | AUTO | true | Fails deployment |
| 7 | requiredCapabilities | nil | ENABLED | AUTO | true | Checks capabilities |
| 8 | enabled: false | any | DISABLED | SKIPPED | N/A | N/A |
| 9 | strictMode: false | sse (mismatch) | ENABLED | AUTO | false | Runs + Warning |
| 10 | strictMode: true | sse (mismatch) | ENABLED | AUTO | true | Fails + Error |

## Migration Guide

### From No Validation → Default Validation

**Before** (validation explicitly disabled):
```yaml
spec:
  image: "mcp-server:latest"
  validation:
    enabled: false
```

**After** (validation enabled by default):
```yaml
spec:
  image: "mcp-server:latest"
  # Validation runs automatically, no config needed
```

### From Implicit Disabled → Explicit Control

**Before** (old behavior - validation disabled without spec.validation):
```yaml
spec:
  image: "mcp-server:latest"
  # No validation ran
```

**After** (new behavior - validation enabled by default):
```yaml
spec:
  image: "mcp-server:latest"
  # Validation runs automatically

  # To disable:
  validation:
    enabled: false
```

## Best Practices

1. **Quick Testing**: Omit `spec.validation` - validation runs with sane defaults
2. **Production**: Use `strictMode: true` + `requiredCapabilities` to enforce requirements
3. **Development**: Use `validation.enabled: false` to skip validation during development
4. **Protocol Migration**: Use `protocol: "auto"` to support both old and new clients
5. **Observability**: Leave validation enabled (even with strictMode: false) to populate status fields
