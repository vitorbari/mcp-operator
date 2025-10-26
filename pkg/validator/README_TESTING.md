# Validator Integration Tests

This directory contains integration tests for the MCP validator, including tests for both SSE and Streamable HTTP client functionality against real MCP servers.

## Test Types

The validator has three types of tests:

1. **Unit Tests**: Standard Go tests that don't require external services
2. **Integration Tests**: Tests that connect to real MCP servers via environment variables
3. **Container Tests**: Tests that use testcontainers to automatically spin up MCP server containers

## Running Integration Tests

### Prerequisites for Manual Integration Tests

You need a running MCP server. You can use any MCP server that implements the MCP protocol.

#### Example: Running MCP Everything Server

The `mcp-everything-server` supports both SSE and Streamable HTTP transports:

```bash
# Start the server with Docker
docker run -p 3001:3001 tzolov/mcp-everything-server:v3

# The server will expose both transports:
# - SSE transport at: http://localhost:3001/sse
# - Streamable HTTP at: http://localhost:3001/mcp
```

### Running SSE Client Integration Tests

Set the `MCP_SSE_TEST_ENDPOINT` environment variable to point to your SSE server:

```bash
# Run all SSE integration tests
export MCP_SSE_TEST_ENDPOINT=http://localhost:3001/sse
go test -v -run TestSSEClientConnect -run TestSSEClientInitialize ./internal/validator/

# Or use the Makefile
make test-integration MCP_SSE_TEST_ENDPOINT=http://localhost:3001/sse

# Run a specific test
go test -v -run TestSSEClientInitialize ./internal/validator/

# Run with race detection
go test -v -race -run TestSSEClient ./internal/validator/

# Run benchmarks
go test -v -bench=BenchmarkSSEClient -run=^$ ./internal/validator/
```

If the environment variable is not set, the tests will be skipped automatically.

### Running Streamable HTTP Client Integration Tests

Set the `MCP_HTTP_TEST_ENDPOINT` environment variable to point to your Streamable HTTP server:

```bash
# Run all Streamable HTTP integration tests
export MCP_HTTP_TEST_ENDPOINT=http://localhost:3001/mcp
go test -v -run TestStreamableHTTPClient ./internal/validator/

# Or use the Makefile
make test-integration MCP_HTTP_TEST_ENDPOINT=http://localhost:3001/mcp

# Run a specific test
go test -v -run TestStreamableHTTPClientInitialize ./internal/validator/

# Run benchmarks
go test -v -bench=BenchmarkStreamableHTTPClient -run=^$ ./internal/validator/
```

If the environment variable is not set, the tests will be skipped automatically.

### Running Both Transport Tests

You can test both transports simultaneously:

```bash
export MCP_SSE_TEST_ENDPOINT=http://localhost:3001/sse
export MCP_HTTP_TEST_ENDPOINT=http://localhost:3001/mcp
make test-integration
```

## Running Container Tests

Container tests use testcontainers to automatically spin up MCP server containers. This provides:
- **No manual setup required**: Tests manage the container lifecycle
- **Faster than E2E tests**: No Kubernetes cluster required
- **Isolated testing**: Each test gets a fresh container
- **Requires Docker**: Docker daemon must be running

```bash
# Run all container tests
make test-container

# Run only SSE container tests
go test -v -run TestSSEClientWithContainer ./internal/validator/

# Run only Streamable HTTP container tests
go test -v -run TestStreamableHTTPClientWithContainer ./internal/validator/

# Skip container tests (useful in CI without Docker)
go test -short ./internal/validator/
```

## Test Coverage

### SSE Client Tests

#### Integration Tests (`sse_client_integration_test.go`)
1. **TestSSEClientConnect**: Verifies connection establishment and endpoint discovery
2. **TestSSEClientInitialize**: Tests the MCP initialize handshake
3. **TestSSEClientEndpointDiscovery**: Validates endpoint URL discovery from SSE stream
4. **TestSSEClientMultipleRequests**: Tests request ID handling and multiple requests
5. **TestSSEClientTimeout**: Verifies timeout handling
6. **TestSSEClientInvalidEndpoint**: Tests error handling for invalid endpoints
7. **TestSSEClientClose**: Verifies proper cleanup
8. **TestSSEClientProtocolVersions**: Validates protocol version detection
9. **BenchmarkSSEClientInitialize**: Performance benchmark for initialize operation

