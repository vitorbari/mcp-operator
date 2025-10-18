# Installation Guide

## Overview

MCP Operator has a modular installation approach with optional monitoring capabilities.

## Core Installation

The core operator provides all essential functionality:
- MCPServer CRD (Custom Resource Definition)
- Controller deployment and RBAC
- Metrics endpoint (standard `/metrics` endpoint on port 8443)

Install the core operator:

```bash
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
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
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
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

## Installation Options

### Option 1: Core Only (No Monitoring)

Perfect for:
- Development environments
- Clusters without Prometheus Operator
- Minimal installations

```bash
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

### Option 2: Core + Monitoring

Perfect for:
- Production environments with monitoring infrastructure
- Clusters with Prometheus Operator installed

```bash
# Install core
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml

# Install monitoring
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
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

### Remove Monitoring (Optional)

If you installed monitoring resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

### Remove Core Operator

```bash
# Delete all MCPServer resources first
kubectl delete mcpserver --all --all-namespaces

# Uninstall the operator
kubectl delete -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
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
