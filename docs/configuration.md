# Configuration Guide

Basic configuration patterns for MCP servers.

## Table of Contents

- [Overview](#overview)
- [Configuration by Environment](#configuration-by-environment)
- [Transport Configuration](#transport-configuration)
- [Resource Sizing](#resource-sizing)
- [Service Exposure](#service-exposure)
- [Best Practices](#best-practices)

## Overview

MCPServer resources can be configured for different use cases from development to production. This guide provides practical patterns for common scenarios.

For advanced configuration (HPA, security contexts, affinity, Kustomize), see the [Advanced Configuration Guide](advanced/configuration-advanced.md).

## Configuration by Environment

### Development Configuration

See [Quick Start](quick-start.md) for minimal development configuration.

### Staging Configuration

Configuration for pre-production testing:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-staging
  namespace: staging
spec:
  image: "myregistry/mcp-server:v1.2.0"

  # Multiple replicas for testing HA
  replicas: 2

  # Explicit protocol for consistency
  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 8080
        path: "/mcp"
        sessionManagement: true

  # Production-like resources
  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"

  # Info logging
  environment:
    - name: LOG_LEVEL
      value: "info"
    - name: ENVIRONMENT
      value: "staging"

  # Enable metrics collection
  metrics:
    enabled: true
    port: 9090

  # Strict validation in staging
  validation:
    enabled: true
    strictMode: true
    requiredCapabilities:
      - "tools"
      - "resources"
```

### Production Configuration

For full production configuration with HPA, security contexts, and pod affinity, see the [Advanced Configuration Guide](advanced/configuration-advanced.md#production-configuration).

## Transport Configuration

### Auto-Detection (Recommended)

Let the operator detect the protocol automatically:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-auto
spec:
  image: "mcp/server:latest"

  transport:
    type: http
    protocol: auto  # Prefers Streamable HTTP over SSE
    config:
      http:
        port: 8080
        # Path will be auto-detected
        # Tries /mcp first, then /sse
```

### Explicit Streamable HTTP (Modern)

Force Streamable HTTP protocol (MCP 2025-03-26+):

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-streamable
spec:
  image: "mcp/modern-server:latest"

  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 8080
        path: "/mcp"
        sessionManagement: true
```

### Explicit SSE (Legacy)

Force Server-Sent Events protocol (MCP 2024-11-05):

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-sse
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

For detailed transport information, see [Transport Protocols](transports/README.md).

### When to Use Which Protocol

**Use `auto` when:**
- You're not sure which protocol your server supports
- Your server supports multiple protocols
- You want the operator to handle protocol selection

**Use `streamable-http` when:**
- You know your server uses modern Streamable HTTP
- You want to ensure only Streamable HTTP is used
- You're deploying new MCP servers (recommended)

**Use `sse` when:**
- You're working with legacy MCP servers
- Your server only supports SSE
- You need compatibility with older MCP clients

## Resource Sizing

### Small Workloads

Suitable for development, testing, or low-traffic services:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-small
spec:
  image: "myregistry/mcp-server:latest"
  replicas: 1

  resources:
    requests:
      cpu: "100m"      # 0.1 CPU cores
      memory: "128Mi"  # 128 megabytes
    limits:
      cpu: "500m"      # 0.5 CPU cores
      memory: "512Mi"  # 512 megabytes
```

**Characteristics:**
- Single replica or 2 for basic HA
- Minimal resource usage
- Suitable for <10 requests/second

### Medium Workloads

Suitable for production services with moderate traffic:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-medium
spec:
  image: "myregistry/mcp-server:latest"
  replicas: 3

  resources:
    requests:
      cpu: "200m"      # 0.2 CPU cores
      memory: "256Mi"  # 256 megabytes
    limits:
      cpu: "1000m"     # 1 CPU core
      memory: "1Gi"    # 1 gigabyte

  # Optional: Enable HPA
  hpa:
    enabled: true
    minReplicas: 3
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
```

**Characteristics:**
- 3+ replicas for high availability
- Moderate resource allocation
- Can handle 10-100 requests/second
- HPA recommended for traffic spikes

### Large Workloads

See [Advanced Configuration Guide](advanced/configuration-advanced.md#large-workloads) for large-scale deployments with HPA.

## Service Exposure

### ClusterIP (Default)

Internal cluster access only:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-clusterip
spec:
  image: "myregistry/mcp-server:latest"

  service:
    type: ClusterIP  # Default
    port: 8080
```

**Use case:** Services accessed only from within the cluster.

### NodePort

Access via node IP and static port:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-nodeport
spec:
  image: "myregistry/mcp-server:latest"

  service:
    type: NodePort
    port: 8080
    # Kubernetes assigns a port in range 30000-32767
```

**Use case:** Development/testing with direct node access.

### LoadBalancer

Cloud load balancer (AWS/GCP/Azure):

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-loadbalancer
spec:
  image: "myregistry/mcp-server:latest"

  service:
    type: LoadBalancer
    port: 8080
    annotations:
      # AWS
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      service.beta.kubernetes.io/aws-load-balancer-backend-protocol: "http"

      # GCP
      cloud.google.com/load-balancer-type: "Internal"

      # Azure
      service.beta.kubernetes.io/azure-load-balancer-internal: "true"
```

**Use case:** Production external access with cloud provider integration.

## Best Practices

### 1. Start Simple, Scale as Needed

Begin with minimal configuration and add complexity as requirements grow:

```yaml
# Start with this
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-server
spec:
  image: "myregistry/mcp-server:latest"

# Add features gradually:
# - Resource limits
# - HPA
# - Security contexts
# - Pod affinity
```

### 2. Always Set Resource Limits

Prevent resource exhaustion:

```yaml
resources:
  requests:  # Guaranteed resources
    cpu: "200m"
    memory: "256Mi"
  limits:    # Maximum resources
    cpu: "1000m"
    memory: "1Gi"
```

### 3. Enable Health Checks

Ensure Kubernetes can monitor server health:

```yaml
healthCheck:
  enabled: true
  path: "/health"
  port: 8080
```

### 4. Use Strict Validation in Production

Catch issues early:

```yaml
validation:
  enabled: true
  strictMode: true  # Fail deployment if validation fails
  requiredCapabilities:
    - "tools"
    - "resources"
```

### 5. Use Secrets for Sensitive Data

Never hardcode credentials:

```yaml
environment:
  - name: API_KEY
    valueFrom:
      secretKeyRef:
        name: mcp-secrets
        key: api-key
```

### 6. Tag Images with Versions

Avoid `latest` tag in production:

```yaml
# Good
image: "myregistry/mcp-server:v1.2.0"

# Bad for production
image: "myregistry/mcp-server:latest"
```

### 7. Monitor Your Servers

Enable metrics collection via the metrics sidecar:

```yaml
metrics:
  enabled: true
  port: 9090  # Metrics endpoint port
```

See [Monitoring Guide](operations/monitoring.md) for details.

## See Also

- [Advanced Configuration](advanced/configuration-advanced.md) - HPA, security, affinity, Kustomize
- [API Reference](api-reference.md) - Complete field documentation
- [Environment Variables Guide](environment-variables.md) - Environment variable configuration
- [Transport Protocols](transports/README.md) - Protocol details
- [Troubleshooting Guide](operations/troubleshooting.md) - Common issues and solutions
- [Configuration Examples](../config/samples/) - Real-world YAML examples
