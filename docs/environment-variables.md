# Environment Variables Guide

This guide covers how to configure environment variables for your MCP servers running in Kubernetes with the MCP Operator.

## Table of Contents

- [Overview](#overview)
- [Basic Configuration](#basic-configuration)
- [Using ConfigMaps](#using-configmaps)
- [Using Secrets](#using-secrets)
- [Volume Mounts for Configuration Files](#volume-mounts-for-configuration-files)
- [Common Environment Variables](#common-environment-variables)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

Environment variables are configured through the `spec.environment` field in your MCPServer resource. This field accepts standard Kubernetes `EnvVar` objects, giving you flexibility in how you provide configuration to your MCP servers.

## Basic Configuration

The simplest way to configure environment variables is to specify them directly in the MCPServer spec.

### Direct Value Specification

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  environment:
    - name: LOG_LEVEL
      value: "info"
    - name: MCP_PORT
      value: "8080"
    - name: METRICS_ENABLED
      value: "true"
    - name: ENVIRONMENT
      value: "production"
```

### Field References

You can reference pod or container fields using `fieldRef`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  environment:
    - name: POD_NAME
      valueFrom:
        fieldRef:
          fieldPath: metadata.name
    - name: POD_NAMESPACE
      valueFrom:
        fieldRef:
          fieldPath: metadata.namespace
    - name: POD_IP
      valueFrom:
        fieldRef:
          fieldPath: status.podIP
    - name: NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
```

### Resource Field References

Reference container resource limits or requests:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  resources:
    requests:
      memory: "256Mi"
      cpu: "200m"
    limits:
      memory: "1Gi"
      cpu: "1000m"

  environment:
    - name: MEMORY_LIMIT
      valueFrom:
        resourceFieldRef:
          containerName: mcp-server
          resource: limits.memory
          divisor: "1Mi"
    - name: CPU_REQUEST
      valueFrom:
        resourceFieldRef:
          containerName: mcp-server
          resource: requests.cpu
```

## Using ConfigMaps

ConfigMaps are ideal for non-sensitive configuration data. They allow you to manage configuration separately from your MCPServer definition.

### Create a ConfigMap

First, create a ConfigMap with your configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mcp-server-config
data:
  log_level: "info"
  max_connections: "100"
  timeout: "30s"
  feature_flags: "feature1,feature2,feature3"
```

Apply it:

```bash
kubectl apply -f configmap.yaml
```

### Reference ConfigMap Values

Reference individual keys from the ConfigMap:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  environment:
    - name: LOG_LEVEL
      valueFrom:
        configMapKeyRef:
          name: mcp-server-config
          key: log_level
    - name: MAX_CONNECTIONS
      valueFrom:
        configMapKeyRef:
          name: mcp-server-config
          key: max_connections
    - name: TIMEOUT
      valueFrom:
        configMapKeyRef:
          name: mcp-server-config
          key: timeout
```

### Import All ConfigMap Keys

Import all keys from a ConfigMap as environment variables:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  podTemplate:
    # Note: envFrom is not directly supported in spec.environment,
    # but you can use podTemplate for advanced scenarios
    # This requires using the pod spec directly
```

**Note:** The `spec.environment` field only supports individual `EnvVar` entries. For bulk import of ConfigMap or Secret keys, you would need to list them individually or consider using init containers or configuration files mounted as volumes.

## Using Secrets

Secrets are designed for sensitive data like API keys, passwords, and tokens. They provide better security than ConfigMaps.

### Create a Secret

Create a Secret with your sensitive data:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mcp-server-secrets
type: Opaque
stringData:
  api-key: "super-secret-api-key"
  db-password: "database-password"
  oauth-token: "oauth-token-value"
```

Apply it:

```bash
kubectl apply -f secret.yaml
```

Or create it from literals:

```bash
kubectl create secret generic mcp-server-secrets \
  --from-literal=api-key=super-secret-api-key \
  --from-literal=db-password=database-password
```

### Reference Secret Values

Reference individual keys from the Secret:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  environment:
    - name: API_KEY
      valueFrom:
        secretKeyRef:
          name: mcp-server-secrets
          key: api-key
    - name: DB_PASSWORD
      valueFrom:
        secretKeyRef:
          name: mcp-server-secrets
          key: db-password
    - name: OAUTH_TOKEN
      valueFrom:
        secretKeyRef:
          name: mcp-server-secrets
          key: oauth-token
```

### Optional References

Make references optional (pod starts even if key is missing):

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  environment:
    - name: OPTIONAL_API_KEY
      valueFrom:
        secretKeyRef:
          name: mcp-server-secrets
          key: optional-api-key
          optional: true
```

## Volume Mounts for Configuration Files

For complex configurations or when your application expects configuration files, use volume mounts.

### ConfigMap as Volume

Mount an entire ConfigMap as files:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mcp-server-files
data:
  config.json: |
    {
      "server": {
        "host": "0.0.0.0",
        "port": 8080
      },
      "logging": {
        "level": "info"
      }
    }
  rules.yaml: |
    rules:
      - name: rule1
        enabled: true
---
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  # Point to the config file location
  environment:
    - name: CONFIG_FILE
      value: "/etc/mcp/config/config.json"

  podTemplate:
    volumes:
      - name: config-volume
        configMap:
          name: mcp-server-files

    volumeMounts:
      - name: config-volume
        mountPath: /etc/mcp/config
        readOnly: true
```

### Secret as Volume

Mount sensitive configuration files:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mcp-server-certs
type: Opaque
stringData:
  tls.crt: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  tls.key: |
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
---
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  environment:
    - name: TLS_CERT_PATH
      value: "/etc/mcp/certs/tls.crt"
    - name: TLS_KEY_PATH
      value: "/etc/mcp/certs/tls.key"

  podTemplate:
    volumes:
      - name: certs-volume
        secret:
          secretName: mcp-server-certs
          defaultMode: 0400  # Read-only for owner

    volumeMounts:
      - name: certs-volume
        mountPath: /etc/mcp/certs
        readOnly: true
```

### Specific File from ConfigMap/Secret

Mount only specific keys as files:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
spec:
  image: "myregistry/mcp-server:latest"

  podTemplate:
    volumes:
      - name: config-volume
        configMap:
          name: mcp-server-files
          items:
            - key: config.json
              path: config.json

    volumeMounts:
      - name: config-volume
        mountPath: /etc/mcp/config
        readOnly: true
```

## Common Environment Variables

While environment variables are application-specific, here are common patterns for MCP servers:

### Server Configuration

```yaml
environment:
  # Server binding
  - name: MCP_HOST
    value: "0.0.0.0"
  - name: MCP_PORT
    value: "8080"

  # Transport configuration
  - name: MCP_TRANSPORT
    value: "sse"  # or "streamable-http"
  - name: MCP_PATH
    value: "/mcp"
```

### Logging

```yaml
environment:
  # Log level
  - name: LOG_LEVEL
    value: "info"  # debug, info, warn, error

  # Log format
  - name: LOG_FORMAT
    value: "json"  # json, text

  # Log output
  - name: LOG_OUTPUT
    value: "stdout"
```

### Feature Flags

```yaml
environment:
  # Enable/disable features
  - name: ENABLE_METRICS
    value: "true"
  - name: ENABLE_TRACING
    value: "false"
  - name: ENABLE_CACHING
    value: "true"

  # Feature configuration
  - name: CACHE_TTL
    value: "3600"
  - name: MAX_CACHE_SIZE
    value: "1000"
```

### Performance Tuning

```yaml
environment:
  # Connection limits
  - name: MAX_CONNECTIONS
    value: "100"
  - name: CONNECTION_TIMEOUT
    value: "30s"

  # Worker configuration
  - name: WORKER_THREADS
    value: "4"
  - name: QUEUE_SIZE
    value: "1000"
```

### Environment Identification

```yaml
environment:
  # Environment type
  - name: ENVIRONMENT
    value: "production"  # development, staging, production

  # Deployment metadata
  - name: VERSION
    value: "1.0.0"
  - name: BUILD_ID
    value: "abc123"
  - name: DEPLOYMENT_ID
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
```

## Best Practices

### 1. Use Secrets for Sensitive Data

Always use Secrets for sensitive information:

```yaml
# ✅ Good - Using Secret
environment:
  - name: API_KEY
    valueFrom:
      secretKeyRef:
        name: mcp-secrets
        key: api-key

# ❌ Bad - Plaintext sensitive data
environment:
  - name: API_KEY
    value: "super-secret-key"
```

### 2. Separate Configuration by Environment

Create separate ConfigMaps for different environments:

```bash
# Development
kubectl create configmap mcp-config-dev \
  --from-literal=log_level=debug \
  --from-literal=cache_enabled=false

# Production
kubectl create configmap mcp-config-prod \
  --from-literal=log_level=warn \
  --from-literal=cache_enabled=true
```

Then reference the appropriate ConfigMap in each environment.

### 3. Use Descriptive Names

Choose clear, descriptive environment variable names:

```yaml
# ✅ Good
environment:
  - name: DATABASE_CONNECTION_TIMEOUT_SECONDS
    value: "30"

# ❌ Less clear
environment:
  - name: DB_TIMEOUT
    value: "30"
```

### 4. Document Required Variables

Document which environment variables are required vs. optional in your application documentation.

### 5. Validate Configuration

Have your application validate configuration on startup and fail fast if required variables are missing or invalid.

### 6. Use Read-Only Mounts

Mount configuration volumes as read-only when possible:

```yaml
podTemplate:
  volumeMounts:
    - name: config-volume
      mountPath: /etc/mcp/config
      readOnly: true  # ✅ Prevents accidental modification
```

### 7. Avoid Hardcoding Defaults

Let your application handle defaults rather than setting them all in the MCPServer spec:

```yaml
# ✅ Good - Only override what's necessary
environment:
  - name: LOG_LEVEL
    value: "info"

# ❌ Unnecessary - Let app use its defaults
environment:
  - name: LOG_LEVEL
    value: "info"
  - name: LOG_FORMAT
    value: "json"
  - name: LOG_COLOR
    value: "false"
  - name: LOG_TIMESTAMPS
    value: "true"
```

### 8. Use Namespaces for Organization

Organize resources by namespace:

```bash
# Create namespace
kubectl create namespace mcp-production

# Create ConfigMap in namespace
kubectl create configmap mcp-config \
  --namespace mcp-production \
  --from-literal=log_level=info

# Reference in MCPServer (in same namespace)
```

## Troubleshooting

### Environment Variable Not Set

**Symptom:** Application can't find expected environment variable.

**Check if the variable is defined:**

```bash
kubectl get mcpserver my-mcp-server -o jsonpath='{.spec.environment}'
```

**Check the actual pod environment:**

```bash
# Get pod name
POD_NAME=$(kubectl get pods -l app.kubernetes.io/instance=my-mcp-server -o jsonpath='{.items[0].metadata.name}')

# Check environment variables
kubectl exec $POD_NAME -- env | grep MY_VAR
```

### ConfigMap/Secret Not Found

**Symptom:** Pod fails to start with error about missing ConfigMap or Secret.

**Verify the resource exists:**

```bash
kubectl get configmap mcp-server-config
kubectl get secret mcp-server-secrets
```

**Check the namespace:**

```bash
# Resources must be in the same namespace as MCPServer
kubectl get configmap mcp-server-config -n my-namespace
```

**Solution:** Create the missing resource or fix the name reference.

### Wrong ConfigMap/Secret Key

**Symptom:** Pod fails to start or variable has wrong value.

**Check available keys:**

```bash
# ConfigMap
kubectl describe configmap mcp-server-config

# Secret
kubectl describe secret mcp-server-secrets
kubectl get secret mcp-server-secrets -o jsonpath='{.data}' | jq
```

**Solution:** Use the correct key name in your `configMapKeyRef` or `secretKeyRef`.

### Volume Mount Issues

**Symptom:** Application can't read configuration file.

**Check if volume is mounted:**

```bash
# Describe the pod
POD_NAME=$(kubectl get pods -l app.kubernetes.io/instance=my-mcp-server -o jsonpath='{.items[0].metadata.name}')
kubectl describe pod $POD_NAME

# Check mounts
kubectl exec $POD_NAME -- ls -la /etc/mcp/config
```

**Check file permissions:**

```bash
# If using read-only root filesystem
kubectl exec $POD_NAME -- ls -la /etc/mcp/config
```

**Solution:** Verify volume and volumeMount configuration, check that paths match, and ensure proper permissions.

### Changes Not Reflected

**Symptom:** Updated ConfigMap or Secret but pod still has old values.

**Why:** Pods don't automatically restart when ConfigMap/Secret changes.

**Solutions:**

1. **Manually restart pods:**
   ```bash
   kubectl rollout restart deployment -l app.kubernetes.io/instance=my-mcp-server
   ```

2. **Update MCPServer spec to trigger recreation:**
   ```bash
   kubectl patch mcpserver my-mcp-server --type='json' \
     -p='[{"op": "add", "path": "/spec/podTemplate/annotations/restartedAt", "value":"'$(date +%s)'"}]'
   ```

3. **Use a tool like Reloader:** Auto-restarts deployments when ConfigMaps/Secrets change.

### Debugging Environment Variables

**View all environment variables in a running pod:**

```bash
POD_NAME=$(kubectl get pods -l app.kubernetes.io/instance=my-mcp-server -o jsonpath='{.items[0].metadata.name}')
kubectl exec $POD_NAME -- env | sort
```

**Check logs for configuration errors:**

```bash
kubectl logs $POD_NAME | grep -i "config\|environment\|variable"
```

**Test with a different value:**

```bash
# Temporarily update MCPServer
kubectl edit mcpserver my-mcp-server

# Or patch it
kubectl patch mcpserver my-mcp-server --type='json' \
  -p='[{"op": "replace", "path": "/spec/environment/0/value", "value":"debug"}]'
```

## See Also

- [API Reference](api-reference.md) - Complete field documentation
- [Configuration Guide](configuration-guide.md) - Configuration patterns and examples
- [Troubleshooting Guide](troubleshooting.md) - Common issues and solutions
- [Kubernetes ConfigMaps](https://kubernetes.io/docs/concepts/configuration/configmap/)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
