# Advanced Configuration

Advanced configuration patterns for production MCP server deployments.

## Table of Contents

- [Production Configuration](#production-configuration)
- [Scaling Strategies](#scaling-strategies)
- [Security Configurations](#security-configurations)
- [Pod Template Advanced Features](#pod-template-advanced-features)
- [Large Workloads](#large-workloads)

## Production Configuration

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

**Production High Availability Tip:** For critical production deployments, also deploy a PodDisruptionBudget to protect against excessive disruptions during voluntary maintenance (node drains, cluster upgrades). See [PodDisruptionBudget examples](poddisruptionbudget-example.yaml) for configuration patterns.

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

## Large Workloads

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

## NetworkPolicy for Security

Control network traffic to and from your MCPServer pods. See [NetworkPolicy examples](networkpolicy-example.yaml) for patterns including:

- Basic ingress/egress rules
- Namespace-scoped access
- Strict egress control
- Multi-port configurations

## PodDisruptionBudget for Availability

Protect your MCPServer from excessive disruptions. See [PodDisruptionBudget examples](poddisruptionbudget-example.yaml) for patterns:

- `minAvailable` for critical services
- `maxUnavailable` for larger deployments
- Percentage-based limits for HPA
- SSE-specific configurations

## See Also

- [Basic Configuration](../configuration.md) - Getting started
- [Kustomize Patterns](kustomize.md) - Multi-environment deployments
- [API Reference](../api-reference.md) - Complete field documentation
- [Monitoring](../operations/monitoring.md) - Metrics and observability
