# Getting Started with MCP Operator

This guide will help you get the MCP Operator running in about 5 minutes.

## Prerequisites

Before you begin, you'll need:

- **Kubernetes cluster** - One of the following:
  - [Kind](https://kind.sigs.k8s.io/) (recommended for testing) - Lightweight and fast
  - [Minikube](https://minikube.sigs.k8s.io/) - Full-featured local cluster
  - Any cloud Kubernetes cluster (GKE, EKS, AKS)
- **kubectl** - [Install kubectl](https://kubernetes.io/docs/tasks/tools/)
- **Basic Kubernetes knowledge** - Familiarity with pods, deployments, and services

### Setting up a local cluster

**Using Kind (recommended):**

```bash
# Install Kind
brew install kind  # macOS
# or download from https://kind.sigs.k8s.io/docs/user/quick-start/#installation

# Create a cluster
kind create cluster --name mcp-test
```

**Using Minikube:**

```bash
# Install Minikube
brew install minikube  # macOS
# or download from https://minikube.sigs.k8s.io/docs/start/

# Start a cluster
minikube start
```

## Step 1: Install the MCP Operator

Install the operator in your cluster:

```bash
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

Wait for the operator to be ready:

```bash
kubectl wait --for=condition=available --timeout=300s \
  deployment/mcp-operator-controller-manager \
  -n mcp-operator-system
```

Verify the installation:

```bash
kubectl get pods -n mcp-operator-system
```

### Optional: Enable Monitoring

If you want metrics and dashboards (requires Prometheus Operator):

```bash
# Skip this if you don't have Prometheus Operator installed
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/monitoring.yaml
```

You should see the controller manager running:

```
NAME                                               READY   STATUS    RESTARTS   AGE
mcp-operator-controller-manager-xxxxxxxxxx-xxxxx   2/2     Running   0          30s
```

## Step 2: Deploy Your First MCP Server

Create a file named `my-first-mcp-server.yaml`:

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-first-mcp-server
  namespace: default
spec:
  image: "tzolov/mcp-everything-server:v3"
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 8080
        sessionManagement: true
  security:
    runAsUser: 1000
    runAsGroup: 1000
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
```

Apply it to your cluster:

```bash
kubectl apply -f my-first-mcp-server.yaml
```

## Step 3: Verify It's Working

Check the MCPServer status:

```bash
kubectl get mcpserver my-first-mcp-server
```

You should see output like:

```
NAME                  PHASE     REPLICAS   READY   TRANSPORT   AGE
my-first-mcp-server   Running   1          1       http        1m
```

View detailed status:

```bash
kubectl describe mcpserver my-first-mcp-server
```

Check the pods:

```bash
kubectl get pods -l app=my-first-mcp-server
```

View logs:

```bash
kubectl logs -l app=my-first-mcp-server
```

## Step 4: Access Your MCP Server

The operator creates a Kubernetes service for your MCP server. Access methods vary by platform:

**Minikube:**

```bash
# Open service in browser (automatically handles port forwarding)
minikube service my-first-mcp-server

# Or get the URL without opening browser
minikube service my-first-mcp-server --url
```

**Kind or other Kubernetes clusters:**

```bash
# Port forward to access locally
kubectl port-forward service/my-first-mcp-server 8080:8080
```

Now you can access your MCP server at `http://localhost:8080`.

## Step 5: Connect an MCP Client

Once your MCP server is accessible, you can connect to it using MCP-compatible clients.

### Using MCP Inspector (Recommended for Testing)

The [MCP Inspector](https://github.com/modelcontextprotocol/inspector) is the official testing tool for MCP servers:

```bash
# Install MCP Inspector globally
npm install -g @modelcontextprotocol/inspector

# Connect to your local MCP server
mcp-inspector http://localhost:8080/mcp
```

This will open a web interface where you can:
- Test available tools and prompts
- Send requests to the MCP server
- View server capabilities and resources
- Debug MCP protocol interactions

### Using Claude Desktop

To connect Claude Desktop to your MCP server, add it to your Claude configuration:

**macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`

**Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "my-server": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

Restart Claude Desktop to load the new server configuration.

### Using Other MCP Clients

For remote MCP servers with ingress enabled, you can connect from any location:

```json
{
  "mcpServers": {
    "remote-server": {
      "url": "https://mcp.example.com/mcp"
    }
  }
}
```

**Additional Resources:**
- [Testing Remote MCP Servers (Cloudflare)](https://developers.cloudflare.com/agents/guides/test-remote-mcp-server/)
- [Awesome Remote MCP Servers](https://github.com/jaw9c/awesome-remote-mcp-servers)
- [MCP Protocol Documentation](https://modelcontextprotocol.io)

## Next Steps

### Try More Examples

Explore the sample configurations:

```bash
# Wikipedia MCP server
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/config/samples/wikipedia-http.yaml

# Full-featured example with ingress
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/config/samples/http-mcp-server-ingress.yaml
```

### Enable Advanced Features

**Horizontal Pod Autoscaling:**

```yaml
spec:
  hpa:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
```

**Ingress for External Access:**

```yaml
spec:
  ingress:
    enabled: true
    host: "mcp.example.com"
    className: "nginx"
```

See the [README](README.md) for complete API reference and more examples.

## Troubleshooting

### MCPServer stuck in "Creating" phase

Check pod status:

```bash
kubectl get pods -l app=my-first-mcp-server
kubectl describe pod -l app=my-first-mcp-server
```

### Pods are CrashLooping

View logs to see what's failing:

```bash
kubectl logs -l app=my-first-mcp-server
```

Common issues:
- **Image pull errors** - Verify the image exists and is accessible
- **Resource limits** - Try increasing memory/CPU limits
- **Port conflicts** - Ensure the port matches your container's expectations

### Operator not responding

Check operator logs:

```bash
kubectl logs -n mcp-operator-system \
  deployment/mcp-operator-controller-manager \
  -c manager
```

## Cleanup

Remove your MCP server:

```bash
kubectl delete mcpserver my-first-mcp-server
```

Uninstall the operator:

```bash
kubectl delete -f https://raw.githubusercontent.com/vitorbari/mcp-operator/main/dist/install.yaml
```

Delete your local cluster:

```bash
# Kind
kind delete cluster --name mcp-test

# Minikube
minikube delete
```

## Get Help

- **Found a bug?** [Open an issue](https://github.com/vitorbari/mcp-operator/issues/new)
- **Have questions?** [Start a discussion](https://github.com/vitorbari/mcp-operator/discussions)
- **Want to contribute?** See [CONTRIBUTING.md](CONTRIBUTING.md)

## What's Next?

- Read the [Architecture Documentation](docs/README.md)
- Explore [Advanced Examples](config/samples/)
- Learn about [Transport Configuration](README.md#transport-configuration)
- Set up [Monitoring and Observability](README.md#monitoring-and-observability)
