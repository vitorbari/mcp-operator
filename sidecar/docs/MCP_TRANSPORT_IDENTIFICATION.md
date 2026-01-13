# MCP Transport Identification Guide

This document captures the critical knowledge for correctly identifying MCP transport types
(Streamable HTTP vs SSE) in the sidecar proxy. This prevents metrics bugs like incorrectly
counting POST request-responses as SSE connections.

## Quick Reference: Transport Identification

| Characteristic | Streamable HTTP (2025+) | SSE Legacy (2024-11-05) |
|----------------|------------------------|-------------------------|
| **POST + SSE Response** | Normal request-response | N/A (POST returns JSON) |
| **GET + SSE Response** | Server notification stream | SSE endpoint for all server messages |
| **SSE Connection** | Only GET requests | Only GET requests |
| **Response Content-Type** | `application/json` OR `text/event-stream` | SSE: `text/event-stream`, POST: `application/json` |

## Critical Rule for SSE Connection Metrics

**ONLY count GET requests with `text/event-stream` response as SSE connections.**

```go
// CORRECT: Only GET requests are true SSE connections
if sw.isSSE && sw.httpMethod == http.MethodGet {
    recorder.SSEConnectionOpened(ctx)
}

// WRONG: Counting all SSE content-type responses as SSE connections
if sw.isSSE {  // This includes POST requests in Streamable HTTP!
    recorder.SSEConnectionOpened(ctx)
}
```

## Transport Details

### 1. Streamable HTTP Transport (Modern - 2025-03-26+)

The modern MCP transport uses a **single endpoint** for all communication.

#### HTTP Methods and Their Purpose

| Method | Purpose | Request Body | Response Content-Type | SSE Connection? |
|--------|---------|--------------|----------------------|-----------------|
| `POST` | Send requests/notifications | JSON-RPC | `application/json` OR `text/event-stream` | **NO** |
| `GET` | Open server→client stream | None | `text/event-stream` | **YES** |
| `DELETE` | Terminate session | None | None (202 Accepted) | NO |

#### Key Insight: POST with SSE Response

In Streamable HTTP, a POST request can receive a `text/event-stream` response. This is **NOT**
an SSE connection - it's a streaming response to a single request that completes when the
response finishes.

```
Client                                  Server
   |                                      |
   |  POST /mcp (initialize request)      |
   |------------------------------------->|
   |                                      |
   |  200 OK                              |
   |  Content-Type: text/event-stream     |
   |  event: message                      |
   |  data: {"jsonrpc":"2.0",...}         |
   |<-------------------------------------|
   |                                      |
   |  (connection closes after response)  |
```

This is fundamentally different from a GET SSE connection which stays open indefinitely.

#### Headers Present in Streamable HTTP

- `Mcp-Session-Id`: Session identifier (after initialization)
- `Mcp-Protocol-Version`: Protocol version (e.g., `2025-11-25`)
- `Accept: application/json, text/event-stream`: Client accepts both formats

### 2. SSE Legacy Transport (2024-11-05)

The legacy transport uses **two separate endpoints**.

#### Endpoint Structure

| Endpoint | Method | Purpose | Content-Type |
|----------|--------|---------|--------------|
| SSE Endpoint (`/sse`) | `GET` | Server→client messages | `text/event-stream` |
| POST Endpoint (`/messages`) | `POST` | Client→server messages | `application/json` |

#### Key Insight: POST Always Returns JSON

In legacy SSE transport, POST requests **always** return `application/json`. The server
sends responses via the SSE stream, not in the POST response.

```
Client                                  Server
   |                                      |
   |  GET /sse                            |
   |------------------------------------->|
   |                                      |
   |  200 OK (SSE stream opens)           |
   |  Content-Type: text/event-stream     |
   |  event: endpoint                     |
   |  data: {"uri":"/messages"}           |
   |<-------------------------------------|
   |                                      |
   |  POST /messages (request)            |
   |------------------------------------->|
   |  200 OK                              |
   |  Content-Type: application/json      |
   |<-------------------------------------|
   |                                      |
   |  (SSE stream: response event)        |
   |  event: message                      |
   |  data: {"jsonrpc":"2.0",...}         |
   |<-------------------------------------|
```

## Implementation in Sidecar Proxy

### Current Implementation (After Fix)

```go
// sseAwareWriter tracks HTTP method to distinguish transport types
type sseAwareWriter struct {
    http.ResponseWriter
    recorder     *metrics.Recorder
    httpMethod   string  // GET, POST, DELETE, etc.
    isSSE        bool    // Response has text/event-stream content-type
    // ...
}

// WriteHeader - only count GET requests as SSE connections
func (sw *sseAwareWriter) WriteHeader(code int) {
    // ...
    sw.isSSE = IsSSEContentType(contentType)

    // CRITICAL: Only GET requests are true SSE connections
    // POST with text/event-stream is Streamable HTTP response streaming
    if sw.isSSE && sw.recorder != nil && sw.httpMethod == http.MethodGet {
        sw.recorder.SSEConnectionOpened(context.Background())
    }
}

// recordSSEClose - matching logic for close
func (sw *sseAwareWriter) recordSSEClose() {
    if sw.isSSE && sw.recorder != nil && sw.httpMethod == http.MethodGet {
        sw.recorder.SSEConnectionClosed(context.Background(), time.Since(sw.startTime))
    }
}
```

### Metrics Interpretation

| Metric | What It Measures |
|--------|------------------|
| `mcp_active_connections` | Currently processing HTTP requests (all methods) |
| `mcp_sse_connections_total` | Total GET SSE streams opened |
| `mcp_sse_connections_active` | Currently open GET SSE streams |
| `mcp_requests_total` | All HTTP requests with MCP method |

### Test Cases for Validation

1. **POST with SSE response** (Streamable HTTP):
   - `mcp_requests_total` should increment
   - `mcp_sse_connections_total` should NOT increment
   - `mcp_sse_connections_active` should NOT change

2. **GET with SSE response** (True SSE stream):
   - `mcp_requests_total` should increment (if MCP method parseable)
   - `mcp_sse_connections_total` should increment
   - `mcp_sse_connections_active` should increment while open

3. **POST with JSON response** (Legacy SSE or Streamable HTTP):
   - `mcp_requests_total` should increment
   - No SSE metrics should change

## Edge Cases and Considerations

### 1. DELETE Requests

DELETE requests in Streamable HTTP are for session termination and never have SSE responses.
They should not affect SSE metrics.

### 2. Mixed Transport Detection

If supporting both transports, detect based on:
- Presence of `Mcp-Protocol-Version` header → Streamable HTTP
- First SSE event is `endpoint` → Legacy SSE
- Response to POST is `text/event-stream` → Streamable HTTP

### 3. Priming Events

Streamable HTTP servers may send a "priming" SSE event (empty data) immediately after
opening a GET SSE connection. This is a valid SSE event and should be counted.

### 4. Stream Resumption

Streamable HTTP supports stream resumption via `Last-Event-ID` header. Each resumed
stream should be counted as a new SSE connection for metrics purposes.

## Protocol Version Reference

| Version | Transport | Notes |
|---------|-----------|-------|
| 2024-11-05 | SSE Legacy | Two endpoints, POST returns JSON only |
| 2025-03-26 | Streamable HTTP | Single endpoint, POST can return SSE |
| 2025-06-18 | Streamable HTTP | Added protocol version header requirement |
| 2025-11-25 | Streamable HTTP | Current stable version |

## References

- [MCP Specification - Transports](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
- [MCP Specification - Versioning](https://modelcontextprotocol.io/specification/versioning)
