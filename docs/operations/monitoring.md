# Monitoring Guide

Complete guide to monitoring MCP servers with Prometheus metrics and Grafana dashboards.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Available Metrics](#available-metrics)
- [Grafana Dashboard](#grafana-dashboard)
- [Recommended Alerts](#recommended-alerts)
- [Query Examples](#query-examples)
- [Troubleshooting](#troubleshooting)

## Overview

The MCP Operator exposes Prometheus metrics for monitoring server health, performance, and protocol validation. An optional Grafana dashboard provides visualization of these metrics.

**Key Features:**
- Real-time server health tracking
- Phase distribution monitoring
- Validation compliance tracking
- Replica count monitoring by transport type
- Reconciliation performance metrics

## Prerequisites

Before installing monitoring, you need:

### 1. Prometheus Operator

The monitoring stack requires [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) to be installed in your cluster.

**Check if Prometheus Operator is installed:**

```bash
kubectl get crd prometheuses.monitoring.coreos.com
kubectl get crd servicemonitors.monitoring.coreos.com
```

If these CRDs exist, you have Prometheus Operator installed.

**Install Prometheus Operator (if needed):**

Using Helm:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace
```

Or using kube-prometheus:

```bash
git clone https://github.com/prometheus-operator/kube-prometheus.git
cd kube-prometheus
kubectl apply --server-side -f manifests/setup
kubectl wait --for condition=Established --all CustomResourceDefinition --namespace=monitoring
kubectl apply -f manifests/
```

### 2. Grafana (Optional)

If you installed Prometheus Operator with kube-prometheus-stack, Grafana is included. Otherwise, install it:

```bash
helm install grafana grafana/grafana \
  --namespace monitoring \
  --set adminPassword=admin
```

## Installation

Once Prometheus Operator is installed, deploy the monitoring manifests:

```bash
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

This creates:
- **ServiceMonitor** - Tells Prometheus to scrape operator metrics
- **Grafana Dashboard** - Pre-built dashboard ConfigMap

### Verify Installation

Check that the ServiceMonitor was created:

```bash
kubectl get servicemonitor -n mcp-operator-system
```

Expected output:

```
NAME                              AGE
mcp-operator-controller-manager   30s
```

Verify Prometheus is scraping metrics:

```bash
# Port forward to Prometheus
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090

# Open http://localhost:9090 in browser
# Go to Status → Targets
# Look for "mcp-operator-system/mcp-operator-controller-manager"
```

## Available Metrics

### mcpserver_ready_total

**Type:** Gauge

**Description:** Number of ready MCPServer instances.

**Labels:**
- `name` - MCPServer name
- `namespace` - MCPServer namespace

**Example:**

```promql
mcpserver_ready_total{name="my-mcp-server",namespace="default"}
```

**Usage:** Track which servers are ready and serving traffic.

### mcpserver_phase

**Type:** Gauge

**Description:** Current phase of MCPServer instances (1 = in this phase, 0 = not in this phase).

**Labels:**
- `name` - MCPServer name
- `namespace` - MCPServer namespace
- `phase` - Phase name (Creating, Running, Scaling, Failed, ValidationFailed, Terminating)

**Example:**

```promql
mcpserver_phase{name="my-mcp-server",namespace="default",phase="Running"}
```

**Usage:** Monitor server lifecycle phases and detect issues.

### mcpserver_validation_compliant

**Type:** Gauge

**Description:** Whether MCPServer passed validation (1 = compliant, 0 = not compliant).

**Labels:**
- `name` - MCPServer name
- `namespace` - MCPServer namespace

**Example:**

```promql
mcpserver_validation_compliant{name="my-mcp-server",namespace="default"}
```

**Usage:** Track protocol compliance across servers.

### mcpserver_replicas

**Type:** Gauge

**Description:** Number of replicas for MCPServer instances.

**Labels:**
- `name` - MCPServer name
- `namespace` - MCPServer namespace
- `transport_type` - Transport type (http, custom)
- `type` - Replica type (desired, current, ready, available)

**Example:**

```promql
mcpserver_replicas{name="my-mcp-server",namespace="default",transport_type="http",type="ready"}
```

**Usage:** Monitor replica counts and scaling behavior.

### mcpserver_reconcile_duration_seconds

**Type:** Histogram

**Description:** Time taken to reconcile MCPServer resources.

**Labels:**
- `name` - MCPServer name
- `namespace` - MCPServer namespace

**Buckets:** 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30

**Example:**

```promql
histogram_quantile(0.95, rate(mcpserver_reconcile_duration_seconds_bucket[5m]))
```

**Usage:** Track operator performance and identify slow reconciliation loops.

## Grafana Dashboard

### Accessing the Dashboard

If you used kube-prometheus-stack, access Grafana:

```bash
# Get Grafana admin password
kubectl get secret -n monitoring prometheus-grafana -o jsonpath="{.data.admin-password}" | base64 --decode

# Port forward to Grafana
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
```

Open http://localhost:3000 in your browser and log in with:
- Username: `admin`
- Password: (from command above)

### Installing the Dashboard

The dashboard is automatically created as a ConfigMap. Import it into Grafana:

**Method 1: Automatic (if using Grafana sidecar)**

The dashboard should appear automatically under "Dashboards → Browse → MCP Operator".

**Method 2: Manual Import**

1. Extract the dashboard JSON:
   ```bash
   kubectl get configmap mcp-operator-dashboard -n mcp-operator-system -o jsonpath='{.data.mcp-operator-dashboard\.json}' > dashboard.json
   ```

2. In Grafana:
   - Click "+" → "Import"
   - Upload `dashboard.json`
   - Select Prometheus data source
   - Click "Import"

### Dashboard Panels

The dashboard includes:

#### 1. Overview

- **Total MCPServers** - Count of all MCPServer resources
- **Ready Servers** - Servers currently ready
- **Validation Compliance Rate** - Percentage of compliant servers
- **Average Replicas** - Average replica count across servers

#### 2. Server Status

- **MCPServer Phase Distribution** - Pie chart showing phase breakdown
- **Server Ready Status** - Table of all servers with ready status
- **Validation State** - Validation status by server

#### 3. Protocol Intelligence

- **Transport Type Distribution** - Breakdown by transport type (HTTP, custom)
- **Protocol Version Distribution** - MCP protocol versions in use
- **Validation Compliance Over Time** - Time series of compliance rate

#### 4. Performance

- **Reconciliation Duration (p95)** - 95th percentile reconciliation time
- **Reconciliation Rate** - Reconciliations per second
- **Slow Reconciliations** - Servers with slow reconciliation loops

#### 5. Scaling

- **Replica Count by Server** - Current vs desired replicas
- **Replica Changes Over Time** - Scaling activity
- **HPA Status** - Autoscaling behavior

## Recommended Alerts

Configure Prometheus alerts to catch issues early.

### Creating Alert Rules

Create a PrometheusRule resource:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: mcp-operator-alerts
  namespace: mcp-operator-system
  labels:
    prometheus: kube-prometheus
spec:
  groups:
    - name: mcp-operator
      interval: 30s
      rules:
        # MCPServer not ready
        - alert: MCPServerNotReady
          expr: mcpserver_ready_total == 0
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "MCPServer {{ $labels.name }} not ready"
            description: "MCPServer {{ $labels.name }} in namespace {{ $labels.namespace }} has been not ready for 5 minutes."

        # MCPServer in Failed phase
        - alert: MCPServerFailed
          expr: mcpserver_phase{phase="Failed"} == 1
          for: 2m
          labels:
            severity: critical
          annotations:
            summary: "MCPServer {{ $labels.name }} in Failed state"
            description: "MCPServer {{ $labels.name }} in namespace {{ $labels.namespace }} is in Failed phase."

        # Validation failed
        - alert: MCPServerValidationFailed
          expr: mcpserver_validation_compliant == 0
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "MCPServer {{ $labels.name }} validation failed"
            description: "MCPServer {{ $labels.name }} in namespace {{ $labels.namespace }} is not MCP compliant."

        # High reconciliation duration
        - alert: MCPServerSlowReconciliation
          expr: histogram_quantile(0.95, rate(mcpserver_reconcile_duration_seconds_bucket[5m])) > 10
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "Slow reconciliation for {{ $labels.name }}"
            description: "MCPServer {{ $labels.name }} reconciliation is taking over 10 seconds (p95)."

        # Replica count mismatch
        - alert: MCPServerReplicaMismatch
          expr: |
            (mcpserver_replicas{type="desired"} - mcpserver_replicas{type="ready"}) > 0
          for: 15m
          labels:
            severity: warning
          annotations:
            summary: "Replica mismatch for {{ $labels.name }}"
            description: "MCPServer {{ $labels.name }} has {{ $value }} fewer ready replicas than desired for 15 minutes."

        # No ready replicas
        - alert: MCPServerNoReplicas
          expr: mcpserver_replicas{type="ready"} == 0
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: "No ready replicas for {{ $labels.name }}"
            description: "MCPServer {{ $labels.name }} has no ready replicas."
```

Apply the alert rules:

```bash
kubectl apply -f alerts.yaml
```

### Viewing Alerts

In Prometheus UI (http://localhost:9090):
- Go to "Alerts" to see configured alerts
- Active alerts will show current status

In Grafana:
- Go to "Alerting → Alert rules"
- Configure notification channels (email, Slack, PagerDuty, etc.)

## Query Examples

### Basic Queries

**List all MCPServers and their ready status:**

```promql
mcpserver_ready_total
```

**Count servers by phase:**

```promql
count by (phase) (mcpserver_phase == 1)
```

**Total ready replicas across all servers:**

```promql
sum(mcpserver_replicas{type="ready"})
```

### Advanced Queries

**Servers with replica count mismatch:**

```promql
(
  mcpserver_replicas{type="desired"}
  - on(name, namespace) group_left()
  mcpserver_replicas{type="ready"}
) > 0
```

**Average reconciliation duration by server:**

```promql
rate(mcpserver_reconcile_duration_seconds_sum[5m])
/ rate(mcpserver_reconcile_duration_seconds_count[5m])
```

**Validation compliance rate (percentage):**

```promql
100 * sum(mcpserver_validation_compliant) / count(mcpserver_validation_compliant)
```

**Servers with most replica changes (churn):**

```promql
delta(mcpserver_replicas{type="ready"}[1h])
```

**Transport type distribution:**

```promql
count by (transport_type) (mcpserver_replicas{type="current"})
```

### Performance Queries

**95th percentile reconciliation time:**

```promql
histogram_quantile(0.95, rate(mcpserver_reconcile_duration_seconds_bucket[5m]))
```

**Reconciliation rate (per second):**

```promql
rate(mcpserver_reconcile_duration_seconds_count[5m])
```

**Slowest servers (p99 reconciliation time):**

```promql
topk(5,
  histogram_quantile(0.99,
    rate(mcpserver_reconcile_duration_seconds_bucket[5m])
  )
)
```

### Alerting Queries

**Servers stuck in Creating phase for >10 minutes:**

```promql
mcpserver_phase{phase="Creating"} == 1
and time() - mcpserver_phase{phase="Creating"} > 600
```

**Servers with validation issues:**

```promql
mcpserver_phase{phase="ValidationFailed"} == 1
or mcpserver_validation_compliant == 0
```

## Troubleshooting

### Metrics Not Appearing

**Check ServiceMonitor:**

```bash
kubectl get servicemonitor -n mcp-operator-system
kubectl describe servicemonitor mcp-operator-controller-manager -n mcp-operator-system
```

**Verify labels match:**

The ServiceMonitor should match the operator service labels:

```bash
kubectl get svc -n mcp-operator-system mcp-operator-controller-manager-metrics-service -o yaml
```

**Check Prometheus targets:**

```bash
# Port forward to Prometheus
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090

# Visit http://localhost:9090/targets
# Look for mcp-operator-controller-manager target
```

**Check Prometheus operator logs:**

```bash
kubectl logs -n monitoring -l app.kubernetes.io/name=prometheus-operator
```

### Dashboard Not Loading

**Verify ConfigMap exists:**

```bash
kubectl get configmap mcp-operator-dashboard -n mcp-operator-system
```

**Check Grafana sidecar labels:**

If using automatic dashboard loading, verify the ConfigMap has the right labels:

```bash
kubectl get configmap mcp-operator-dashboard -n mcp-operator-system -o yaml
```

Should have labels like:

```yaml
labels:
  grafana_dashboard: "1"
```

**Manual import:**

Extract and manually import the dashboard (see [Installing the Dashboard](#installing-the-dashboard)).

### Missing Data Points

**Check metric endpoints:**

```bash
# Port forward to operator metrics
kubectl port-forward -n mcp-operator-system \
  svc/mcp-operator-controller-manager-metrics-service 8443:8443

# Query metrics (if HTTPS)
curl -k https://localhost:8443/metrics

# Or if HTTP
curl http://localhost:8443/metrics
```

**Verify MCPServers exist:**

Metrics are only generated for existing MCPServer resources:

```bash
kubectl get mcpservers --all-namespaces
```

**Check scrape interval:**

Metrics update based on Prometheus scrape interval (default: 30s). Wait a minute for data to appear.

### High Cardinality Warnings

**Symptom:** Prometheus warns about high cardinality metrics.

**Cause:** Too many unique label combinations (e.g., many MCPServer instances).

**Solution:** This is expected for large deployments. Consider:
- Increasing Prometheus resources
- Reducing metric retention period
- Using recording rules to pre-aggregate metrics

### Permission Errors

**Symptom:** ServiceMonitor created but Prometheus can't scrape.

**Check RBAC:**

```bash
kubectl get clusterrole prometheus-k8s -o yaml
```

Verify it has permissions to get/list/watch services and endpoints in `mcp-operator-system` namespace.

**Solution:** Update Prometheus RBAC if needed:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus-k8s
rules:
  - apiGroups: [""]
    resources:
      - services
      - endpoints
      - pods
    verbs: ["get", "list", "watch"]
```

## See Also

- [Configuration Guide](../configuration.md) - Configure your MCP servers
- [Troubleshooting Guide](troubleshooting.md) - Common issues and solutions
- [API Reference](../api-reference.md) - Complete CRD documentation
- [Prometheus Operator Documentation](https://prometheus-operator.dev/)
- [Grafana Documentation](https://grafana.com/docs/)
