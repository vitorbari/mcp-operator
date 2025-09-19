# ADR-006: Custom Metrics and Dashboard Implementation

## Status

Implemented

## Context

The MCP Operator has basic observability infrastructure (events, ServiceMonitor) but lacks:
- Custom Prometheus metrics for MCP-specific monitoring
- Transport-specific ingress annotations for traffic analytics
- Grafana dashboards for operational visibility

## Decision

Implement missing observability components:

### Custom Prometheus Metrics

Add MCP-specific metrics to controller:

```go
// internal/controller/metrics.go
package controller

import (
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
    mcpServerReady = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mcpserver_ready_total",
            Help: "Number of ready MCP servers",
        },
        []string{"namespace", "name"},
    )
    
    mcpServerReplicas = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mcpserver_replicas",
            Help: "Current replica count per MCP server",
        },
        []string{"namespace", "name", "transport_type"},
    )
    
    transportTypeTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mcpserver_transport_type_total",
            Help: "Total count of transport types used",
        },
        []string{"type"},
    )
    
    ingressEnabled = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mcpserver_ingress_enabled",
            Help: "Whether ingress is enabled for MCP server",
        },
        []string{"namespace", "name"},
    )
)

func init() {
    metrics.Registry.MustRegister(
        mcpServerReady,
        mcpServerReplicas, 
        transportTypeTotal,
        ingressEnabled,
    )
}
```

### Metrics Collection in Controller

```go
// Update reconcile loop to collect metrics
func (r *MCPServerReconciler) updateMetrics(mcpServer *mcpv1.MCPServer) {
    labels := []string{mcpServer.Namespace, mcpServer.Name}
    
    // Track ready status
    readyValue := float64(0)
    if mcpServer.Status.ReadyReplicas > 0 {
        readyValue = 1
    }
    mcpServerReady.WithLabelValues(labels...).Set(readyValue)
    
    // Track replica count and transport type
    transportType := "http"
    if mcpServer.Spec.Transport != nil {
        transportType = mcpServer.Spec.Transport.Type
    }
    
    mcpServerReplicas.WithLabelValues(
        mcpServer.Namespace,
        mcpServer.Name,
        transportType,
    ).Set(float64(mcpServer.Status.Replicas))
    
    // Increment transport type counter (only on creation)
    if mcpServer.Status.ObservedGeneration == 1 {
        transportTypeTotal.WithLabelValues(transportType).Inc()
    }
    
    // Track ingress enablement
    ingressValue := float64(0)
    if mcpServer.Spec.Ingress != nil && mcpServer.Spec.Ingress.Enabled {
        ingressValue = 1
    }
    ingressEnabled.WithLabelValues(labels...).Set(ingressValue)
}
```

### Transport-Specific Ingress Annotations

```go
// Add to ingress creation logic
func getIngressAnnotations(mcpServer *mcpv1.MCPServer) map[string]string {
    annotations := map[string]string{
        // Base timeouts for streaming connections
        "nginx.ingress.kubernetes.io/proxy-read-timeout":    "3600",
        "nginx.ingress.kubernetes.io/proxy-send-timeout":    "3600",
        "nginx.ingress.kubernetes.io/proxy-connect-timeout": "60",
        
        // Enable metrics collection
        "nginx.ingress.kubernetes.io/enable-metrics": "true",
        
        // Custom log format for MCP analytics
        "nginx.ingress.kubernetes.io/server-snippet": `
            access_log /var/log/nginx/mcp-access.log json_combined;
            
            location /mcp {
                proxy_set_header X-MCP-Transport "` + getMCPTransportType(mcpServer) + `";
                proxy_pass $target;
            }
        `,
    }
    
    // Transport-specific annotations
    if mcpServer.Spec.Transport != nil {
        switch mcpServer.Spec.Transport.Type {
        case "http":
            annotations["nginx.ingress.kubernetes.io/upstream-hash-by"] = "$http_mcp_session_id"
        case "custom":
            if config := mcpServer.Spec.Transport.Config.Custom; config != nil {
                if config.Protocol == "tcp" {
                    annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTP"
                }
            }
        }
    }
    
    return annotations
}
```

### Grafana Dashboard ConfigMap

```yaml
# config/grafana/mcp-operator-dashboard.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mcp-operator-dashboard
  namespace: mcp-operator-system
  labels:
    grafana_dashboard: "1"
    app.kubernetes.io/name: mcp-operator
    app.kubernetes.io/component: observability
data:
  mcp-operator.json: |
    {
      "dashboard": {
        "id": null,
        "title": "MCP Operator Overview",
        "description": "Monitor MCP Operator and MCPServer resources",
        "tags": ["mcp", "operator", "kubernetes"],
        "timezone": "browser",
        "panels": [
          {
            "id": 1,
            "title": "MCPServer Status",
            "type": "stat",
            "targets": [
              {
                "expr": "sum(mcpserver_ready_total)",
                "legendFormat": "Ready Servers"
              }
            ],
            "fieldConfig": {
              "defaults": {
                "color": {"mode": "palette-classic"},
                "unit": "short"
              }
            }
          },
          {
            "id": 2, 
            "title": "Transport Type Distribution",
            "type": "piechart",
            "targets": [
              {
                "expr": "sum by (type) (mcpserver_transport_type_total)",
                "legendFormat": "{{type}}"
              }
            ]
          },
          {
            "id": 3,
            "title": "MCPServer Replicas",
            "type": "timeseries", 
            "targets": [
              {
                "expr": "mcpserver_replicas",
                "legendFormat": "{{namespace}}/{{name}} ({{transport_type}})"
              }
            ]
          },
          {
            "id": 4,
            "title": "Controller Reconciliation Rate",
            "type": "timeseries",
            "targets": [
              {
                "expr": "rate(controller_runtime_reconcile_total{controller=\"mcpserver\"}[5m])",
                "legendFormat": "{{result}}"
              }
            ]
          }
        ],
        "time": {
          "from": "now-1h",
          "to": "now"
        },
        "refresh": "30s"
      }
    }
```

### Kustomization Update

```yaml
# config/grafana/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- mcp-operator-dashboard.yaml

commonLabels:
  app.kubernetes.io/name: mcp-operator
  app.kubernetes.io/component: observability
```

## Implementation Files

1. **Create `internal/controller/metrics.go`** - Custom metrics definitions
2. **Update `internal/controller/mcpserver_controller.go`** - Add metrics collection
3. **Create `config/grafana/`** - Dashboard ConfigMaps
4. **Update `config/default/kustomization.yaml`** - Include grafana resources

## Benefits

- MCP-specific monitoring visibility
- Transport usage analytics
- Operational dashboards for production use
- HTTP traffic insights via ingress

## References

- [Kubebuilder Metrics](https://book.kubebuilder.io/reference/metrics)
- [Grafana Dashboard Provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/#dashboards)
