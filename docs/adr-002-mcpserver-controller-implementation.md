# ADR-002: MCPServer Controller Implementation

## Status

Accepted

## Context

Following the MCPServer API design (ADR-001), we needed to implement a comprehensive Kubernetes controller to manage the complete lifecycle of MCP servers. The controller must handle:

1. Resource reconciliation (Deployments, Services, ServiceAccounts, RBAC)
2. Status reporting and condition management
3. Enterprise security and operational requirements
4. Error handling and recovery
5. Proper Kubernetes controller patterns and best practices

## Decision

We have implemented a comprehensive MCPServer controller with the following architecture:

### Core Controller Pattern

#### Reconciliation Loop
- **Main Reconcile Function**: Orchestrates the complete resource lifecycle
- **Finalizer Management**: Proper cleanup handling with `mcp.mcp-operator.io/finalizer`
- **Error Handling**: Comprehensive error recovery with status updates
- **Requeue Strategy**: 5-minute requeue interval for ongoing monitoring

#### Resource Ownership
- **Controller References**: All owned resources properly linked to MCPServer
- **Cascade Deletion**: Automatic cleanup of owned resources
- **Resource Watching**: Controller watches all owned resource types for changes

### Resource Management Functions

#### Deployment Management (`reconcileDeployment`, `buildDeployment`)
- **Container Specification**: Configurable image, ports, environment variables
- **Replica Management**: Scaling support with configurable replica counts
- **Health Checks**: Comprehensive liveness and readiness probes
- **Security Contexts**: Container-level security settings (RunAsUser, ReadOnlyRootFilesystem)
- **Resource Requirements**: CPU/memory limits and requests
- **Pod Template Features**: Labels, annotations, node selectors, tolerations, affinity
- **Volume Management**: Support for additional volumes and volume mounts

#### Service Management (`reconcileService`, `buildService`)
- **Service Types**: Support for ClusterIP, NodePort, LoadBalancer
- **Port Configuration**: Configurable ports and protocols
- **Service Discovery**: Proper labeling for pod selection
- **Annotations**: Custom service annotations support

#### Security & RBAC (`reconcileRBAC`, `reconcileServiceAccount`)
- **ServiceAccount Creation**: Automatic service account provisioning
- **Role-Based Access**: Dynamic Role and RoleBinding creation
- **User/Group Access**: Support for allowed users and groups configuration
- **Permission Management**: Scoped permissions for service access

### Status Management

#### Phase Tracking (`updateMCPServerStatus`)
- **Lifecycle Phases**: Pending, Creating, Running, Scaling, Updating, Failed, Terminating
- **Deployment Status Mapping**: Phase determination based on deployment readiness
- **Replica Counts**: Tracking of total, ready, and available replicas
- **Service Endpoints**: Automatic endpoint discovery and reporting

#### Condition Management (`updateConditions`, `setCondition`)
- **Standard Conditions**: Ready, Available, Progressing, Degraded, Reconciled
- **Condition Transitions**: Proper timestamp and reason tracking
- **Error Conditions**: Failed reconciliation reporting with detailed messages
- **Status Aggregation**: Deployment condition mapping to MCPServer conditions

### Enterprise Features

#### Security Implementation
```go
// Container security context with configurable settings
securityContext := &corev1.SecurityContext{
    RunAsUser:                mcpServer.Spec.Security.RunAsUser,
    RunAsGroup:               mcpServer.Spec.Security.RunAsGroup,
    ReadOnlyRootFilesystem:   mcpServer.Spec.Security.ReadOnlyRootFilesystem,
}
```

#### RBAC Integration
```go
// Dynamic role creation for user/group access control
role := &rbacv1.Role{
    Rules: []rbacv1.PolicyRule{{
        APIGroups:     []string{""},
        Resources:     []string{"services"},
        Verbs:         []string{"get", "list"},
        ResourceNames: []string{mcpServer.Name},
    }},
}
```

#### Operational Features
- **Health Check Configuration**: Customizable probe parameters
- **Resource Monitoring**: Status reporting with metrics integration
- **Service Discovery**: Automatic endpoint resolution for different service types
- **Labels and Annotations**: Full support for Kubernetes metadata

### Error Handling Strategy

#### Status Error Management (`updateStatusWithError`)
- **Error Phase Setting**: Automatic transition to Failed phase
- **Condition Updates**: Error condition with detailed messages
- **Requeue Strategy**: 2-minute requeue for error recovery
- **Status Persistence**: Guaranteed status updates even during failures

#### Reconciliation Recovery
- **Idempotent Operations**: All reconciliation functions are idempotent
- **Partial Failure Handling**: Continue processing other resources on partial failures
- **Resource Drift Detection**: Automatic correction of configuration drift

### Generated Artifacts Integration

#### CRD Generation Compatibility
- **Type Safety**: Fixed float64 fields to string for CRD compatibility
- **Validation Markers**: All kubebuilder markers properly applied
- **Schema Generation**: Complete OpenAPI v3 schema validation

