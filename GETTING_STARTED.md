# Getting Started with MCP Operator

This guide will get you up and running with MCP Operator in about 5 minutes. Let's go!

## What You'll Need

- **A Kubernetes cluster** - Any of these will work:
  - [Kind](https://kind.sigs.k8s.io/) (great for local testing)
  - [Minikube](https://minikube.sigs.k8s.io/) (another good local option)
  - Any cloud cluster (GKE, EKS, AKS, etc.)
- **kubectl** - [Install it here](https://kubernetes.io/docs/tasks/tools/) if you don't have it

### Quick Cluster Setup

Don't have a cluster yet? Here's the fastest way to get one:

**Using Kind (recommended for testing):**

```bash
# Install Kind
brew install kind  # macOS
# or download from https://kind.sigs.k8s.io/docs/user/quick-start/

# Create a cluster
kind create cluster --name mcp-demo
```

**Using Minikube:**

```bash
# Install Minikube
brew install minikube  # macOS
# or download from https://minikube.sigs.k8s.io/docs/start/

# Start a cluster
minikube start
```

## Step 1: Install the Operator

Choose your preferred installation method. Both options work the same - **use Helm for easier customization**.

### Option A: Install with Helm

Install MCP Operator using Helm (automatically installs the latest version):

```bash
helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --namespace mcp-operator-system \
  --create-namespace
```

Wait for it to be ready:

```bash
kubectl wait --for=condition=available --timeout=300s \
  deployment/mcp-operator-controller-manager \
  -n mcp-operator-system
```

Verify it's running:

```bash
kubectl get pods -n mcp-operator-system
```

### Option B: Install with kubectl

Install MCP Operator using kubectl:

```bash
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

Wait for it to be ready:

```bash
kubectl wait --for=condition=available --timeout=300s \
  deployment/mcp-operator-controller-manager \
  -n mcp-operator-system
```

Verify it's running:

```bash
kubectl get pods -n mcp-operator-system
```

### Verification

You should see something like:

```
NAME                                               READY   STATUS    RESTARTS   AGE
mcp-operator-controller-manager-xxxxxxxxxx-xxxxx   2/2     Running   0          30s
```

That's it! The operator is now watching for MCPServers in your cluster.

## Step 2: Create Your First MCP Server

Let's deploy the Wikipedia MCP server. Create a file called `wikipedia.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: wikipedia
spec:
  image: "mcp/wikipedia-mcp:latest"
  args: ["--transport", "sse", "--port", "3001", "--host", "0.0.0.0"]

  transport:
    type: http
    protocol: auto  # The operator will auto-detect the protocol
    config:
      http:
        port: 3001
        path: "/sse"
```

Apply it:

```bash
kubectl apply -f wikipedia.yaml
```

The operator needs a minute to start the server and validate it. Watch it in real-time:

```bash
kubectl get mcpservers -w
```

You'll see the progression:

```
NAME        PHASE      REPLICAS   READY   PROTOCOL   VALIDATION   CAPABILITIES                      AGE
wikipedia   Creating   0          0                   Pending                                        2s
wikipedia   Creating   1          0                   Pending                                        5s
wikipedia   Running    1          1                   Validating                                     15s
wikipedia   Running    1          1       sse        Validated    ["tools","resources","prompts"]   25s
```

What's happening here?
- **Creating** - Kubernetes is starting the pod (Validation: Pending)
- **Running** (Validating) - Server is up, operator is validating it
- **Running** (Validated) - ✅ Validation complete, protocol detected!

Press Ctrl+C to stop watching.

Get more details:

```bash
kubectl describe mcpserver wikipedia
```

Check the validation status:

```bash
kubectl get mcpserver wikipedia -o jsonpath='{.status.validation}' | jq
```

You'll see:

```json
{
  "state": "Validated",
  "compliant": true,
  "protocol": "sse",
  "protocolVersion": "2024-11-05",
  "endpoint": "http://wikipedia.default.svc:3001/sse",
  "requiresAuth": false,
  "capabilities": ["tools", "resources", "prompts"],
  "lastValidated": "2025-11-29T10:30:00Z",
  "validatedGeneration": 1
}
```

## Step 4: Access Your Server

The operator created a Kubernetes service for your MCP server. Here's how to access it:

**If using Kind or a cloud cluster:**

```bash
# Forward the port to your local machine
kubectl port-forward service/wikipedia 3001:3001
```

**If using Minikube:**

```bash
# Minikube can open it for you
minikube service wikipedia --url
```

Your MCP server is now accessible at `http://localhost:3001/sse`!

## Step 5: Test It

You can test your MCP server using the [MCP Inspector](https://github.com/modelcontextprotocol/inspector):

```bash
# Install MCP Inspector
npm install -g @modelcontextprotocol/inspector

# Connect to your server
mcp-inspector http://localhost:3001/sse
```

This opens a web interface where you can:
- Test the server's tools and prompts
- See what capabilities it has
- Send requests and see responses

## What's Next?

### Add Monitoring

If you have Prometheus Operator installed, enable monitoring:

**With Helm (if you used Helm to install):**
```bash
helm upgrade mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
  --namespace mcp-operator-system \
  --reuse-values \
  --set prometheus.enable=true \
  --set grafana.enabled=true
```

**With kubectl (if you used kubectl to install):**
```bash
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

This adds a Grafana dashboard showing all your MCP servers' health and performance.

### Explore More Examples

Check out the sample configurations:

```bash
# Strict mode enabled
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/config/samples/wikipedia-http.yaml

# mcp-everything-server - MCP with all capabilities
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/config/samples/mcp-basic-example.yaml
```

## Troubleshooting

### Server stuck in "Creating" phase?

Check the pods:

```bash
kubectl get pods
kubectl describe pod -l app.kubernetes.io/instance=wikipedia
```

### Check the logs:

```bash
kubectl logs -l app.kubernetes.io/instance=wikipedia
```

### Validation failing?

See what's wrong:

```bash
kubectl get mcpserver wikipedia -o jsonpath='{.status.validation.issues}' | jq
```

Common issues:
- **Wrong port** - Make sure the port in your config matches what the server listens on
- **Image not found** - Check that the image exists and is accessible
- **Protocol mismatch** - The operator will detect this and suggest using `auto`

### Still stuck?

The operator logs can help:

```bash
kubectl logs -n mcp-operator-system \
  deployment/mcp-operator-controller-manager \
  -c manager
```

## Clean Up

Remove your MCP server:

```bash
kubectl delete mcpserver wikipedia
```

Uninstall the operator:

**If you used Helm:**
```bash
helm uninstall mcp-operator --namespace mcp-operator-system
```

**If you used kubectl:**
```bash
kubectl delete -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

Delete your local cluster:

```bash
# Kind
kind delete cluster --name mcp-demo

# Minikube
minikube delete
```

## Need Help?

- **Questions?** [Start a discussion](https://github.com/vitorbari/mcp-operator/discussions)
- **Found a bug?** [Open an issue](https://github.com/vitorbari/mcp-operator/issues)
- **Want to contribute?** Check out [CONTRIBUTING.md](CONTRIBUTING.md)

## Learn More

- [Full Documentation](README.md)
- [Configuration Examples](config/samples/)
- [MCP Protocol Specification](https://modelcontextprotocol.io)

That's it! You've successfully deployed and validated your first MCP server on Kubernetes. Welcome aboard! 🎉
