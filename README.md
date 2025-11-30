# MCP Operator

[![Lint](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml)
[![Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml)
[![E2E Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml)
[![Release](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml)

> **⚠️ Alpha Software - Not Production Ready**
>
> This project is in early development and should be considered **experimental**. While we encourage you to try it out and provide feedback, please don't use it in production environments yet. APIs may change, features may be incomplete, and bugs are expected.
>
> **We'd love your feedback!** Please open issues for bugs, feature requests, or questions.

Run [Model Context Protocol](https://modelcontextprotocol.io) (MCP) servers on Kubernetes with automatic protocol validation, horizontal scaling, and built-in observability.

![demo](https://github.com/user-attachments/assets/81a95736-11fe-450b-bb57-ad4d2db2a9ac)

## What is this?

MCP Operator makes it easy to run MCP servers in Kubernetes. Just define your server using a simple YAML file, and the operator handles the deployment, scaling, monitoring, and protocol validation for you.

**Key Features:**
- **Auto-detection** - Automatically detects transport type and MCP protocol version
- **Protocol Validation** - Ensures your servers are MCP-compliant
- **Horizontal Scaling** - Built-in autoscaling based on CPU and memory
- **Observability** - Prometheus metrics and Grafana dashboards out of the box
- **Production Ready** - Pod security standards and health checks

## Quick Start

> 📖 **New to MCP Operator?** Check out the [Getting Started Guide](GETTING_STARTED.md) for a complete walkthrough.

### Installation

Install the operator using kubectl:

```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

### Your First MCP Server

Create a file called `my-server.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: wikipedia
spec:
  image: "mcp/wikipedia-mcp:latest"
  args: ["--transport", "sse", "--port", "3001", "--host", "0.0.0.0"]

  transport:
    type: http
    protocol: auto  # Automatically detects the protocol
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
```

Apply it:

```sh
kubectl apply -f my-server.yaml
```

The operator needs a minute to start the server and validate it. Watch it in real-time:

```sh
kubectl get mcpservers -w
```

You should see something like:

```
NAME        PHASE     REPLICAS   READY   PROTOCOL   VALIDATION   CAPABILITIES                      AGE
wikipedia   Running   1          1       sse        Validated    ["tools","resources","prompts"]   109s
```

That's it! Your MCP server is running, validated, and ready to use.

## What Gets Created

When you create an MCPServer, the operator automatically sets up:

- **Deployment** - Manages your server pods with health checks
- **Service** - Exposes your server inside the cluster
- **HPA (optional)** - Scales pods based on traffic
- **Validation** - Checks protocol compliance and reports capabilities

## Examples

### Production Setup with Auto-Scaling

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "tzolov/mcp-everything-server:v3"
  command: ["node", "dist/index.js", "sse"]

  transport:
    type: http
    protocol: auto  # Let the operator detect the protocol
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true

  # Scale between 2-10 pods based on CPU
  hpa:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70

  # Pod security
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true

  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"
```

## Protocol Validation

The operator automatically validates your MCP servers to ensure they're working correctly. Here's what it checks:

- **Transport Detection** - Verifies the server responds on the configured endpoint
- **Protocol Version** - Detects which MCP protocol version the server uses
- **Authentication** - Identifies if the server requires auth
- **Capabilities** - Discovers what the server can do (tools, resources, prompts)

Check validation status:

```sh
kubectl get mcpserver my-server -o jsonpath='{.status.validation}' | jq
```

Example output:

```json
{
  "state": "Validated",
  "compliant": true,
  "protocol": "sse",
  "protocolVersion": "2024-11-05",
  "endpoint": "http://my-server.default.svc:3001/sse",
  "requiresAuth": false,
  "capabilities": ["tools", "resources", "prompts"],
  "lastValidated": "2025-11-29T10:30:00Z",
  "validatedGeneration": 1
}
```

### Strict Mode

Want to ensure only compliant servers run? Enable strict mode:

```yaml
spec:
  validation:
    enabled: true
    strictMode: true  # Deployment deleted if validation fails
    requiredCapabilities:
      - "tools"
      - "resources"
```

## Monitoring (Optional)

If you have Prometheus Operator installed, you can enable metrics and dashboards:

```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

<img width="3452" height="3726" alt="localhost_3000_d_mcp-operator-overview_mcp-operator-protocol-intelligence_orgId=1 from=now-15m to=now timezone=browser refresh=30s" src="https://github.com/user-attachments/assets/f81ed38e-a03d-4a3b-aa72-727487e6c2ff" />

This gives you:
- **Prometheus metrics** - Track server health, phase distribution, replica counts
- **Grafana dashboard** - Pre-built dashboard with essential metrics

**Key Metrics:**
- `mcpserver_ready_total` - Number of ready servers
- `mcpserver_phase` - Current phase (Running, Creating, Failed, etc.)
- `mcpserver_validation_compliant` - Compliance status
- `mcpserver_replicas` - Replica counts by transport type

## Transport Configuration

The operator supports both modern and legacy MCP protocols:

**Auto-detection (recommended):**
```yaml
transport:
  type: http
  protocol: auto  # Automatically chooses the best protocol
```

**Force Streamable HTTP (modern):**
```yaml
transport:
  type: http
  protocol: streamable-http
```

**Force SSE (legacy):**
```yaml
transport:
  type: http
  protocol: sse
```

Most of the time, `auto` works great and saves you from having to figure out which protocol your server uses.

## Documentation

- **[Getting Started Guide](GETTING_STARTED.md)** - 5-minute walkthrough
- **[Installation Guide](docs/installation.md)** - Detailed installation instructions
- **[API Reference](README.md#api-reference)** - Complete CRD documentation
- **[Configuration Examples](config/samples/)** - Real-world examples

## API Reference

### MCPServer Spec

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | **Required.** Container image for the MCP server |
| `replicas` | int32 | Number of desired replicas (default: 1) |
| `transport` | object | Transport configuration (defaults to HTTP auto-detection) |
| `validation` | object | Protocol validation configuration |
| `resources` | object | CPU and memory resource requirements |
| `hpa` | object | Horizontal Pod Autoscaler configuration |
| `security` | object | Pod security context settings |
| `service` | object | Service exposure configuration |
| `healthCheck` | object | Health check probe configuration |
| `environment` | []object | Environment variables |

### MCPServer Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase: Creating, Running, Scaling, Failed, ValidationFailed, Terminating |
| `replicas` | int32 | Total replica count |
| `readyReplicas` | int32 | Number of ready replicas |
| `validation` | object | Validation results with protocol, auth, and capabilities info |
| `serviceEndpoint` | string | Service endpoint URL |
| `conditions` | []object | Detailed status conditions |

See `config/samples/` for complete examples showing all available fields.

## Security

The operator automatically applies secure defaults compliant with Kubernetes [Pod Security Standards (Restricted)](https://kubernetes.io/docs/concepts/security/pod-security-standards/):

**Default Security Context:**
- `runAsNonRoot: true` - Containers must run as non-root
- `runAsUser: 1000` - Default non-root user ID
- `runAsGroup: 1000` - Default group ID
- `fsGroup: 1000` - File system group for volume permissions
- `allowPrivilegeEscalation: false` - No privilege escalation
- `capabilities: drop: ["ALL"]` - All Linux capabilities dropped
- `seccompProfile: RuntimeDefault` - Default seccomp profile

These defaults are applied automatically when `spec.security` is not specified. You only need to specify security settings if you want to override the defaults:

```yaml
spec:
  security:
    runAsUser: 2000              # Override default user
    runAsGroup: 2000             # Override default group
    fsGroup: 2000                # Override default fsGroup
    readOnlyRootFilesystem: true # Add read-only root filesystem
```

**Note:** Partial configurations are supported - unspecified fields will use the secure defaults.

## Examples and Samples

Check out the `config/samples/` directory for real-world examples:

- **`wikipedia-http.yaml`** - Simple example using the Wikipedia MCP server
- **`mcp-basic-example.yaml`** - Production setup with HPA and monitoring
- **`mcp-complete-example.yaml`** - Shows all available configuration options

Apply all samples:

```sh
kubectl apply -k config/samples/
```

## Contributing

We welcome contributions! Whether it's:
- 🐛 Bug reports
- 💡 Feature requests
- 📝 Documentation improvements
- 🔧 Code contributions

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Support

- **Found a bug?** [Open an issue](https://github.com/vitorbari/mcp-operator/issues)
- **Have questions?** [Start a discussion](https://github.com/vitorbari/mcp-operator/discussions)

## License

Copyright 2025 Vitor Bari.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