#### RBAC Automation
- **Permission Annotation**: All required permissions defined in controller annotations
- **Automatic Generation**: RBAC manifests generated from controller code
- **Principle of Least Privilege**: Scoped permissions for each resource type

## Implementation Details

### Key Constants
```go
const (
    mcpServerFinalizer = "mcp.mcp-operator.io/finalizer"
    defaultPort        = 8080
    defaultHealthPath  = "/health"
    defaultMetricsPath = "/metrics"
)
```

### Resource Labeling Strategy
```go
labels := map[string]string{
    "app":                          mcpServer.Name,
    "app.kubernetes.io/name":       "mcpserver",
    "app.kubernetes.io/instance":   mcpServer.Name,
    "app.kubernetes.io/component":  "mcp-server",
    "app.kubernetes.io/managed-by": "mcp-operator",
}
```

### Controller Manager Setup
```go
ctrl.NewControllerManagedBy(mgr).
    For(&mcpv1.MCPServer{}).
    Owns(&appsv1.Deployment{}).
    Owns(&corev1.Service{}).
    Owns(&corev1.ServiceAccount{}).
    Owns(&rbacv1.Role{}).
    Owns(&rbacv1.RoleBinding{}).
    Named("mcpserver").
    Complete(r)
```

## Rationale

### Design Principles

1. **Kubernetes Native**: Follow standard controller patterns and best practices
2. **Enterprise Ready**: Comprehensive security, RBAC, and operational features
3. **Resilient**: Robust error handling and recovery mechanisms
4. **Observable**: Rich status reporting and condition management
5. **Maintainable**: Clear separation of concerns and modular design

### Key Design Decisions

#### Comprehensive Resource Management
- **Single Controller**: Manage all related resources in one controller for consistency
- **Owner References**: Ensure proper garbage collection and resource relationships
- **Configuration Drift**: Automatic detection and correction of resource changes

#### Status-First Approach
- **Rich Status**: Detailed phase and condition reporting for operational visibility
- **Error Transparency**: Clear error messages and recovery guidance
- **Monitoring Integration**: Status fields designed for metrics and alerting systems

#### Security by Design
- **RBAC Integration**: Native Kubernetes RBAC for access control
- **Security Contexts**: Container-level security with configurable settings
- **Service Isolation**: Scoped permissions and service account isolation

#### Enterprise Operational Features
- **Health Monitoring**: Comprehensive health check configuration
- **Resource Management**: Full support for resource limits and scheduling
- **Service Discovery**: Automatic endpoint management for different deployment types

## Consequences

### Positive
- **Complete Lifecycle Management**: Full automation of MCP server operations
- **Enterprise Ready**: Meets all security and operational requirements
- **Kubernetes Native**: Leverages standard Kubernetes patterns and features
- **Maintainable**: Clear code structure with separated concerns
- **Observable**: Rich status and condition reporting for operations teams

### Negative
- **Complexity**: Comprehensive feature set increases controller complexity
- **Resource Usage**: Multiple owned resources per MCPServer instance
- **Testing Requirements**: Complex reconciliation logic requires thorough testing

### Mitigation Strategies
- **Modular Design**: Separated functions for different resource types
- **Comprehensive Testing**: Unit and integration tests for all reconciliation paths
- **Documentation**: Clear ADRs and code comments for maintainability
- **Error Handling**: Robust error recovery and status reporting

## Testing Strategy

### Unit Testing Requirements
- **Reconciliation Functions**: Test each resource reconciliation function independently
- **Status Management**: Verify correct phase transitions and condition updates
- **Error Scenarios**: Test error handling and recovery paths
- **Security Features**: Validate RBAC and security context creation

### Integration Testing
- **End-to-End Flows**: Complete MCPServer lifecycle testing
- **Resource Ownership**: Verify proper cascade deletion and garbage collection
- **Configuration Changes**: Test update and scaling scenarios
- **Error Recovery**: Validate controller recovery from various failure modes

## Future Enhancements

### Monitoring and Metrics
- **Prometheus Integration**: Controller metrics for operational monitoring
- **Resource Metrics**: MCP server performance and usage metrics
- **Alerting Rules**: Predefined alerts for common failure scenarios

### Advanced Features
- **Horizontal Pod Autoscaling**: Automatic scaling based on metrics
- **Blue-Green Deployments**: Zero-downtime update strategies
- **Multi-Cluster Support**: Cross-cluster MCP server management

## References

- **Kubernetes Controller Patterns**: https://book.kubebuilder.io/cronjob-tutorial/controller-overview.html
- **Controller Runtime**: https://pkg.go.dev/sigs.k8s.io/controller-runtime
- **Operator Best Practices**: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
- **RBAC Documentation**: https://kubernetes.io/docs/reference/access-authn-authz/rbac/
