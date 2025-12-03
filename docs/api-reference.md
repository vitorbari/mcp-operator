# MCPServer API Reference

Complete reference documentation for the `MCPServer` Custom Resource Definition (CRD).

## Table of Contents

- [Overview](#overview)
- [MCPServerSpec](#mcpserverspec)
  - [Core Fields](#core-fields)
  - [Transport Configuration](#transport-configuration)
  - [Resources](#resources)
  - [Security](#security)
  - [Service](#service)
  - [Health Checks](#health-checks)
  - [Environment Variables](#environment-variables)
  - [Pod Template](#pod-template)
  - [Horizontal Pod Autoscaler (HPA)](#horizontal-pod-autoscaler-hpa)
  - [Validation](#validation)
- [MCPServerStatus](#mcpserverstatus)
- [Security Defaults](#security-defaults)

## Overview

The `MCPServer` resource defines an MCP (Model Context Protocol) server deployment in Kubernetes. The operator manages the complete lifecycle including deployment, scaling, validation, and monitoring.

**API Group:** `mcp.mcp-operator.io/v1`

**Kind:** `MCPServer`

**Example:**

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"
  replicas: 3
  transport:
    type: http
    protocol: auto
```

## MCPServerSpec

The `spec` section defines the desired state of your MCP server.

### Core Fields

#### `image` (required)

- **Type:** `string`
- **Description:** Container image for the MCP server
- **Validation:** Must match pattern `^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::[a-zA-Z0-9][a-zA-Z0-9._-]*)?$`
- **Example:**
  ```yaml
  image: "mcp/wikipedia-mcp:latest"
  ```

#### `command` (optional)

- **Type:** `[]string`
- **Description:** Override the container's default entrypoint
- **Default:** Uses image's default ENTRYPOINT
- **Example:**
  ```yaml
  command: ["python", "-m", "wikipedia_mcp"]
  ```

#### `args` (optional)

- **Type:** `[]string`
- **Description:** Override or append to the container's default arguments
- **Default:** Uses image's default CMD
- **Example:**
  ```yaml
  args: ["--transport", "sse", "--port", "8080", "--host", "0.0.0.0"]
  ```

#### `replicas` (optional)

- **Type:** `int32`
- **Description:** Number of MCP server instances to run
- **Default:** `1`
- **Validation:** Minimum value is `0`
- **Example:**
  ```yaml
  replicas: 3
  ```

### Transport Configuration

#### `transport` (optional)

Defines the MCP transport configuration. If omitted, defaults to HTTP transport with auto-detection.

**Type:** `object`

**Fields:**

##### `transport.type` (optional)

- **Type:** `string`
- **Description:** Transport layer type
- **Valid Values:** `http`, `custom`
- **Default:** `http`
- **Example:**
  ```yaml
  transport:
    type: http
  ```

##### `transport.protocol` (optional)

- **Type:** `string`
- **Description:** MCP protocol variant to use
- **Valid Values:**
  - `auto` - Auto-detect protocol (prefers Streamable HTTP over SSE)
  - `streamable-http` - Use Streamable HTTP transport (MCP 2025-03-26+)
  - `sse` - Use Server-Sent Events transport (MCP 2024-11-05)
- **Default:** `auto`
- **Example:**
  ```yaml
  transport:
    protocol: auto
  ```

##### `transport.config` (optional)

- **Type:** `object`
- **Description:** Transport-specific configuration
- **Fields:**

###### `transport.config.http` (optional)

HTTP transport configuration:

- **`port`** (optional)
  - **Type:** `int32`
  - **Default:** `8080`
  - **Validation:** Between 1 and 65535
  - **Description:** Port for HTTP connections

- **`path`** (optional)
  - **Type:** `string`
  - **Default:** Auto-detected (`/mcp` for Streamable HTTP, `/sse` for SSE)
  - **Description:** HTTP endpoint path

- **`sessionManagement`** (optional)
  - **Type:** `bool`
  - **Description:** Enable session management for the HTTP transport

**Example:**

```yaml
transport:
  type: http
  protocol: auto
  config:
    http:
      port: 8080
      path: "/mcp"
      sessionManagement: true
```

### Resources

#### `resources` (optional)

- **Type:** `object` (Kubernetes `ResourceRequirements`)
- **Description:** CPU and memory resource requirements
- **Default:** No resource limits or requests set
- **Fields:**
  - `requests` - Minimum resources needed
    - `cpu` - CPU request (e.g., "200m", "1")
    - `memory` - Memory request (e.g., "256Mi", "1Gi")
  - `limits` - Maximum resources allowed
    - `cpu` - CPU limit
    - `memory` - Memory limit
- **Example:**
  ```yaml
  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"
  ```

### Security

#### `security` (optional)

Security-related configuration for the MCP server. When omitted, the operator applies secure defaults compliant with [Kubernetes Pod Security Standards (Restricted)](https://kubernetes.io/docs/concepts/security/pod-security-standards/).

**Type:** `object`

**Fields:**

##### `security.runAsUser` (optional)

- **Type:** `int64`
- **Description:** User ID to run the MCP server process
- **Default:** `1000`
- **Example:**
  ```yaml
  security:
    runAsUser: 2000
  ```

##### `security.runAsGroup` (optional)

- **Type:** `int64`
- **Description:** Group ID to run the MCP server process
- **Default:** `1000`
- **Example:**
  ```yaml
  security:
    runAsGroup: 2000
  ```

##### `security.runAsNonRoot` (optional)

- **Type:** `bool`
- **Description:** Indicates that the container must run as a non-root user
- **Default:** `true`
- **Example:**
  ```yaml
  security:
    runAsNonRoot: true
  ```

##### `security.allowPrivilegeEscalation` (optional)

- **Type:** `bool`
- **Description:** Controls whether a process can gain more privileges than its parent
- **Default:** `false`
- **Example:**
  ```yaml
  security:
    allowPrivilegeEscalation: false
  ```

##### `security.fsGroup` (optional)

- **Type:** `int64`
- **Description:** Special supplemental group for volume permissions
- **Default:** `1000`
- **Example:**
  ```yaml
  security:
    fsGroup: 2000
  ```

##### `security.readOnlyRootFilesystem` (optional)

- **Type:** `bool`
- **Description:** Whether the container should have a read-only root filesystem
- **Default:** Not set (container can write to root filesystem)
- **Example:**
  ```yaml
  security:
    readOnlyRootFilesystem: true
  ```

**Complete Example:**

```yaml
security:
  runAsUser: 1000
  runAsGroup: 1000
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  fsGroup: 1000
  readOnlyRootFilesystem: true
```

### Service

#### `service` (optional)

Defines how the MCP server is exposed as a Kubernetes service.

**Type:** `object`

**Fields:**

##### `service.type` (optional)

- **Type:** `string`
- **Description:** Type of Kubernetes service
- **Valid Values:** `ClusterIP`, `NodePort`, `LoadBalancer`
- **Default:** `ClusterIP`
- **Example:**
  ```yaml
  service:
    type: ClusterIP
  ```

##### `service.port` (optional)

- **Type:** `int32`
- **Description:** Port on which the service listens
- **Default:** `8080`
- **Validation:** Between 1 and 65535
- **Example:**
  ```yaml
  service:
    port: 8080
  ```

##### `service.targetPort` (optional)

- **Type:** `intstr.IntOrString`
- **Description:** Container port to target (can be port number or name)
- **Default:** Same as `service.port`
- **Example:**
  ```yaml
  service:
    targetPort: 8080
  ```

##### `service.protocol` (optional)

- **Type:** `string`
- **Description:** Network protocol
- **Valid Values:** `TCP`, `UDP`
- **Default:** `TCP`
- **Example:**
  ```yaml
  service:
    protocol: TCP
  ```

##### `service.annotations` (optional)

- **Type:** `map[string]string`
- **Description:** Annotations to add to the service (useful for cloud load balancers)
- **Example:**
  ```yaml
  service:
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
  ```

### Health Checks

#### `healthCheck` (optional)

Defines health check probe configuration for the MCP server.

**Type:** `object`

**Fields:**

##### `healthCheck.enabled` (optional)

- **Type:** `bool`
- **Description:** Whether health checks should be performed
- **Default:** `true`
- **Example:**
  ```yaml
  healthCheck:
    enabled: true
  ```

##### `healthCheck.path` (optional)

- **Type:** `string`
- **Description:** HTTP path for health checks
- **Default:** `/health`
- **Example:**
  ```yaml
  healthCheck:
    path: "/health"
  ```

##### `healthCheck.port` (optional)

- **Type:** `intstr.IntOrString`
- **Description:** Port for health checks (can be port number or name)
- **Default:** Uses the transport HTTP port
- **Example:**
  ```yaml
  healthCheck:
    port: 8080
  ```

### Environment Variables

#### `environment` (optional)

- **Type:** `[]object` (array of Kubernetes `EnvVar`)
- **Description:** Environment variables for the MCP server
- **Default:** Empty array
- **Example:**
  ```yaml
  environment:
    - name: LOG_LEVEL
      value: "info"
    - name: MCP_PORT
      value: "8080"
  ```

For detailed information on environment variable configuration including ConfigMaps, Secrets, and best practices, see the [Environment Variables Guide](environment-variables.md).

### Pod Template

#### `podTemplate` (optional)

Additional pod template specifications for advanced configuration.

**Type:** `object`

**Fields:**

##### `podTemplate.labels` (optional)

- **Type:** `map[string]string`
- **Description:** Additional labels to add to pods
- **Example:**
  ```yaml
  podTemplate:
    labels:
      monitoring: enabled
  ```

##### `podTemplate.annotations` (optional)

- **Type:** `map[string]string`
- **Description:** Additional annotations to add to pods
- **Example:**
  ```yaml
  podTemplate:
    annotations:
      prometheus.io/scrape: "true"
  ```

##### `podTemplate.nodeSelector` (optional)

- **Type:** `map[string]string`
- **Description:** Node selection constraints
- **Example:**
  ```yaml
  podTemplate:
    nodeSelector:
      disktype: ssd
  ```

##### `podTemplate.tolerations` (optional)

- **Type:** `[]object` (array of Kubernetes `Toleration`)
- **Description:** Pod tolerations for node taints
- **Example:**
  ```yaml
  podTemplate:
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "mcp-servers"
        effect: "NoSchedule"
  ```

##### `podTemplate.affinity` (optional)

- **Type:** `object` (Kubernetes `Affinity`)
- **Description:** Pod affinity and anti-affinity rules
- **Example:**
  ```yaml
  podTemplate:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values: ["my-mcp-server"]
              topologyKey: kubernetes.io/hostname
  ```

##### `podTemplate.serviceAccountName` (optional)

- **Type:** `string`
- **Description:** Service account to use for the pod
- **Example:**
  ```yaml
  podTemplate:
    serviceAccountName: mcp-service-account
  ```

##### `podTemplate.imagePullSecrets` (optional)

- **Type:** `[]object` (array of Kubernetes `LocalObjectReference`)
- **Description:** Secrets for pulling container images
- **Example:**
  ```yaml
  podTemplate:
    imagePullSecrets:
      - name: registry-credentials
  ```

##### `podTemplate.volumes` (optional)

- **Type:** `[]object` (array of Kubernetes `Volume`)
- **Description:** Additional volumes to mount
- **Example:**
  ```yaml
  podTemplate:
    volumes:
      - name: config-volume
        configMap:
          name: mcp-config
  ```

##### `podTemplate.volumeMounts` (optional)

- **Type:** `[]object` (array of Kubernetes `VolumeMount`)
- **Description:** Volume mounts for the container
- **Example:**
  ```yaml
  podTemplate:
    volumeMounts:
      - name: config-volume
        mountPath: /etc/mcp/config
        readOnly: true
  ```

### Horizontal Pod Autoscaler (HPA)

#### `hpa` (optional)

Horizontal Pod Autoscaler configuration for automatic scaling based on metrics.

**Type:** `object`

**Fields:**

##### `hpa.enabled` (optional)

- **Type:** `bool`
- **Description:** Whether HPA should be created
- **Default:** `false`
- **Example:**
  ```yaml
  hpa:
    enabled: true
  ```

##### `hpa.minReplicas` (optional)

- **Type:** `int32`
- **Description:** Lower limit for the number of replicas
- **Default:** `1`
- **Validation:** Minimum value is `1`
- **Example:**
  ```yaml
  hpa:
    minReplicas: 2
  ```

##### `hpa.maxReplicas` (optional)

- **Type:** `int32`
- **Description:** Upper limit for the number of replicas
- **Default:** `10`
- **Validation:** Minimum value is `1`
- **Example:**
  ```yaml
  hpa:
    maxReplicas: 20
  ```

##### `hpa.targetCPUUtilizationPercentage` (optional)

- **Type:** `int32`
- **Description:** Target average CPU utilization percentage
- **Validation:** Between 1 and 100
- **Example:**
  ```yaml
  hpa:
    targetCPUUtilizationPercentage: 70
  ```

##### `hpa.targetMemoryUtilizationPercentage` (optional)

- **Type:** `int32`
- **Description:** Target average memory utilization percentage
- **Validation:** Between 1 and 100
- **Example:**
  ```yaml
  hpa:
    targetMemoryUtilizationPercentage: 80
  ```

##### `hpa.scaleUpBehavior` (optional)

- **Type:** `object`
- **Description:** Configures scaling up behavior
- **Fields:**
  - `stabilizationWindowSeconds` (int32, 1-3600) - Seconds to consider past recommendations
  - `policies` (array) - List of scaling policies
    - `type` (string) - `Percent` or `Pods`
    - `value` (int32, min 1) - Amount of change permitted
    - `periodSeconds` (int32, 1-1800) - How long to hold the policy
- **Example:**
  ```yaml
  hpa:
    scaleUpBehavior:
      stabilizationWindowSeconds: 60
      policies:
        - type: "Percent"
          value: 100
          periodSeconds: 15
  ```

##### `hpa.scaleDownBehavior` (optional)

- **Type:** `object`
- **Description:** Configures scaling down behavior
- **Fields:** Same as `scaleUpBehavior`
- **Example:**
  ```yaml
  hpa:
    scaleDownBehavior:
      stabilizationWindowSeconds: 300
      policies:
        - type: "Pods"
          value: 1
          periodSeconds: 60
  ```

### Validation

#### `validation` (optional)

MCP protocol validation configuration. **IMPORTANT:** Validation is **ENABLED BY DEFAULT** even when this field is omitted.

**Type:** `object`

**Fields:**

##### `validation.enabled` (optional)

- **Type:** `bool`
- **Description:** Whether protocol validation should be performed
- **Default:** `true` (validation runs even when `spec.validation` is omitted)
- **Note:** Set to `false` to explicitly disable all validation
- **Example:**
  ```yaml
  validation:
    enabled: false
  ```

##### `validation.transportProtocol` (optional)

- **Type:** `string`
- **Description:** Which transport protocol to validate against
- **Valid Values:** `auto`, `streamable-http`, `sse`
- **Default:** Uses `spec.transport.protocol`
- **Example:**
  ```yaml
  validation:
    transportProtocol: auto
  ```

##### `validation.strictMode` (optional)

- **Type:** `bool`
- **Description:** Fail deployment if validation fails
- **Default:** `false` (deployment continues, issues reported in status)
- **Behavior:** When `true`, MCPServer phase becomes "ValidationFailed" and deployment scales to 0
- **Example:**
  ```yaml
  validation:
    strictMode: true
  ```

##### `validation.requiredCapabilities` (optional)

- **Type:** `[]string`
- **Description:** Capabilities that must be present for validation to succeed
- **Valid Values:** `tools`, `resources`, `prompts`
- **Default:** Empty (no required capabilities)
- **Example:**
  ```yaml
  validation:
    requiredCapabilities:
      - "tools"
      - "resources"
  ```

**Complete Example:**

```yaml
validation:
  enabled: true
  transportProtocol: auto
  strictMode: false
  requiredCapabilities:
    - "tools"
    - "resources"
```

For comprehensive details on validation behavior, see the [Validation Behavior Guide](validation-behavior.md).

## MCPServerStatus

The `status` section reflects the observed state of the MCP server.

### Status Fields

#### `phase` (string)

Current phase of the MCP server deployment.

**Valid Values:**
- `Pending` - MCP server is pending creation
- `Creating` - MCP server is being created
- `Running` - MCP server is running normally
- `Scaling` - MCP server is scaling up or down
- `Updating` - MCP server is being updated
- `Failed` - MCP server deployment failed
- `ValidationFailed` - Validation failed in strict mode
- `Terminating` - MCP server is being terminated

#### `message` (string)

Additional information about the current phase.

#### `replicas` (int32)

Current number of running replicas.

#### `readyReplicas` (int32)

Number of ready replicas.

#### `availableReplicas` (int32)

Number of available replicas.

#### `serviceEndpoint` (string)

Endpoint where the MCP server is accessible (e.g., `http://my-server.default.svc:8080/mcp`).

#### `transportType` (string)

Active transport type (e.g., `http`).

#### `lastReconcileTime` (timestamp)

Last time the MCP server was reconciled.

#### `observedGeneration` (int64)

Most recent generation observed by the controller.

### Validation Status

#### `validation` (object)

MCP protocol validation status.

**Fields:**

##### `validation.state` (string)

Overall validation state.

**Valid Values:**
- `Pending` - Validation has not started
- `Validating` - Validation is in progress
- `Validated` - Validation succeeded, server is compliant
- `AuthRequired` - Server requires authentication
- `Failed` - Validation ran but found issues
- `Disabled` - User disabled validation

##### `validation.compliant` (bool)

Whether the server is protocol compliant.

##### `validation.protocol` (string)

Detected MCP protocol variant (e.g., `streamable-http`, `sse`).

##### `validation.protocolVersion` (string)

Detected MCP specification version (e.g., `2024-11-05`, `2025-03-26`).

##### `validation.endpoint` (string)

Full URL that was validated.

##### `validation.requiresAuth` (bool)

Whether the server requires authentication.

##### `validation.capabilities` ([]string)

Capabilities discovered from the server (e.g., `["tools", "resources", "prompts"]`).

##### `validation.attempts` (int32)

Number of validation attempts made.

##### `validation.lastAttemptTime` (timestamp)

Timestamp of the last validation attempt.

##### `validation.lastValidated` (timestamp)

Timestamp of the last successful validation.

##### `validation.validatedGeneration` (int64)

Generation of the MCPServer that was validated.

##### `validation.issues` ([]object)

Validation issues found (if any).

**Issue Fields:**
- `level` (string) - Severity: `error`, `warning`, `info`
- `message` (string) - Human-readable description
- `code` (string) - Machine-readable issue code

**Example Status:**

```yaml
status:
  phase: Running
  replicas: 3
  readyReplicas: 3
  availableReplicas: 3
  serviceEndpoint: "http://my-mcp-server.default.svc:8080/mcp"
  transportType: http
  validation:
    state: Validated
    compliant: true
    protocol: "streamable-http"
    protocolVersion: "2025-03-26"
    endpoint: "http://my-mcp-server.default.svc:8080/mcp"
    requiresAuth: false
    capabilities: ["tools", "resources", "prompts"]
    lastValidated: "2025-12-03T10:00:00Z"
    validatedGeneration: 1
```

### Conditions

#### `conditions` ([]object)

Detailed status conditions following Kubernetes conventions.

**Condition Types:**
- `Ready` - MCP server is ready to serve traffic
- `Available` - MCP server has minimum required replicas available
- `Progressing` - MCP server is progressing towards desired state
- `Degraded` - MCP server is in a degraded state
- `Reconciled` - MCP server has been successfully reconciled

**Condition Fields:**
- `type` (string) - Condition type
- `status` (string) - `True`, `False`, or `Unknown`
- `lastTransitionTime` (timestamp) - When the condition last changed
- `reason` (string) - One-word CamelCase reason for transition
- `message` (string) - Human-readable message about the transition

## Security Defaults

The operator automatically applies secure defaults compliant with Kubernetes [Pod Security Standards (Restricted)](https://kubernetes.io/docs/concepts/security/pod-security-standards/) when `spec.security` is not specified.

**Default Security Context:**

```yaml
security:
  runAsNonRoot: true           # Containers must run as non-root
  runAsUser: 1000              # Default non-root user ID
  runAsGroup: 1000             # Default group ID
  fsGroup: 1000                # File system group for volume permissions
  allowPrivilegeEscalation: false  # No privilege escalation
```

**Additional Container Security:**
- All Linux capabilities are dropped (`capabilities: drop: ["ALL"]`)
- Default seccomp profile is applied (`seccompProfile: RuntimeDefault`)

**Overriding Defaults:**

You only need to specify security settings if you want to override the defaults. Partial configurations are supported - unspecified fields will use the secure defaults.

```yaml
spec:
  security:
    runAsUser: 2000              # Override default user
    readOnlyRootFilesystem: true # Add read-only root filesystem
    # Other fields use secure defaults
```

## See Also

- [Configuration Guide](configuration-guide.md) - Configuration patterns and examples
- [Environment Variables Guide](environment-variables.md) - Environment variable configuration
- [Validation Behavior Guide](validation-behavior.md) - Protocol validation deep dive
- [Configuration Examples](../config/samples/) - Real-world YAML examples
