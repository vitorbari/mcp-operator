# MCP Operator

[![Lint](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/lint.yml)
[![Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test.yml)
[![E2E Tests](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/test-e2e.yml)
[![Release](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml/badge.svg)](https://github.com/vitorbari/mcp-operator/actions/workflows/release.yml)
[![Awesome MCP](https://awesome.re/mentioned-badge.svg)](https://github.com/punkpeye/awesome-mcp-devtools)

> **⚠️ Alpha Software - Not Production Ready**
>
> This project is in early development and should be considered **experimental**. While we encourage you to try it out and provide feedback, please don't use it in production environments yet. APIs may change, features may be incomplete, and bugs are expected.
>
> **We'd love your feedback!** Please open issues for bugs, feature requests, or questions.

Deploy your MCP servers on Kubernetes with automatic protocol validation, horizontal scaling, and built-in observability.

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

### Installation Options

Choose your preferred installation method:

#### Option 1: Install via Helm (Recommended)

```sh
# See https://github.com/vitorbari/mcp-operator/releases for latest version
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --namespace mcp-operator-system \
  --create-namespace
```

#### Option 2: Install via kubectl

```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

**Use Helm** for easier configuration and upgrades. **Use kubectl** for minimal dependencies.

### Your First MCP Server

Create a file called `my-server.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: customer-data-mcp
spec:
  image: "your-registry.company.com/customer-data-mcp:v1.2.0"
  # Operator handles validation, scaling, monitoring
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
NAME              PHASE     REPLICAS   READY   PROTOCOL   VALIDATION   CAPABILITIES                      AGE
customer-data-mcp Running   1          1       sse        Validated    ["tools","resources","prompts"]   109s
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

The operator automatically validates your MCP servers to ensure they're protocol-compliant. It checks transport connectivity, protocol version, authentication requirements, and available capabilities.

Enable strict mode to fail deployments that don't pass validation:

```yaml
spec:
  validation:
    strictMode: true
```

For detailed validation behavior, see the [Validation Behavior Guide](docs/validation-behavior.md).

## Monitoring

Enable Prometheus metrics and Grafana dashboards:

**With Helm:**
```sh
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --namespace mcp-operator-system \
  --create-namespace \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

**With kubectl:**
```sh
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

<img width="3452" height="3726" alt="localhost_3000_d_mcp-operator-overview_mcp-operator-protocol-intelligence_orgId=1 from=now-15m to=now timezone=browser refresh=30s" src="https://github.com/user-attachments/assets/f81ed38e-a03d-4a3b-aa72-727487e6c2ff" />

Requires [Prometheus Operator](https://prometheus-operator.dev/). See the [Monitoring Guide](docs/monitoring.md) for details on available metrics, dashboards, and alerting.

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

### Getting Started
- [Getting Started Guide](GETTING_STARTED.md) - 5-minute walkthrough
- [Installation Guide](docs/installation.md) - Detailed installation options

### Configuration
- [Configuration Guide](docs/configuration-guide.md) - Complete configuration patterns and examples
- [Environment Variables](docs/environment-variables.md) - Configuring environment variables
- [Configuration Examples](config/samples/) - Real-world YAML examples

### Reference
- [API Reference](docs/api-reference.md) - Complete CRD field documentation
- [Validation Behavior](docs/validation-behavior.md) - Protocol validation deep dive

### Operations
- [Troubleshooting Guide](docs/troubleshooting.md) - Common issues and solutions
- [Monitoring Guide](docs/monitoring.md) - Metrics, dashboards, and alerts

### Advanced Topics
- [Release Process](docs/release-process.md) - For maintainers
- [Contributing](CONTRIBUTING.md) - Development and contribution guidelines


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
