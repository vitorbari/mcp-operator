# ADR-005: Prometheus Metrics and Ingress Observability

## Status

Implemented

## Context

The MCP Operator lacks metrics collection for monitoring controller performance and MCP server usage patterns. Production operators require:

- Controller reconciliation metrics
- MCP server health and status tracking
- HTTP traffic analytics via ingress
- Integration with Prometheus monitoring stack

## Decision

Implement Prometheus metrics and ingress-based observability:

### Prometheus Metrics

Expose operator metrics via `/metrics` endpoint:

```go
// Controller Metrics
reconcile_duration_seconds{controller="mcpserver"} - Reconciliation latency
reconcile_total{controller="mcpserver",result="success|error"} - Reconciliation count
mcpserver_ready_total{namespace,name} - Ready MCPServers count
mcpserver_replicas{namespace,name,transport_type} - Current replica count

// Transport Metrics  
mcpserver_transport_type{type="http|custom"} - Transport distribution
mcpserver_ingress_enabled{namespace,name} - External access tracking

// Resource Metrics
mcpserver_resource_requests{resource="cpu|memory",namespace,name} - Resource allocation
mcpserver_hpa_enabled{namespace,name} - Autoscaling usage
```

### ServiceMonitor Configuration

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: mcp-operator-metrics
  namespace: mcp-operator-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
  - port: https
    path: /metrics
    interval: 30s
    scheme: https
    tlsConfig:
      insecureSkipVerify: true
```

### Ingress Traffic Metrics

Configure ingress controllers to collect MCP-specific metrics:

```yaml
# NGINX Ingress annotations
nginx.ingress.kubernetes.io/enable-metrics: "true"
nginx.ingress.kubernetes.io/server-snippet: |
  access_log /var/log/nginx/mcp-access.log json;
  
# Custom log format for MCP analysis
log_format mcp_json escape=json '{'
  '"timestamp":"$time_iso8601",'
  '"remote_addr":"$remote_addr",'
  '"request_method":"$request_method",'
  '"request_uri":"$request_uri",'
  '"status":$status,'
  '"request_time":$request_time,'
  '"upstream_response_time":"$upstream_response_time",'
  '"mcp_transport":"$http_mcp_protocol_version"'
'}';
```

## Implementation

### 1. Custom Metrics Collection

Add Prometheus metrics using controller-runtime global registry:

```go
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
    
    transportTypeDistribution = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mcpserver_transport_type_total",
            Help: "Total count of transport types used",
        },
        []string{"type"},
    )
)

func init() {
    // Register custom metrics with the global prometheus registry
    metrics.Registry.MustRegister(mcpServerReady, mcpServerReplicas, transportTypeDistribution)
}
```

### 2. Metrics Collection in Controller

```go
func (r *MCPServerReconciler) updateMetrics(mcpServer *mcpv1.MCPServer) {
    labels := []string{mcpServer.Namespace, mcpServer.Name}
    
    // Track ready status
    if mcpServer.Status.ReadyReplicas > 0 {
        mcpServerReady.WithLabelValues(labels...).Set(1)
    } else {
        mcpServerReady.WithLabelValues(labels...).Set(0)
    }
    
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
    
    transportTypeDistribution.WithLabelValues(transportType).Inc()
}
```

### 3. ServiceMonitor Creation

Add ServiceMonitor to operator manifests:

```go
func (r *MCPServerReconciler) createServiceMonitor() error {
    serviceMonitor := &monitoringv1.ServiceMonitor{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mcp-operator-metrics",
            Namespace: "mcp-operator-system",
        },
        Spec: monitoringv1.ServiceMonitorSpec{
            Selector: metav1.LabelSelector{
                MatchLabels: map[string]string{
                    "control-plane": "controller-manager",
                },
            },
            Endpoints: []monitoringv1.Endpoint{{
                Port:     "https",
                Path:     "/metrics",
                Interval: "30s",
                Scheme:   "https",
                TLSConfig: &monitoringv1.TLSConfig{
                    InsecureSkipVerify: true,
                },
            }},
        },
    }
    
    return r.Create(context.TODO(), serviceMonitor)
}
```

### 4. Ingress Annotations

Add metrics annotations when creating ingress:

```go
func (r *MCPServerReconciler) createIngress(mcpServer *mcpv1.MCPServer) error {
    annotations := map[string]string{
        "nginx.ingress.kubernetes.io/proxy-read-timeout":    "3600",
        "nginx.ingress.kubernetes.io/proxy-send-timeout":    "3600",
        "nginx.ingress.kubernetes.io/enable-metrics":        "true",
        "nginx.ingress.kubernetes.io/server-snippet": `
            access_log /var/log/nginx/mcp-access.log json;
        `,
    }
    
    ingress := &networkingv1.Ingress{
        ObjectMeta: metav1.ObjectMeta{
            Name:        mcpServer.Name + "-ingress",
            Namespace:   mcpServer.Namespace,
            Annotations: annotations,
        },
        // ... spec configuration
    }
    
    return r.Create(context.TODO(), ingress)
}
```

## Expected Metrics

### Operator Health
- Reconciliation success/failure rates
- Resource creation latency
- Controller memory/CPU usage

### MCPServer Status  
- Ready vs desired replicas
- Transport configuration distribution
- External access patterns

### Usage Analytics
- Request volume per MCP server
- Error rates by transport type
- Response times

## Benefits

- Production-ready monitoring
- Performance optimization data
- Usage pattern analysis
- Integration with existing monitoring infrastructure

## References

- [Prometheus Operator Metrics](https://prometheus-operator.dev/docs/operator/design/#metrics)
- [Controller Runtime Metrics](https://book.kubebuilder.io/reference/metrics.html)