#### Container Tests (`sse_client_container_test.go`)
1. **TestSSEClientWithContainer**: Full lifecycle test with container
2. **TestSSEClientContainerMultipleRequests**: Request ID handling with container
3. **TestSSEClientContainerCleanup**: Cleanup verification with container

### Streamable HTTP Client Tests

#### Integration Tests (`streamable_http_client_integration_test.go`)
1. **TestStreamableHTTPClientInitialize**: Tests the MCP initialize handshake
2. **TestStreamableHTTPClientMultipleRequests**: Tests request ID handling
3. **TestStreamableHTTPClientListTools**: Tests tools/list endpoint
4. **TestStreamableHTTPClientListResources**: Tests resources/list endpoint
5. **TestStreamableHTTPClientListPrompts**: Tests prompts/list endpoint
6. **TestStreamableHTTPClientTimeout**: Verifies timeout handling
7. **TestStreamableHTTPClientInvalidEndpoint**: Tests error handling for invalid endpoints
8. **TestStreamableHTTPClientPing**: Tests convenience ping method
9. **TestStreamableHTTPClientProtocolVersions**: Validates protocol version detection
10. **BenchmarkStreamableHTTPClientInitialize**: Performance benchmark

#### Container Tests (`streamable_http_client_container_test.go`)
1. **TestStreamableHTTPClientWithContainer**: Full lifecycle test with container
2. **TestStreamableHTTPClientContainerMultipleRequests**: Request ID handling with container
3. **TestStreamableHTTPClientContainerListTools**: Tools listing with container
4. **TestStreamableHTTPClientContainerListResources**: Resources listing with container
5. **TestStreamableHTTPClientContainerListPrompts**: Prompts listing with container
6. **TestStreamableHTTPClientContainerPing**: Ping test with container

## Expected Test Behavior

### Successful Integration Test Output

SSE Client:
```
=== RUN   TestSSEClientConnect
    sse_client_integration_test.go:61: Successfully connected to SSE endpoint: http://localhost:3001/sse
    sse_client_integration_test.go:62: Messages URL: http://localhost:3001/messages?sessionId=abc123
--- PASS: TestSSEClientConnect (0.15s)

=== RUN   TestSSEClientInitialize
    sse_client_integration_test.go:99: Protocol version: 2024-11-05
    sse_client_integration_test.go:100: Server: mcp-everything-server v3.0.0
    sse_client_integration_test.go:101: Capabilities: tools=true, resources=true, prompts=true
--- PASS: TestSSEClientInitialize (0.25s)
```

Streamable HTTP Client:
```
=== RUN   TestStreamableHTTPClientInitialize
    streamable_http_client_integration_test.go:66: Protocol version: 2024-11-05
    streamable_http_client_integration_test.go:67: Server: mcp-everything-server v3.0.0
    streamable_http_client_integration_test.go:68: Capabilities: tools=true, resources=true, prompts=true
--- PASS: TestStreamableHTTPClientInitialize (0.10s)
```

### Successful Container Test Output

```
=== RUN   TestSSEClientWithContainer
    sse_client_container_test.go:67: Testing SSE client against container endpoint: http://localhost:49234/sse
    sse_client_container_test.go:83: Successfully connected. Messages URL: http://localhost:49234/messages?sessionId=xyz
    sse_client_container_test.go:103: Initialize successful:
    sse_client_container_test.go:104:   Protocol version: 2024-11-05
    sse_client_container_test.go:105:   Server: mcp-everything-server v3.0.0
    sse_client_container_test.go:106:   Capabilities: tools=true, resources=true, prompts=true
--- PASS: TestSSEClientWithContainer (12.34s)
```

### Skipped Tests

Integration tests (no endpoint set):
```
=== RUN   TestSSEClientConnect
    sse_client_integration_test.go:36: Skipping integration test: MCP_SSE_TEST_ENDPOINT not set
--- SKIP: TestSSEClientConnect (0.00s)
```

