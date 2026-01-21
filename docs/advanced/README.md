# Advanced Configuration

This section covers advanced configuration topics for production deployments.

## Configuration Guides

| Document | Description |
|----------|-------------|
| [Advanced Configuration](configuration-advanced.md) | HPA, security, affinity, and more |
| [Kustomize Patterns](kustomize.md) | Multi-environment deployments |

## Architecture & Internals

| Document | Description |
|----------|-------------|
| [Validation Behavior](validation-behavior.md) | Protocol validation and compliance |
| [Sidecar Architecture](sidecar-architecture.md) | Metrics sidecar deep-dive |
| [Containerizing MCP Servers](containerizing.md) | Build container images for MCP servers |

## Production Examples

| File | Description |
|------|-------------|
| [NetworkPolicy Example](networkpolicy-example.yaml) | Network security patterns |
| [PodDisruptionBudget Example](poddisruptionbudget-example.yaml) | High availability patterns |

## Quick Reference

### HPA Configuration

```yaml
spec:
  hpa:
    enabled: true
    minReplicas: 3
    maxReplicas: 20
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80
    scaleUpBehavior:
      stabilizationWindowSeconds: 60
      policies:
        - type: "Percent"
          value: 50
          periodSeconds: 30
```

### Security Context

```yaml
spec:
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
```

### Pod Anti-Affinity

```yaml
spec:
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

## See Also

- [Basic Configuration](../configuration.md) - Getting started with configuration
- [API Reference](../api-reference.md) - Complete field documentation
- [Monitoring](../operations/monitoring.md) - Metrics and observability
