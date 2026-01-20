# Configuration Guide

Complete guide to configuring MCP servers with patterns and examples for different use cases.

## Table of Contents

- [Overview](#overview)
- [Configuration by Environment](#configuration-by-environment)
- [Transport Configuration](#transport-configuration)
- [Resource Sizing](#resource-sizing)
- [Scaling Strategies](#scaling-strategies)
- [Security Configurations](#security-configurations)
- [Service Exposure](#service-exposure)
- [Pod Template Advanced Features](#pod-template-advanced-features)
- [Multi-Environment with Kustomize](#multi-environment-with-kustomize)
- [Best Practices](#best-practices)

## Overview

MCPServer resources can be configured for different use cases from development to production. This guide provides practical patterns and complete examples.

## Configuration by Environment

### Development Configuration

Minimal configuration optimized for local development and testing:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-dev
  namespace: development
spec:
  image: "myregistry/mcp-server:latest"

  # Single replica for development
  replicas: 1

  # Auto-detect protocol
  transport:
    type: http
    protocol: auto
    config:
      http:
        port: 8080
        path: "/mcp"

  # Minimal resources
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"

  # Debug logging
  environment:
    - name: LOG_LEVEL
      value: "debug"
    - name: ENVIRONMENT
      value: "development"

  # Validation enabled but not strict
  validation:
    enabled: true
    strictMode: false
```

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

  # Security context
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
```

### Production Configuration

Full production configuration with all safety features:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-production
  namespace: production
  labels:
    app.kubernetes.io/name: mcp-operator
    environment: production
spec:
  image: "myregistry/mcp-server:v1.2.0"

  # High availability with HPA
  replicas: 3

  hpa:
    enabled: true
    minReplicas: 3
    maxReplicas: 20
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80

    # Conservative scale-up
    scaleUpBehavior:
      stabilizationWindowSeconds: 60
      policies:
        - type: "Percent"
          value: 50
          periodSeconds: 30

    # Gradual scale-down
    scaleDownBehavior:
      stabilizationWindowSeconds: 300
      policies:
        - type: "Pods"
          value: 1
          periodSeconds: 120

  # Explicit protocol
  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 8080
        path: "/mcp"
        sessionManagement: true

  # Production resources
  resources:
    requests:
      cpu: "500m"
      memory: "512Mi"
    limits:
      cpu: "2000m"
      memory: "2Gi"

  # Production environment vars
  environment:
    - name: LOG_LEVEL
      value: "warn"
    - name: ENVIRONMENT
      value: "production"
    - name: API_KEY
      valueFrom:
        secretKeyRef:
          name: mcp-secrets
          key: api-key

  # Enable metrics collection
  metrics:
    enabled: true
    port: 9090

  # Strict validation
  validation:
    enabled: true
    strictMode: true
    requiredCapabilities:
      - "tools"
      - "resources"

  # Full security context
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    fsGroup: 1000
    readOnlyRootFilesystem: true

  # Health checks
  healthCheck:
    enabled: true
    path: "/health"
    port: 8080

  # Pod template for production
  podTemplate:
    labels:
      monitoring: enabled

    # Spread across nodes
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app.kubernetes.io/instance
                    operator: In
                    values:
                      - mcp-production
              topologyKey: kubernetes.io/hostname
```

**Production High Availability Tip:** For critical production deployments, also deploy a PodDisruptionBudget to protect against excessive disruptions during voluntary maintenance (node drains, cluster upgrades). See the [PodDisruptionBudget example](../config/samples/poddisruptionbudget-example.yaml) for configuration patterns.

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

Suitable for high-traffic production services:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-large
spec:
  image: "myregistry/mcp-server:latest"
  replicas: 5

  resources:
    requests:
      cpu: "500m"      # 0.5 CPU cores
      memory: "512Mi"  # 512 megabytes
    limits:
      cpu: "2000m"     # 2 CPU cores
      memory: "2Gi"    # 2 gigabytes

  # HPA essential for large workloads
  hpa:
    enabled: true
    minReplicas: 5
    maxReplicas: 50
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80

    scaleUpBehavior:
      stabilizationWindowSeconds: 30
      policies:
        - type: "Percent"
          value: 100
          periodSeconds: 15

    scaleDownBehavior:
      stabilizationWindowSeconds: 300
      policies:
        - type: "Pods"
          value: 2
          periodSeconds: 60
```

**Characteristics:**
- 5+ baseline replicas
- Significant resource allocation
- Can handle 100+ requests/second
- Aggressive scaling policies

## Scaling Strategies

### Static Replicas

Fixed number of replicas (simplest approach):

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-static
spec:
  image: "myregistry/mcp-server:latest"
  replicas: 3  # Always 3 replicas
```

**When to use:**
- Predictable traffic patterns
- Cost optimization with fixed capacity
- Development/staging environments

### Basic HPA

Simple autoscaling based on CPU:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-hpa-basic
spec:
  image: "myregistry/mcp-server:latest"
  replicas: 3  # Initial replicas (used as min when HPA disabled)

  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"

  hpa:
    enabled: true
    minReplicas: 3
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
```

**When to use:**
- Variable traffic patterns
- CPU-bound workloads
- Simple scaling requirements

### Advanced HPA

Complex autoscaling with custom policies:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-hpa-advanced
spec:
  image: "myregistry/mcp-server:latest"

  resources:
    requests:
      cpu: "500m"
      memory: "512Mi"
    limits:
      cpu: "2000m"
      memory: "2Gi"

  hpa:
    enabled: true
    minReplicas: 5
    maxReplicas: 50
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80

    # Fast scale-up for traffic spikes
    scaleUpBehavior:
      stabilizationWindowSeconds: 30
      policies:
        # Add up to 100% of current pods every 15s
        - type: "Percent"
          value: 100
          periodSeconds: 15
        # Or add up to 5 pods every 15s
        - type: "Pods"
          value: 5
          periodSeconds: 15

    # Gradual scale-down to prevent flapping
    scaleDownBehavior:
      stabilizationWindowSeconds: 300  # 5 minutes
      policies:
        # Remove max 10% of current pods every 60s
        - type: "Percent"
          value: 10
          periodSeconds: 60
        # Or remove max 1 pod every 60s
        - type: "Pods"
          value: 1
          periodSeconds: 60
```

**When to use:**
- Unpredictable traffic with rapid spikes
- Need to optimize for both cost and performance
- Production environments with SLAs

## Security Configurations

### Default Security (Recommended)

Use operator's secure defaults (no spec.security needed):

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-secure-default
spec:
  image: "myregistry/mcp-server:latest"
  # No security section = secure defaults applied
```

**Defaults applied:**
- `runAsNonRoot: true`
- `runAsUser: 1000`
- `runAsGroup: 1000`
- `fsGroup: 1000`
- `allowPrivilegeEscalation: false`
- Capabilities dropped: `ALL`
- Seccomp profile: `RuntimeDefault`

### Custom Security Context

Override specific security settings:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-custom-security
spec:
  image: "myregistry/mcp-server:latest"

  security:
    runAsUser: 2000               # Custom user ID
    runAsGroup: 2000              # Custom group ID
    fsGroup: 2000                 # Custom fsGroup
    runAsNonRoot: true            # Still non-root
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true  # Add read-only root
```

### Read-Only Root Filesystem

Maximum security with read-only root filesystem:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-readonly
spec:
  image: "myregistry/mcp-server:latest"

  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true

  # Provide writable directories where needed
  podTemplate:
    volumes:
      - name: tmp-volume
        emptyDir: {}
      - name: cache-volume
        emptyDir:
          sizeLimit: 1Gi

    volumeMounts:
      - name: tmp-volume
        mountPath: /tmp
      - name: cache-volume
        mountPath: /var/cache/mcp
```

### Pod Security Standards Compliance

Ensure compliance with Kubernetes Pod Security Standards:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-pss-restricted
  namespace: restricted-namespace
spec:
  image: "myregistry/mcp-server:latest"

  # Restricted Pod Security Standard requirements
  security:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true

  # No privileged containers, hostNetwork, hostPID, hostIPC
  # All capabilities dropped (handled by operator)
```

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

### Custom Port Mapping

Custom port configuration:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-custom-ports
spec:
  image: "myregistry/mcp-server:latest"

  service:
    type: ClusterIP
    port: 80          # Service port (what clients connect to)
    targetPort: 8080  # Container port (what server listens on)
    protocol: TCP
```

## Pod Template Advanced Features

### Node Selectors

Schedule pods on specific nodes:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-node-selector
spec:
  image: "myregistry/mcp-server:latest"

  podTemplate:
    nodeSelector:
      disktype: ssd           # Nodes with SSD storage
      workload-type: mcp      # Nodes designated for MCP workloads
```

### Tolerations

Allow pods on tainted nodes:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-tolerations
spec:
  image: "myregistry/mcp-server:latest"

  podTemplate:
    tolerations:
      # Tolerate dedicated nodes
      - key: "dedicated"
        operator: "Equal"
        value: "mcp-servers"
        effect: "NoSchedule"

      # Tolerate nodes with specific effects
      - key: "high-priority"
        operator: "Exists"
        effect: "PreferNoSchedule"
```

### Affinity Rules

Control pod placement with affinity:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-affinity
spec:
  image: "myregistry/mcp-server:latest"

  podTemplate:
    affinity:
      # Spread pods across nodes
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app.kubernetes.io/instance
                    operator: In
                    values:
                      - mcp-affinity
              topologyKey: kubernetes.io/hostname

      # Prefer nodes with specific characteristics
      nodeAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 50
            preference:
              matchExpressions:
                - key: node-role.kubernetes.io/worker
                  operator: Exists
```

### Custom Service Accounts

Use custom service account with specific permissions:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mcp-service-account
  namespace: production
---
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-custom-sa
  namespace: production
spec:
  image: "myregistry/mcp-server:latest"

  podTemplate:
    serviceAccountName: mcp-service-account
```

### Image Pull Secrets

Pull images from private registries:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-private-registry
spec:
  image: "private-registry.example.com/mcp-server:latest"

  podTemplate:
    imagePullSecrets:
      - name: registry-credentials
      - name: docker-hub-credentials
```

## Multi-Environment with Kustomize

Use Kustomize to manage configurations across environments.

### Directory Structure

```
kustomize/
├── base/
│   ├── kustomization.yaml
│   └── mcpserver.yaml
├── overlays/
│   ├── development/
│   │   ├── kustomization.yaml
│   │   └── patches.yaml
│   ├── staging/
│   │   ├── kustomization.yaml
│   │   └── patches.yaml
│   └── production/
│       ├── kustomization.yaml
│       └── patches.yaml
```

### Base Configuration

`base/mcpserver.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  transport:
    type: http
    protocol: auto
    config:
      http:
        port: 8080

  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"
```

`base/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - mcpserver.yaml
```

### Development Overlay

`overlays/development/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: development

bases:
  - ../../base

patchesStrategicMerge:
  - patches.yaml

commonLabels:
  environment: development
```

`overlays/development/patches.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-server
spec:
  replicas: 1

  environment:
    - name: LOG_LEVEL
      value: "debug"
    - name: ENVIRONMENT
      value: "development"
```

### Production Overlay

`overlays/production/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

bases:
  - ../../base

patchesStrategicMerge:
  - patches.yaml

commonLabels:
  environment: production
```

`overlays/production/patches.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-server
spec:
  replicas: 5

  hpa:
    enabled: true
    minReplicas: 5
    maxReplicas: 20
    targetCPUUtilizationPercentage: 70

  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    readOnlyRootFilesystem: true

  environment:
    - name: LOG_LEVEL
      value: "warn"
    - name: ENVIRONMENT
      value: "production"
```

### Deploy with Kustomize

```bash
# Development
kubectl apply -k overlays/development

# Staging
kubectl apply -k overlays/staging

# Production
kubectl apply -k overlays/production
```

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
# - etc.
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

### 5. Implement Pod Anti-Affinity

Spread replicas across nodes for high availability:

```yaml
podTemplate:
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchExpressions:
                - key: app.kubernetes.io/instance
                  operator: In
                  values: ["my-server"]
            topologyKey: kubernetes.io/hostname
```

### 6. Use PodDisruptionBudgets for High Availability

Protect your MCPServer from excessive disruptions during voluntary maintenance (node drains, upgrades):

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: my-server-pdb
  namespace: production
spec:
  # Ensure at least 1 pod is always available
  minAvailable: 1
  selector:
    matchLabels:
      app: my-server
      app.kubernetes.io/name: mcpserver
      app.kubernetes.io/component: mcp-server
```

**Key considerations:**

- **With 2 replicas:** Use `minAvailable: 1` to ensure one pod always runs
- **With 3+ replicas:** Use `maxUnavailable: 1` for controlled rolling updates
- **With HPA:** Use percentage-based limits like `minAvailable: 75%`
- **SSE servers:** Combine with `maxUnavailable: 0` in deployment strategy for graceful rollouts

See [PodDisruptionBudget examples](../config/samples/poddisruptionbudget-example.yaml) for comprehensive patterns.

### 7. Implement NetworkPolicies for Security

Control network traffic to and from your MCPServer pods to implement defense-in-depth security:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: my-server-netpol
  namespace: production
spec:
  podSelector:
    matchLabels:
      app: my-server
      app.kubernetes.io/name: mcpserver
      app.kubernetes.io/component: mcp-server

  policyTypes:
    - Ingress
    - Egress

  ingress:
    # Allow MCP client connections
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: client-apps
      ports:
        - protocol: TCP
          port: 8080  # MCP server port

    # Allow Prometheus metrics scraping
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
          podSelector:
            matchLabels:
              app.kubernetes.io/name: prometheus
      ports:
        - protocol: TCP
          port: 9090  # Metrics port

  egress:
    # Allow DNS queries
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - protocol: UDP
          port: 53

    # Allow HTTPS for external APIs
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443
```

**Key considerations:**

- **Always allow DNS:** Required for Kubernetes service discovery
- **Match your ports:** Update port numbers to match your `transport.config.http.port`
- **Prometheus integration:** If `metrics.enabled: true`, allow ingress on port 9090
- **Start permissive, then restrict:** Begin with basic policies and tighten based on traffic patterns
- **Verify CNI support:** Ensure your cluster's CNI plugin supports NetworkPolicy (Calico, Cilium, etc.)

See [NetworkPolicy examples](../config/samples/networkpolicy-example.yaml) for comprehensive patterns including namespace isolation and strict egress control.

### 8. Use Secrets for Sensitive Data

Never hardcode credentials:

```yaml
environment:
  - name: API_KEY
    valueFrom:
      secretKeyRef:
        name: mcp-secrets
        key: api-key
```

### 9. Tag Images with Versions

Avoid `latest` tag in production:

```yaml
# ✅ Good
image: "myregistry/mcp-server:v1.2.0"

# ❌ Bad for production
image: "myregistry/mcp-server:latest"
```

### 10. Monitor Your Servers

Enable metrics collection via the metrics sidecar:

```yaml
metrics:
  enabled: true
  port: 9090  # Metrics endpoint port

# Optional: customize sidecar resources
sidecar:
  resources:
    requests:
      cpu: "50m"
      memory: "64Mi"
    limits:
      cpu: "200m"
      memory: "128Mi"
```

When `metrics.enabled` is true, a sidecar container is automatically injected that:
- Proxies MCP traffic and collects protocol-specific metrics
- Exposes Prometheus metrics at the specified port
- Auto-creates a ServiceMonitor if Prometheus Operator is installed

### 11. Use Namespaces

Organize resources by environment or team:

```bash
kubectl create namespace mcp-production
kubectl create namespace mcp-staging
kubectl create namespace mcp-development
```

### 12. Document Your Configuration

Add annotations and labels:

```yaml
metadata:
  name: my-server
  annotations:
    description: "MCP server for customer support"
    owner: "platform-team"
    docs: "https://wiki.example.com/mcp-server"
  labels:
    app.kubernetes.io/name: mcp-server
    app.kubernetes.io/version: "1.2.0"
    environment: production
```

## See Also

- [API Reference](api-reference.md) - Complete field documentation
- [Environment Variables Guide](environment-variables.md) - Environment variable configuration
- [Validation Behavior](validation-behavior.md) - Protocol validation details
- [Troubleshooting Guide](troubleshooting.md) - Common issues and solutions
- [Monitoring Guide](monitoring.md) - Metrics and observability
- [Configuration Examples](../config/samples/) - Real-world YAML examples
