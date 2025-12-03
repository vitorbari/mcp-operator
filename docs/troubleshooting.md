# Troubleshooting Guide

Common issues and solutions for the MCP Operator.

## Table of Contents

- [Quick Diagnostics](#quick-diagnostics)
- [Installation Issues](#installation-issues)
- [Deployment Issues](#deployment-issues)
- [Validation Issues](#validation-issues)
- [Scaling Issues](#scaling-issues)
- [Resource Issues](#resource-issues)
- [Configuration Issues](#configuration-issues)
- [Networking Issues](#networking-issues)
- [Debug Mode](#debug-mode)
- [Getting Help](#getting-help)

## Quick Diagnostics

Before diving into specific issues, run these commands to gather information:

### Check MCPServer Status

```bash
# List all MCPServers
kubectl get mcpservers --all-namespaces

# Get detailed status
kubectl get mcpserver <name> -o yaml

# Watch status changes in real-time
kubectl get mcpservers -w
```

### Check Pods

```bash
# List pods for your MCPServer
kubectl get pods -l app.kubernetes.io/instance=<mcpserver-name>

# Describe pod for events
kubectl describe pod <pod-name>

# Check pod logs
kubectl logs <pod-name>

# Check previous pod logs (if pod restarted)
kubectl logs <pod-name> --previous
```

### Check Validation Status

```bash
# Get validation details
kubectl get mcpserver <name> -o jsonpath='{.status.validation}' | jq

# Check for validation issues
kubectl get mcpserver <name> -o jsonpath='{.status.validation.issues}' | jq
```

### Check Operator Logs

```bash
# Get operator pod name
OPERATOR_POD=$(kubectl get pods -n mcp-operator-system -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}')

# View operator logs
kubectl logs -n mcp-operator-system $OPERATOR_POD -c manager

# Follow logs in real-time
kubectl logs -n mcp-operator-system $OPERATOR_POD -c manager -f
```

## Installation Issues

### Operator Not Installing

**Symptom:** Installation command fails or operator pod doesn't start.

**Check installation:**

```bash
kubectl get pods -n mcp-operator-system
kubectl get deployment -n mcp-operator-system
```

**Common causes:**

1. **Network issues downloading manifests**

   ```bash
   # Download manifest first
   curl -O https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml

   # Inspect and apply
   kubectl apply -f install.yaml
   ```

2. **Insufficient permissions**

   ```bash
   # Check if you can create namespaces
   kubectl auth can-i create namespaces

   # Check if you can create CRDs
   kubectl auth can-i create customresourcedefinitions
   ```

3. **Resource constraints**

   ```bash
   # Check node resources
   kubectl top nodes

   # Check if pods are pending
   kubectl get pods -n mcp-operator-system
   kubectl describe pod <operator-pod-name> -n mcp-operator-system
   ```

**Solution:**

```bash
# Ensure you have cluster-admin permissions
# Or ask your cluster administrator to install the operator

# Verify installation completed
kubectl wait --for=condition=available --timeout=300s \
  deployment/mcp-operator-controller-manager \
  -n mcp-operator-system
```

### CRDs Not Being Created

**Symptom:** MCPServer resources not recognized.

**Check CRDs:**

```bash
kubectl get crd mcpservers.mcp.mcp-operator.io
```

**If CRD doesn't exist:**

```bash
# Reapply installation manifest
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml

# Or install CRDs separately
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/config/crd/bases/mcp.mcp-operator.io_mcpservers.yaml
```

### Operator Crashing

**Symptom:** Operator pod in CrashLoopBackOff.

**Check logs:**

```bash
kubectl logs -n mcp-operator-system \
  deployment/mcp-operator-controller-manager \
  -c manager
```

**Common causes:**

1. **Certificate issues** - Webhook certificates not configured
2. **RBAC issues** - Missing permissions
3. **Resource limits** - Operator OOMKilled

**Solution:**

```bash
# Check events
kubectl describe deployment -n mcp-operator-system mcp-operator-controller-manager

# Verify RBAC
kubectl get clusterrole mcp-operator-manager-role
kubectl get clusterrolebinding mcp-operator-manager-rolebinding
```

## Deployment Issues

### Server Stuck in "Creating" Phase

**Symptom:** MCPServer phase stays "Creating" for a long time.

**Check pod status:**

```bash
kubectl get pods -l app.kubernetes.io/instance=<mcpserver-name>
kubectl describe pod <pod-name>
```

**Common causes:**

#### 1. Image Pull Errors

```bash
# Look for ImagePullBackOff or ErrImagePull
kubectl get pods -l app.kubernetes.io/instance=<mcpserver-name>
```

**Solutions:**

```bash
# Verify image exists
docker pull <image-name>

# Check image pull secrets
kubectl get mcpserver <name> -o jsonpath='{.spec.podTemplate.imagePullSecrets}'

# Create image pull secret if needed
kubectl create secret docker-registry registry-credentials \
  --docker-server=<registry-url> \
  --docker-username=<username> \
  --docker-password=<password>
```

#### 2. Insufficient Resources

```bash
# Check if pod is pending
kubectl get pods -l app.kubernetes.io/instance=<mcpserver-name>

# Check events
kubectl describe pod <pod-name> | grep -A 5 Events
```

**Solution:**

```yaml
# Reduce resource requests
spec:
  resources:
    requests:
      cpu: "100m"      # Reduced from 200m
      memory: "128Mi"  # Reduced from 256Mi
```

#### 3. Node Selector/Affinity Issues

```bash
# Check if nodes match selectors
kubectl get nodes --show-labels

# Check pod events
kubectl describe pod <pod-name>
```

**Solution:**

```bash
# Remove or update node selectors
kubectl edit mcpserver <name>

# Or label nodes to match
kubectl label nodes <node-name> disktype=ssd
```

#### 4. Container Crash Loop

```bash
# Check container logs
kubectl logs <pod-name>

# Check previous logs if pod restarted
kubectl logs <pod-name> --previous
```

**Solution:** Fix application issues preventing startup.

### Server in "Failed" Phase

**Symptom:** MCPServer phase is "Failed".

**Check status message:**

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.message}'
```

**Check operator logs:**

```bash
kubectl logs -n mcp-operator-system \
  deployment/mcp-operator-controller-manager \
  -c manager | grep <mcpserver-name>
```

**Common causes:**

1. **Deployment creation failed**
2. **Service creation failed**
3. **RBAC issues**
4. **Validation failed in strict mode**

**Solution:**

```bash
# Check events
kubectl get events --field-selector involvedObject.name=<mcpserver-name>

# Fix underlying issue and update MCPServer
kubectl edit mcpserver <name>
```

### Server Running but Not Accessible

**Symptom:** Pods are running but service doesn't respond.

**Check service:**

```bash
kubectl get svc <mcpserver-name>
kubectl describe svc <mcpserver-name>
```

**Test connectivity:**

```bash
# From within cluster
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  wget -O- http://<mcpserver-name>:8080/mcp

# Port forward to test locally
kubectl port-forward svc/<mcpserver-name> 8080:8080

# Test: curl http://localhost:8080/mcp
```

**Common causes:**

#### 1. Wrong Port Configuration

```bash
# Check service ports
kubectl get svc <mcpserver-name> -o jsonpath='{.spec.ports}'

# Check container port
kubectl get pod <pod-name> -o jsonpath='{.spec.containers[0].ports}'
```

**Solution:**

```yaml
spec:
  transport:
    config:
      http:
        port: 8080  # Must match what server listens on
  service:
    port: 8080
    targetPort: 8080
```

#### 2. Health Check Failing

```bash
# Check pod readiness
kubectl get pods -l app.kubernetes.io/instance=<mcpserver-name>

# Check readiness probe
kubectl describe pod <pod-name> | grep -A 5 Readiness
```

**Solution:**

```yaml
spec:
  healthCheck:
    enabled: true
    path: "/health"  # Ensure this endpoint exists
    port: 8080
```

#### 3. Network Policy Blocking Traffic

```bash
# Check network policies
kubectl get networkpolicy
kubectl describe networkpolicy <policy-name>
```

**Solution:** Update network policies to allow traffic.

## Validation Issues

### Validation State "AuthRequired"

**Symptom:** Validation status shows `AuthRequired`.

**Meaning:** Server requires authentication but validator doesn't have credentials.

**Check validation status:**

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.validation}' | jq
```

**Solutions:**

1. **Disable validation if auth is expected:**

   ```yaml
   spec:
     validation:
       enabled: false
   ```

2. **Configure auth for validation** (future feature)

3. **Use strict mode to fail if auth required:**

   ```yaml
   spec:
     validation:
       enabled: true
       strictMode: true
   ```

### Validation State "Failed"

**Symptom:** Validation status shows `Failed`.

**Check validation issues:**

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.validation.issues}' | jq
```

**Common causes:**

#### 1. Wrong Protocol

```yaml
# Issue: Server uses SSE but spec says streamable-http
status:
  validation:
    issues:
      - level: error
        message: "Server does not support streamable-http protocol"
```

**Solution:**

```yaml
spec:
  transport:
    protocol: auto  # Let operator detect
    # or
    protocol: sse  # Explicitly use SSE
```

#### 2. Wrong Port/Path

**Solution:**

```yaml
spec:
  transport:
    config:
      http:
        port: 3001       # Match server's port
        path: "/sse"     # Match server's path
```

#### 3. Server Not Ready

**Solution:** Wait for server to fully start, then check validation again:

```bash
kubectl get mcpserver <name> -w
```

### Required Capabilities Not Met

**Symptom:** Validation fails because server lacks required capabilities.

**Check capabilities:**

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.validation.capabilities}'
```

**Solution:**

```yaml
spec:
  validation:
    requiredCapabilities:
      - "tools"
      # Remove capabilities your server doesn't support
      # - "resources"
      # - "prompts"
```

### Strict Mode Failures

**Symptom:** Server immediately goes to "ValidationFailed" phase.

**Check validation:**

```bash
kubectl get mcpserver <name> -o jsonpath='{.status.validation}' | jq
```

**Solution:**

1. **Fix validation issues** (see above)

2. **Disable strict mode temporarily:**

   ```yaml
   spec:
     validation:
       strictMode: false  # Allow deployment even if validation fails
   ```

## Scaling Issues

### HPA Not Working

**Symptom:** Replicas don't scale despite CPU/memory usage.

**Check HPA status:**

```bash
kubectl get hpa <mcpserver-name>-hpa
kubectl describe hpa <mcpserver-name>-hpa
```

**Common causes:**

#### 1. Metrics Server Not Installed

```bash
# Check if metrics-server is running
kubectl get deployment metrics-server -n kube-system
```

**Solution:**

```bash
# Install metrics-server
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

#### 2. No Resource Requests Set

HPA requires resource requests to calculate utilization:

```yaml
spec:
  resources:
    requests:  # Required for HPA
      cpu: "200m"
      memory: "256Mi"
```

#### 3. Metrics Not Available

```bash
# Check if metrics are available
kubectl top pods -l app.kubernetes.io/instance=<mcpserver-name>
```

**Solution:** Wait a few minutes for metrics to be collected.

### Replicas Not Scaling Down

**Symptom:** HPA scales up but never scales down.

**Check scale-down behavior:**

```bash
kubectl get hpa <mcpserver-name>-hpa -o yaml
```

**Possible causes:**

1. **Scale-down stabilization window**
2. **Scale-down policies too conservative**

**Solution:**

```yaml
spec:
  hpa:
    scaleDownBehavior:
      stabilizationWindowSeconds: 60  # Reduced from 300
      policies:
        - type: "Pods"
          value: 2  # Scale down faster
          periodSeconds: 30
```

### Replicas Stuck at Minimum

**Symptom:** HPA stays at minReplicas.

**Check current vs desired:**

```bash
kubectl get hpa <mcpserver-name>-hpa
```

**Possible causes:**

1. **CPU/Memory below target** - This is normal, HPA maintains minimum
2. **Metrics not available**

**Verify metrics:**

```bash
kubectl top pods -l app.kubernetes.io/instance=<mcpserver-name>
```

## Resource Issues

### Pods Being OOMKilled

**Symptom:** Pods restart with OOMKilled status.

**Check events:**

```bash
kubectl describe pod <pod-name> | grep -A 5 "Last State"
```

**Solution:**

```yaml
spec:
  resources:
    requests:
      memory: "512Mi"  # Increased
    limits:
      memory: "2Gi"    # Increased
```

**Investigate memory usage:**

```bash
# Monitor memory in real-time
kubectl top pods -l app.kubernetes.io/instance=<mcpserver-name>

# Check container memory
kubectl exec <pod-name> -- cat /sys/fs/cgroup/memory/memory.usage_in_bytes
```

### CPU Throttling

**Symptom:** Application runs slowly despite CPU limit not being reached.

**Check CPU usage:**

```bash
kubectl top pods -l app.kubernetes.io/instance=<mcpserver-name>
```

**Solution:**

```yaml
spec:
  resources:
    limits:
      cpu: "2000m"  # Increased from 1000m
```

**Investigate:**

```bash
# Check CPU throttling stats
kubectl exec <pod-name> -- cat /sys/fs/cgroup/cpu/cpu.stat
```

### Disk Pressure

**Symptom:** Pods evicted due to disk pressure.

**Check node condition:**

```bash
kubectl describe node <node-name> | grep DiskPressure
```

**Solution:**

1. **Clean up node:**

   ```bash
   # SSH to node and clean up unused images
   docker system prune -a
   ```

2. **Use emptyDir with size limit:**

   ```yaml
   spec:
     podTemplate:
       volumes:
         - name: tmp
           emptyDir:
             sizeLimit: 1Gi
   ```

## Configuration Issues

### Environment Variables Not Working

**Symptom:** Application can't find expected environment variables.

**Check environment:**

```bash
POD_NAME=$(kubectl get pods -l app.kubernetes.io/instance=<mcpserver-name> -o jsonpath='{.items[0].metadata.name}')
kubectl exec $POD_NAME -- env | grep <VAR_NAME>
```

**Check spec:**

```bash
kubectl get mcpserver <name> -o jsonpath='{.spec.environment}' | jq
```

**Solution:** See [Environment Variables Guide](environment-variables.md) for detailed troubleshooting.

### Volume Mounts Failing

**Symptom:** Pod fails to start with volume mount errors.

**Check events:**

```bash
kubectl describe pod <pod-name> | grep -A 10 Events
```

**Common causes:**

1. **ConfigMap/Secret doesn't exist**

   ```bash
   kubectl get configmap <name>
   kubectl get secret <name>
   ```

2. **Wrong namespace**

   ```bash
   # ConfigMap/Secret must be in same namespace as MCPServer
   kubectl get configmap <name> -n <namespace>
   ```

3. **Wrong mount path conflicts with read-only root**

   ```yaml
   spec:
     security:
       readOnlyRootFilesystem: true
     podTemplate:
       volumeMounts:
         - name: cache
           mountPath: /tmp/cache  # Must be writable volume
   ```

### ConfigMap/Secret Changes Not Reflected

**Symptom:** Updated ConfigMap but pod still has old values.

**Why:** Pods don't automatically restart when ConfigMaps/Secrets change.

**Solution:**

```bash
# Restart pods to pick up changes
kubectl rollout restart deployment -l app.kubernetes.io/instance=<mcpserver-name>

# Or trigger update via annotation
kubectl patch mcpserver <name> --type='json' \
  -p='[{"op": "add", "path": "/spec/podTemplate/annotations/restartedAt", "value":"'$(date +%s)'"}]'
```

## Networking Issues

### Service Not Accessible

**Symptom:** Can't reach service from within cluster.

**Test DNS:**

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup <mcpserver-name>
```

**Test connectivity:**

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  wget -O- http://<mcpserver-name>:8080/mcp
```

**Check service endpoints:**

```bash
kubectl get endpoints <mcpserver-name>
```

If no endpoints, pods aren't ready or labels don't match.

### Port Conflicts

**Symptom:** Service fails to bind to port.

**Check for conflicts:**

```bash
kubectl get svc --all-namespaces | grep <port-number>
```

**Solution:** Use a different port or namespace.

### DNS Resolution Issues

**Symptom:** Can't resolve service name.

**Check CoreDNS:**

```bash
kubectl get pods -n kube-system -l k8s-app=kube-dns
```

**Test DNS:**

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup kubernetes.default
```

## Debug Mode

### Enable Verbose Logging

**In MCPServer:**

```yaml
spec:
  environment:
    - name: LOG_LEVEL
      value: "debug"
```

**In Operator:**

```bash
# Edit operator deployment
kubectl edit deployment -n mcp-operator-system mcp-operator-controller-manager

# Add --zap-log-level=debug to manager container args
```

### Collect Diagnostics

**Gather all relevant information:**

```bash
#!/bin/bash
MCPSERVER_NAME="my-mcp-server"
OUTPUT_DIR="diagnostics-$(date +%Y%m%d-%H%M%S)"
mkdir -p $OUTPUT_DIR

# MCPServer resource
kubectl get mcpserver $MCPSERVER_NAME -o yaml > $OUTPUT_DIR/mcpserver.yaml

# Pods
kubectl get pods -l app.kubernetes.io/instance=$MCPSERVER_NAME -o yaml > $OUTPUT_DIR/pods.yaml

# Pod logs
for pod in $(kubectl get pods -l app.kubernetes.io/instance=$MCPSERVER_NAME -o jsonpath='{.items[*].metadata.name}'); do
  kubectl logs $pod > $OUTPUT_DIR/log-$pod.txt 2>&1
  kubectl logs $pod --previous > $OUTPUT_DIR/log-$pod-previous.txt 2>&1 || true
done

# Events
kubectl get events --field-selector involvedObject.name=$MCPSERVER_NAME > $OUTPUT_DIR/events.txt

# Service
kubectl get svc $MCPSERVER_NAME -o yaml > $OUTPUT_DIR/service.yaml

# Operator logs
kubectl logs -n mcp-operator-system deployment/mcp-operator-controller-manager -c manager > $OUTPUT_DIR/operator-logs.txt

# Create tarball
tar czf $OUTPUT_DIR.tar.gz $OUTPUT_DIR
echo "Diagnostics collected in $OUTPUT_DIR.tar.gz"
```

### Test Validation Manually

**Port forward to server:**

```bash
kubectl port-forward svc/<mcpserver-name> 8080:8080
```

**Test streamable-http:**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}'
```

**Test SSE:**

```bash
curl http://localhost:8080/sse
```

## Getting Help

### Before Asking for Help

1. **Check this troubleshooting guide**
2. **Search existing GitHub issues**
3. **Collect diagnostics** (see [Debug Mode](#debug-mode))

### Where to Get Help

**GitHub Issues:** [vitorbari/mcp-operator/issues](https://github.com/vitorbari/mcp-operator/issues)

Use for:
- Bug reports
- Feature requests
- Documentation issues

**GitHub Discussions:** [vitorbari/mcp-operator/discussions](https://github.com/vitorbari/mcp-operator/discussions)

Use for:
- Questions
- Ideas
- General discussion

### What to Include

When opening an issue, include:

1. **Environment:**
   - Kubernetes version: `kubectl version`
   - Operator version: `kubectl get deployment -n mcp-operator-system mcp-operator-controller-manager -o jsonpath='{.spec.template.spec.containers[0].image}'`
   - Cloud provider or bare metal

2. **MCPServer Configuration:**
   ```bash
   kubectl get mcpserver <name> -o yaml
   ```

3. **Current Status:**
   ```bash
   kubectl get mcpserver <name> -o jsonpath='{.status}' | jq
   ```

4. **Logs:**
   - Pod logs
   - Operator logs
   - Relevant events

5. **What you've tried:**
   - Steps to reproduce
   - Solutions attempted

## See Also

- [Configuration Guide](configuration-guide.md) - Configuration patterns
- [Environment Variables Guide](environment-variables.md) - Environment variable configuration
- [Monitoring Guide](monitoring.md) - Metrics and alerting
- [Validation Behavior](validation-behavior.md) - Protocol validation details
- [API Reference](api-reference.md) - Complete field documentation
