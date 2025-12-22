# Installation Guide

## Overview

MCP Operator has a modular installation approach with optional monitoring capabilities. You can install using Helm (recommended for production) or kubectl (for minimal dependencies).

## Helm Installation (Recommended)

Helm provides the easiest way to install, configure, and upgrade the operator.

### Prerequisites

- Kubernetes 1.24+
- Helm 3.8+

**Install Helm (if needed):**
```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
helm version
```

### Basic Installation

Install the operator from GitHub Container Registry:

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Install
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace
```

This creates the `mcp-operator-system` namespace and installs the chart there.

**Verify installation:**

```bash
# Check Helm release
helm list -n mcp-operator-system

# Check operator pods
kubectl get pods -n mcp-operator-system

# Check CRD is installed
kubectl get crd mcpservers.mcp.mcp-operator.io
```

### Custom Configuration

#### Enable Prometheus Monitoring

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Install with monitoring
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

#### Custom Resource Limits

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Install with custom resources
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace \
  --set controllerManager.container.resources.limits.cpu=1000m \
  --set controllerManager.container.resources.limits.memory=512Mi
```

#### Using a Values File

Create a `values.yaml` file:

```yaml
controllerManager:
  replicas: 2
  container:
    image:
      repository: ghcr.io/vitorbari/mcp-operator
      tag: 0.1.0-alpha.12
    resources:
      limits:
        cpu: 1000m
        memory: 512Mi
      requests:
        cpu: 100m
        memory: 128Mi

prometheus:
  enable: true
  additionalLabels:
    release: monitoring

grafana:
  enabled: true
```

Install with your values:

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Install with values file
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --create-namespace \
  -f values.yaml
```

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controllerManager.replicas` | Number of operator replicas | `1` |
| `controllerManager.container.image.repository` | Container image repository | `ghcr.io/vitorbari/mcp-operator` |
| `controllerManager.container.image.tag` | Container image tag | `0.1.0-alpha.12` |
| `controllerManager.container.resources` | Resource limits and requests | See values.yaml |
| `prometheus.enable` | Enable ServiceMonitor for Prometheus | `true` |
| `prometheus.additionalLabels` | Additional labels for ServiceMonitor | `release: monitoring` |
| `metrics.enable` | Enable metrics endpoint | `true` |
| `grafana.enabled` | Create Grafana dashboard ConfigMap | `true` |
| `crd.enable` | Install CRDs | `true` |
| `crd.keep` | Keep CRDs on uninstall | `true` |
| `rbac.enable` | Create RBAC resources | `true` |

For all available options, see `dist/chart/values.yaml` in the repository.

### Upgrading

Upgrade to a new version:

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Upgrade
helm upgrade mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --reuse-values
```

Upgrade with new configuration:

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name' | sed 's/^v//')

# Upgrade with new config
helm upgrade mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --version ${VERSION} \
  --namespace mcp-operator-system \
  --reuse-values \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

### Uninstalling

Remove the operator:

```bash
helm uninstall mcp-operator --namespace mcp-operator-system
```

**Note:** By default, CRDs are kept even after uninstalling (controlled by `crd.keep: true`). To also remove CRDs:

```bash
kubectl delete crd mcpservers.mcp.mcp-operator.io
```

## kubectl Installation

### Core Installation

The core operator provides all essential functionality:
- MCPServer CRD (Custom Resource Definition)
- Controller deployment and RBAC
- Metrics endpoint (standard `/metrics` endpoint on port 8443)

Install the core operator:

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name')

# Install
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/${VERSION}/dist/install.yaml
```

This will:
1. Create the `mcp-operator-system` namespace
2. Install the MCPServer CRD
3. Create necessary RBAC (ClusterRole, ClusterRoleBinding, ServiceAccount)
4. Deploy the controller manager
5. Create a metrics service

**Verify installation:**

```bash
# Check operator pods
kubectl get pods -n mcp-operator-system

# Check CRD is installed
kubectl get crd mcpservers.mcp.mcp-operator.io

# Check you can create MCPServer resources
kubectl auth can-i create mcpserver
```

## Optional: Monitoring

Monitoring resources provide Prometheus metrics collection and Grafana dashboards.

### Prerequisites

