# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building and Testing
```bash
# Run comprehensive test suite with envtest
make test

# Run specific controller tests
go test ./internal/controller -v -run "TestMCPServer"

# Run tests for a specific component (e.g., transport)
go test ./pkg/transport -v

# Run E2E tests with Kind cluster
make test-e2e

# Build the operator binary
make build

# Run locally for development (requires CRDs installed)
make run
```

### Code Generation and Manifests
```bash
# Generate CRDs, RBAC, and webhooks after API changes
make manifests

# Generate deepcopy methods after API changes
make generate

# Install/uninstall CRDs in cluster
make install
make uninstall

# Deploy/undeploy operator to cluster
make deploy IMG=controller:latest
make undeploy
```

### Linting and Quality
```bash
# Run golangci-lint
make lint

# Auto-fix linting issues
make lint-fix

# Format code
make fmt

# Vet code
make vet
```

### Container and Distribution
```bash
# Build and push container image
make docker-build IMG=myregistry/mcp-operator:tag
make docker-push IMG=myregistry/mcp-operator:tag

# Build multi-platform images
make docker-buildx IMG=myregistry/mcp-operator:tag

# Generate consolidated install.yaml
make build-installer IMG=myregistry/mcp-operator:tag
```

## Architecture Overview

This is a Kubernetes operator for managing Model Context Protocol (MCP) servers. The architecture follows standard controller-runtime patterns with sophisticated transport abstraction.

### Core Components

**API Layer (`api/v1/mcpserver_types.go`)**
- `MCPServer` CRD with comprehensive configuration options
- Transport-specific configuration (HTTP/Custom)
- Enterprise features: HPA, ingress, RBAC, security contexts
- Rich status reporting with phase tracking and conditions

**Controller (`internal/controller/mcpserver_controller.go`)**
- `MCPServerReconciler` implements reconciliation loop
- Transport-aware resource management via Strategy pattern
- Retry logic with `client-go/util/retry.RetryOnConflict` for optimistic concurrency
- Comprehensive status updates with condition management
- Resource ownership and finalizer-based cleanup

**Transport System (`pkg/transport/`)**
- `ManagerFactory` creates transport-specific managers
- `ResourceManager` interface with `CreateResources()`, `UpdateResources()`, `DeleteResources()`
- `HTTPResourceManager`: MCP streamable HTTP transport with session management
- `CustomResourceManager`: TCP/UDP/SCTP protocols with flexible configuration

**Utilities (`pkg/utils/resources.go`)**
- Resource builders: `BuildDeployment()`, `BuildService()`, `BuildBaseContainer()`
- Label management with transport and capability tracking
- Health probe configuration with `AddHealthProbes()`

**Metrics (`internal/metrics/`)**
- Prometheus metrics for MCPServer status, transport distribution, reconciliation performance
- Custom metrics registration and collection

### Key Patterns

**Transport Abstraction**
The operator uses a Strategy pattern where each transport type has its own manager:
```go
// Transport managers implement this interface
type ResourceManager interface {
    CreateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error
    UpdateResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error
    DeleteResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error
    GetTransportType() mcpv1.MCPTransportType
}
```

**Reconciliation Flow**
1. Finalizer management for proper cleanup
2. ServiceAccount and RBAC creation
3. Transport-specific resource reconciliation (delegation to transport managers)
4. HPA reconciliation if enabled
5. Ingress reconciliation if enabled
6. Status updates with retry logic

**Resource Management**
- All resources use owner references for garbage collection
- Retry logic prevents conflicts during concurrent reconciliation
- Transport managers handle protocol-specific networking (ports, annotations, service types)

### Container Command/Args Handling

The operator preserves container command and args specified in the MCPServer spec:
- `BuildBaseContainer()` in `pkg/utils/resources.go` applies `mcpServer.Spec.Command` and `mcpServer.Spec.Args`
- Tests in `internal/controller/mcpserver_controller_test.go` verify preservation

### Testing Structure

**Controller Tests**
- Uses Ginkgo/Gomega BDD framework with envtest
- Each test context creates isolated resources with unique names
- Comprehensive test coverage for different transport configurations
- Tests verify resource creation, status updates, and reconciliation behavior

**Test Patterns**
```go
// Example test structure
Context("When reconciling MCPServer with custom command and args", func() {
    It("should create Deployment with custom command and args", func() {
        // Test implementation
    })
})
```

**E2E Tests**
- Kind-based cluster setup with automated lifecycle
- Full operator deployment validation
- Located in `test/e2e/`

### Transport Configuration

**HTTP Transport (MCP Streamable HTTP)**
```yaml
transport:
  type: "http"
  config:
    http:
      port: 8080
      path: "/mcp"
      sessionManagement: true
      security:
        validateOrigin: true
        allowedOrigins: ["https://myapp.example.com"]
```

**Custom Transport**
```yaml
transport:
  type: "custom"
  config:
    custom:
      port: 9090
      protocol: "tcp"
      config:
        bufferSize: "4096"
        timeout: "30s"
```

### Status Management

The controller maintains rich status information:
- **Phase tracking**: Creating, Running, Scaling, Updating, Failed, Terminating
- **Replica counts**: Current, ready, available
- **Conditions**: Standard Kubernetes condition types (Ready, Available, Progressing)
- **Transport info**: Active transport type and service endpoints
- **Optimistic concurrency**: Status updates use retry logic to handle conflicts

### Metrics and Observability

Prometheus metrics are automatically collected:
- `mcpserver_ready_total`: Readiness tracking
- `mcpserver_replicas`: Replica counts by transport type
- `mcpserver_reconcile_duration_seconds`: Performance monitoring
- Located in `internal/metrics/metrics.go`

### Configuration Examples

See `config/samples/` for comprehensive examples:
- `wikipedia-http.yaml`: Minimal example with Wikipedia MCP server using SSE transport
- `mcp-basic-example.yaml`: Common production setup with HPA and monitoring
- `mcp-complete-example.yaml`: Complete example showing all available CRD fields

## Important Implementation Notes

- Always use retry logic (`client-go/util/retry.RetryOnConflict`) for resource updates
- Status updates check for actual changes using `reflect.DeepEqual` before updating
- Transport managers are responsible for protocol-specific resource configuration
- Health probes are configurable and transport-aware
- The operator supports both declarative and imperative resource management patterns