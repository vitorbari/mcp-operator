# Multi-Environment with Kustomize

Use Kustomize to manage MCPServer configurations across multiple environments.

## Directory Structure

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

## Base Configuration

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

## Development Overlay

`overlays/development/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: development

resources:
  - ../../base

patches:
  - path: patches.yaml

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

## Staging Overlay

`overlays/staging/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: staging

resources:
  - ../../base

patches:
  - path: patches.yaml

commonLabels:
  environment: staging
```

`overlays/staging/patches.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-server
spec:
  replicas: 2

  validation:
    enabled: true
    strictMode: true

  environment:
    - name: LOG_LEVEL
      value: "info"
    - name: ENVIRONMENT
      value: "staging"
```

## Production Overlay

`overlays/production/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

resources:
  - ../../base

patches:
  - path: patches.yaml

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

## Deploy with Kustomize

```bash
# Development
kubectl apply -k overlays/development

# Staging
kubectl apply -k overlays/staging

# Production
kubectl apply -k overlays/production
```

## Preview Changes

Preview what will be applied without deploying:

```bash
# See the rendered manifests
kubectl kustomize overlays/production

# Diff against current state
kubectl diff -k overlays/production
```

## Advanced Patterns

### Image Tag Overrides

Override the image tag per environment:

`overlays/production/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

resources:
  - ../../base

images:
  - name: myregistry/mcp-server
    newTag: v1.2.0  # Pin to specific version

patches:
  - path: patches.yaml

commonLabels:
  environment: production
```

### ConfigMap and Secret Generators

Generate ConfigMaps from files:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

resources:
  - ../../base

configMapGenerator:
  - name: mcp-config
    files:
      - config.yaml

secretGenerator:
  - name: mcp-secrets
    envs:
      - secrets.env  # Contains KEY=value pairs
```

### Strategic Merge Patches

Use strategic merge patches for complex changes:

`overlays/production/security-patch.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: mcp-server
spec:
  podTemplate:
    securityContext:
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    volumes:
      - name: tmp-volume
        emptyDir: {}
    volumeMounts:
      - name: tmp-volume
        mountPath: /tmp
```

`overlays/production/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

resources:
  - ../../base

patches:
  - path: patches.yaml
  - path: security-patch.yaml
```

### JSON Patches

For precise modifications:

`overlays/production/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

resources:
  - ../../base

patches:
  - target:
      kind: MCPServer
      name: mcp-server
    patch: |-
      - op: replace
        path: /spec/replicas
        value: 5
      - op: add
        path: /spec/hpa
        value:
          enabled: true
          minReplicas: 5
          maxReplicas: 20
```

## See Also

- [Basic Configuration](../configuration.md) - Getting started
- [Advanced Configuration](configuration-advanced.md) - HPA, security, affinity
- [Kustomize Documentation](https://kustomize.io/)