Monitoring requires [Prometheus Operator](https://prometheus-operator.dev/) to be installed in your cluster.

**Check if Prometheus Operator is installed:**

```bash
kubectl get crd servicemonitors.monitoring.coreos.com
```

If the CRD exists, you have Prometheus Operator installed.

**Install Prometheus Operator (if needed):**

```bash
# Install Prometheus Operator
kubectl apply --server-side -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.79.2/bundle.yaml

# Wait for it to be ready
kubectl wait --for=condition=available --timeout=300s \
  deployment/prometheus-operator \
  -n default
```

### Install Monitoring Resources

Once Prometheus Operator is installed:

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name')

# Install monitoring
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/${VERSION}/dist/monitoring.yaml
```

This creates:
- **ServiceMonitor**: Configures Prometheus to scrape operator metrics
- **Grafana Dashboard**: Pre-configured ConfigMap for Grafana

**Verify monitoring installation:**

```bash
# Check ServiceMonitor
kubectl get servicemonitor -n mcp-operator-system

# Check Grafana Dashboard ConfigMap
kubectl get configmap mcp-operator-grafana-dashboard -n mcp-operator-system
```

## Comparison: Helm vs kubectl

| Feature | Helm | kubectl |
|---------|------|---------|
| **Ease of Installation** | ✅ Single command | ✅ Single command |
| **Configuration** | ✅ Values file or --set flags | ❌ Must edit YAML |
| **Upgrades** | ✅ `helm upgrade` with version management | ⚠️ Manual kubectl apply |
| **Rollback** | ✅ `helm rollback` to previous version | ❌ Manual process |
| **Monitoring Setup** | ✅ Enable with `--set prometheus.enable=true` | ⚠️ Separate manifest |
| **Custom Resources** | ✅ Override any value | ⚠️ Edit manifests |
| **Dependencies** | Requires Helm CLI | ✅ Only kubectl |
| **Best For** | Production, teams, customization | CI/CD, minimal deps |

**Recommendation:** Use Helm for production environments. Use kubectl for quick testing or CI/CD pipelines with minimal dependencies.

## Installation Options Summary

### Option 1: Helm (Recommended)

**Best for:** Production, development, any environment needing customization

```bash
# Basic installation (latest version)
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --namespace mcp-operator-system \
  --create-namespace

# With monitoring
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --namespace mcp-operator-system \
  --create-namespace \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

### Option 2: kubectl - Core Only

**Best for:** Minimal installations, CI/CD, quick testing

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name')

# Install
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/${VERSION}/dist/install.yaml
```

### Option 3: kubectl - Core + Monitoring

**Best for:** Production with existing Prometheus Operator

```bash
# Get latest version
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name')

# Install core
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/${VERSION}/dist/install.yaml

# Install monitoring
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/${VERSION}/dist/monitoring.yaml
```

## Namespace Strategy

The operator follows Kubernetes best practices for namespace organization:

### Operator Namespace

The operator runs in its own dedicated namespace:

```yaml
namespace: mcp-operator-system
```

This namespace contains:
- Controller manager deployment
- Operator ServiceAccount and RBAC
- Metrics service
- Monitoring resources (ServiceMonitor, Grafana dashboard)

### User Namespaces

MCPServer custom resources should be created in **user namespaces**, not in the operator namespace:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-server
  namespace: my-application  # ← Your namespace
spec:
  image: my-mcp-server:latest
```

The operator watches all namespaces and manages MCPServer resources wherever they are created.

## Uninstallation

### Using Helm

If you installed with Helm:

```bash
# Delete all MCPServer resources first
kubectl delete mcpserver --all --all-namespaces

# Uninstall the operator
helm uninstall mcp-operator --namespace mcp-operator-system

# Optionally remove CRDs (they're kept by default)
kubectl delete crd mcpservers.mcp.mcp-operator.io
```

### Using kubectl

If you installed with kubectl:

```bash
# Delete all MCPServer resources first
kubectl delete mcpserver --all --all-namespaces

# Get the version you installed
VERSION=$(curl -s https://api.github.com/repos/vitorbari/mcp-operator/releases | jq -r '.[0].tag_name')

# Remove monitoring (if installed)
kubectl delete -f https://raw.githubusercontent.com/vitorbari/mcp-operator/${VERSION}/dist/monitoring.yaml

# Uninstall the operator
kubectl delete -f https://raw.githubusercontent.com/vitorbari/mcp-operator/${VERSION}/dist/install.yaml
```

## Building from Source

If you want to build and deploy from source:

```bash
# Clone the repository
git clone https://github.com/vitorbari/mcp-operator.git
cd mcp-operator

# Build installation manifests
make build-installer-all IMG=my-registry/mcp-operator:dev

# Install
kubectl apply -f dist/install.yaml
kubectl apply -f dist/monitoring.yaml  # Optional
```

## Troubleshooting

### Monitoring Installation Fails

**Error:** `no matches for kind "ServiceMonitor"`

**Solution:** Prometheus Operator is not installed. Either:
- Install Prometheus Operator first (see Prerequisites above)
- Skip the monitoring installation (operator works fine without it)

### Operator Pods Not Starting

Check pod status and logs:

```bash
kubectl get pods -n mcp-operator-system
kubectl logs -n mcp-operator-system deployment/mcp-operator-controller-manager
kubectl describe pod -n mcp-operator-system -l control-plane=controller-manager
```

### CRD Already Exists

If you get "CRD already exists" errors, you may have a previous installation:

```bash
# Check existing CRDs
kubectl get crd | grep mcp

# Delete old CRD if needed
kubectl delete crd mcpservers.mcp.mcp-operator.io
```

## Next Steps

- [Getting Started Guide](../GETTING_STARTED.md) - Deploy your first MCPServer
- [Examples](../config/samples/) - Sample MCPServer configurations
- [Release Process](release-process.md) - How to create releases
