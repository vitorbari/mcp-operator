# Contributing to MCP Operator

Thank you for your interest in contributing to the MCP Operator! This document provides guidelines and instructions for contributing.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/mcp-operator.git
   cd mcp-operator
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/vitorbari/mcp-operator.git
   ```

## Development Setup

### Prerequisites

- **Go 1.23+** - [Install Go](https://go.lang.org/doc/install)
- **Docker** - For building container images
- **kubectl** - [Install kubectl](https://kubernetes.io/docs/tasks/tools/)
- **Kind** or **Minikube** - For local testing

### Install Development Tools

The Makefile will automatically install required tools:

```bash
make manifests  # Installs controller-gen if needed
make test       # Installs envtest if needed
```

### Building the Project

```bash
# Build the operator binary
make build

# Build the Docker image
make docker-build IMG=mcp-operator:dev
```

## Development Workflow

### 1. Create a Branch

```bash
git checkout -b feature/my-new-feature
# or
git checkout -b fix/bug-description
```

Use descriptive branch names:
- `feature/` for new features
- `fix/` for bug fixes
- `docs/` for documentation changes

### 2. Make Your Changes

#### Code Style

- Follow standard Go conventions
- Run `make fmt` to format code
- Run `make vet` to check for issues
- Run `make lint` to run the linter

#### Testing Requirements

All contributions should include tests:

- **Unit tests** for business logic
- **Controller tests** for reconciliation logic
- **Integration tests** when adding new features

### 3. Run Tests

```bash
# Run unit tests
make test

# Run linter
make lint

# Run E2E tests (requires Kind cluster)
make test-e2e
```

### 4. Update Generated Code

If you modify the API (CRD) or controller:

```bash
# Generate CRDs, RBAC, webhooks
make manifests

# Generate deepcopy methods
make generate
```

### 5. Test Locally

Run the operator locally against your cluster:

```bash
# Install CRDs
make install

# Run the operator (development mode)
make run

# In another terminal, apply a sample
kubectl apply -f config/samples/01-wikipedia-sse.yaml
```

Or deploy to a Kind cluster:

```bash
# Build and load image into Kind
make docker-build IMG=mcp-operator:dev
kind load docker-image mcp-operator:dev --name mcp-test

# Deploy to cluster
make deploy IMG=mcp-operator:dev

# Check logs
kubectl logs -n mcp-operator-system deployment/mcp-operator-controller-manager -f
```

### 6. Commit Your Changes

Write clear, descriptive commit messages:

```bash
git add .
git commit -m "Add feature: support for custom health check paths

- Add HealthCheckPath field to MCPServer spec
- Update controller to configure probes with custom path
- Add tests for custom health check configuration
- Update documentation"
```

Good commit messages:
- Start with a verb (Add, Fix, Update, Remove)
- Keep first line under 50 characters
- Add details in the body if needed
- Reference issues: "Fixes #123"

### 7. Push and Create Pull Request

```bash
git push origin feature/my-new-feature
```

Then open a Pull Request on GitHub with:
- Clear description of changes
- Reference to related issues
- Screenshots (if UI-related)
- Test results

## Pull Request Guidelines

### Before Submitting

- [ ] Tests pass locally (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Code is formatted (`make fmt`)
- [ ] Generated code is updated (`make manifests generate`)
- [ ] Documentation is updated
- [ ] Commit messages are clear

### PR Description Should Include

1. **What** - What does this PR do?
2. **Why** - Why is this change needed?
3. **How** - How does it work?
4. **Testing** - How was it tested?

### Example PR Template

```markdown
## Description
Adds support for custom health check paths to allow users to configure
different health check endpoints per MCPServer.

## Motivation
Some MCP servers expose health checks at non-standard paths like
`/healthz` or `/_health`. This change allows users to specify the path.

## Changes
- Added `healthCheckPath` field to MCPServer spec
- Updated controller to use custom path in probe configuration
- Added unit tests for path validation
- Updated documentation and examples

## Testing
- Unit tests: `make test`
- E2E tests: `make test-e2e`
- Manual testing with custom health check path

Fixes #42
```

## Testing Guidelines

### Writing Tests

**Controller tests** use Ginkgo/Gomega:

```go
Context("When reconciling MCPServer with custom transport", func() {
    It("should create Service with correct port", func() {
        // Test implementation
        Expect(service.Spec.Ports[0].Port).To(Equal(int32(9000)))
    })
})
```

**Table-driven tests** for utilities:

```go
func TestBuildLabels(t *testing.T) {
    tests := []struct {
        name     string
        input    *MCPServer
        expected map[string]string
    }{
        // Test cases
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test logic
        })
    }
}
```

### Running Specific Tests

```go
# Run specific test file
go test ./internal/controller -v

# Run specific test
go test ./internal/controller -v -run TestMCPServerReconciler

# Run with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Code Review Process

1. Maintainers will review your PR
2. Address feedback by pushing new commits
3. Once approved, your PR will be merged
4. Your changes will be included in the next release

## Reporting Issues

### Bug Reports

Include:
- MCP Operator version
- Kubernetes version
- Steps to reproduce
- Expected vs actual behavior
- Logs and error messages

### Feature Requests

Include:
- Use case and motivation
- Proposed solution
- Alternative approaches considered

## Documentation

Update documentation when you:
- Add new features
- Change existing behavior
- Add configuration options
- Fix bugs that affect usage

Documentation locations:
- **README.md** - Main documentation and API reference
- **GETTING_STARTED.md** - Quickstart guide
- **docs/** - Architecture Decision Records (ADRs)
- **config/samples/** - Example configurations

## Community Guidelines

- Be respectful and inclusive
- Help others learn and grow
- Provide constructive feedback
- Ask questions when unclear
- Share knowledge and experiences

## Questions?

- Open a [Discussion](https://github.com/vitorbari/mcp-operator/discussions)
- Ask in your PR or issue
- Check existing issues and PRs

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

---

Thank you for contributing to MCP Operator! 🎉