Container tests (short mode):
```
=== RUN   TestSSEClientWithContainer
    sse_client_container_test.go:32: Skipping container test in short mode
--- SKIP: TestSSEClientWithContainer (0.00s)
```

## Troubleshooting

### Connection Refused

If you get "connection refused" errors:
- Verify the server is running: `curl http://localhost:8080/sse`
- Check the port matches your server configuration
- Ensure firewall rules allow the connection

### Timeout Errors

If tests timeout:
- Increase the timeout in test setup
- Check server logs for errors
- Verify the server is responding to SSE requests

### Protocol Errors

If you get protocol-related errors:
- Verify the server implements MCP SSE transport correctly
- Check server logs for SSE event format
- Ensure the server sends the "endpoint" event with messages URL

## CI/CD Integration

### Integration Tests in CI

Integration tests are designed to be skipped when no endpoints are configured. For CI environments:

```yaml
# Example GitHub Actions workflow using manual server setup
- name: Start test MCP server
  run: |
    docker run -d -p 3001:3001 tzolov/mcp-everything-server:v3
    sleep 10  # Wait for server to start

- name: Run integration tests
  env:
    MCP_SSE_TEST_ENDPOINT: http://localhost:3001/sse
    MCP_HTTP_TEST_ENDPOINT: http://localhost:3001/mcp
  run: make test-integration
```

### Container Tests in CI (Recommended)

Container tests are better suited for CI as they manage their own server lifecycle:

```yaml
# Example GitHub Actions workflow using testcontainers
- name: Run container tests
  run: make test-container
  # No manual server setup required!
  # Testcontainers will automatically:
  # 1. Pull the MCP server image
  # 2. Start containers
  # 3. Run tests
  # 4. Clean up containers
```

Benefits of container tests for CI:
- No manual server setup
- Faster than full E2E tests
- Automatic cleanup
- Isolated test environment
- Works on any CI system with Docker

## Adding New Tests

### Adding Integration Tests

When adding new integration tests:

1. Check for endpoint using appropriate helper function:
   - `getSSETestEndpoint(t)` for SSE tests
   - `getHTTPTestEndpoint(t)` for Streamable HTTP tests
2. Use appropriate timeouts (30s is reasonable for most operations)
3. Always defer `client.Close()` to clean up resources
4. Log relevant information using `t.Logf()` for debugging
5. Test both success and error paths

Example integration test:

```go
func TestSSEClientNewFeature(t *testing.T) {
    endpoint := getSSETestEndpoint(t)

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    client := NewSSEClient(endpoint, 30*time.Second)
    defer client.Close()

    err := client.Connect(ctx)
    if err != nil {
        t.Fatalf("Failed to connect: %v", err)
    }

    // Add your test logic here
}
```

### Adding Container Tests

When adding new container tests:

1. Check for short mode: `if testing.Short() { t.Skip("...") }`
2. Create container with proper wait strategy
3. Always defer container cleanup with `container.Terminate(ctx)`
4. Get dynamic endpoint using `container.Host()` and `container.MappedPort()`
5. Use reasonable timeouts for container startup (30s)

Example container test:

```go
func TestStreamableHTTPClientWithContainerNewFeature(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping container test in short mode")
    }

    ctx := context.Background()

    req := testcontainers.ContainerRequest{
        Image:        "tzolov/mcp-everything-server:v3",
        ExposedPorts: []string{"3001/tcp"},
        WaitingFor: wait.ForAll(
            wait.ForListeningPort("3001/tcp"),
            wait.ForLog("Server running").WithStartupTimeout(30*time.Second),
        ),
    }

    container, err := testcontainers.GenericContainer(ctx,
        testcontainers.GenericContainerRequest{
            ContainerRequest: req,
            Started:          true,
        })
    if err != nil {
        t.Fatalf("Failed to start container: %v", err)
    }
    defer container.Terminate(ctx)

    // Get endpoint
    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "3001")
    endpoint := fmt.Sprintf("http://%s:%s/mcp", host, port.Port())

    // Run your test against endpoint
}
```
