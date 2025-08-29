# ADR-003: MCPServer Horizontal Pod Autoscaler Support

## Status

Accepted

## Context

The MCPServer operator needed to support automatic scaling capabilities to handle varying workloads efficiently. Horizontal Pod Autoscaling (HPA) is a critical requirement for production deployments where MCP server workload can fluctuate significantly based on:

1. Number of connected clients
2. Query complexity and frequency
3. Resource utilization patterns
4. Time-based usage patterns

Without HPA support, operators would need to manually scale MCPServer deployments, leading to either over-provisioning (wasted resources) or under-provisioning (performance degradation).

## Decision

We have implemented comprehensive HPA support for MCPServer resources with the following design decisions:

### API Design

Added `HPA` field to `MCPServerSpec` with complete configuration options:

```go
type MCPServerHPA struct {
    Enabled *bool `json:"enabled,omitempty"`
    MinReplicas *int32 `json:"minReplicas,omitempty"`
    MaxReplicas *int32 `json:"maxReplicas,omitempty"`
    TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
    TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`
    ScaleUpBehavior *MCPServerHPABehavior `json:"scaleUpBehavior,omitempty"`
    ScaleDownBehavior *MCPServerHPABehavior `json:"scaleDownBehavior,omitempty"`
}
```

### Controller Implementation

1. **HPA Lifecycle Management**: Controller creates, updates, and deletes HPA resources as part of the reconciliation loop
2. **Ownership Model**: HPA resources are owned by MCPServer resources for proper garbage collection
3. **Configuration Translation**: MCPServer HPA configuration is translated to Kubernetes HPA v2 API
4. **Advanced Scaling Behaviors**: Support for custom scale-up and scale-down policies with stabilization windows

### RBAC Integration

Added comprehensive HPA permissions to the controller:
- `horizontalpodautoscalers` resources with full CRUD access
- Integration with existing RBAC patterns

### Validation and Defaults

- Optional fields with sensible defaults
- Validation through Kubebuilder markers in CRD generation
- Error handling for invalid configurations

## Implementation Details

### Controller Logic

The controller implements HPA management in `reconcileHPA()` function:

```go
func (r *MCPServerReconciler) reconcileHPA(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
    if mcpServer.Spec.HPA == nil || mcpServer.Spec.HPA.Enabled == nil || !*mcpServer.Spec.HPA.Enabled {
        // Delete HPA if it exists but is disabled
        return r.deleteHPAIfExists(ctx, mcpServer)
    }
    
    hpa := r.buildHPA(mcpServer)
    // Create or update HPA resource
}
```

### HPA Resource Generation

The `buildHPA()` function creates Kubernetes HPA v2 resources with:
- CPU and memory utilization targets
- Custom scaling behaviors with policies
- Proper owner references and labels
- Namespace and naming conventions

### Error Handling

- Graceful degradation when metrics-server is unavailable
- Proper error reporting in MCPServer status conditions
- Retry logic for transient failures

## Alternatives Considered

1. **Custom Metrics Only**: Considered supporting only custom metrics, but CPU/memory are most common use cases
2. **HPA v1 API**: Rejected in favor of v2 API for advanced scaling behaviors
3. **External HPA Management**: Considered requiring users to manage HPA separately, but this would break the operator pattern

## Consequences

### Positive

1. **Production Ready**: MCPServer deployments can automatically scale based on resource utilization
2. **Cost Optimization**: Resources are scaled down during low usage periods
3. **Performance**: Automatic scale-up prevents performance degradation under load
4. **Operational Simplicity**: Single MCPServer resource manages both deployment and autoscaling
5. **Advanced Configuration**: Support for sophisticated scaling policies and behaviors

### Negative

1. **Complexity**: Added complexity to the controller and API surface
2. **Dependencies**: Requires metrics-server to be available in the cluster
3. **Learning Curve**: Users need to understand HPA concepts and tuning

### Mitigation Strategies

1. **Optional Feature**: HPA is disabled by default, users opt-in explicitly
2. **Comprehensive Documentation**: Provide examples and best practices
3. **Sensible Defaults**: Default values work for most common scenarios
4. **Error Reporting**: Clear status conditions when HPA cannot function

## Testing

- Integration tests verify HPA creation and updates
- Sample configurations demonstrate common use cases
- Error scenarios tested for graceful degradation

## Future Considerations

1. **Custom Metrics**: Support for application-specific metrics
2. **Predictive Scaling**: Integration with KEDA or similar for advanced scaling
3. **Multi-dimensional Scaling**: Support for scaling based on multiple metrics simultaneously

## References

- [Kubernetes HPA Documentation](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
- [HPA v2 API Reference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#horizontalpodautoscaler-v2-autoscaling)
- MCPServer API Design (ADR-001)
- MCPServer Controller Implementation (ADR-002)
