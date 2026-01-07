# MCP Proxy Sidecar Test Deployment

This directory contains Kubernetes manifests for manually testing the MCP metrics sidecar proxy before operator integration.

## Prerequisites

- Kubernetes cluster (Kind, Minikube, or other)
- kubectl configured
- (Optional) Prometheus Operator for ServiceMonitor support

## Building and Pushing the Image

Before deploying, build and push the sidecar image:

```bash
cd sidecar

# Build the image
docker build -t ghcr.io/vitorbari/mcp-proxy:dev .

# Push to registry (requires authentication)
docker push ghcr.io/vitorbari/mcp-proxy:dev

# For Kind clusters, load the image directly:
kind load docker-image ghcr.io/vitorbari/mcp-proxy:dev --name <cluster-name>
```

Update `test-pod.yaml` to use `:dev` tag if not using `:latest`.

## Deploying

```bash
# Deploy all manifests
kubectl apply -f sidecar/deploy/

# Wait for pod to be ready
kubectl wait --for=condition=ready pod -l app=mcp-test --timeout=60s

# Verify both containers are running
kubectl get pod -l app=mcp-test
```

## Verifying the Deployment

### Check Pod Status

```bash
# List containers in the pod
kubectl get pod -l app=mcp-test -o jsonpath='{.items[0].spec.containers[*].name}'
# Expected: mcp-server mcp-proxy

# Check container readiness
kubectl get pod -l app=mcp-test -o jsonpath='{.items[0].status.containerStatuses[*].ready}'
# Expected: true true

# View logs
kubectl logs -l app=mcp-test -c mcp-proxy
kubectl logs -l app=mcp-test -c mcp-server
```

### Port Forwarding

```bash
# Forward both MCP and metrics ports
kubectl port-forward svc/mcp-test 8080:8080 9090:9090
```

### Testing MCP Requests

```bash
# Initialize connection (note: path is /mcp for Streamable HTTP)
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}'

# List available tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}'
```

### Using MCP Inspector

You can also test using MCP Inspector:

1. Open MCP Inspector at http://localhost:6274/
2. Connect to: `http://localhost:8080/mcp`
3. Use the Inspector UI to explore tools, resources, and prompts

### Health Checks

```bash
# Liveness probe
curl http://localhost:9090/healthz

# Readiness probe
curl http://localhost:9090/readyz
```

### Metrics

```bash
# View all metrics
curl http://localhost:9090/metrics

# Filter MCP-specific metrics
curl -s http://localhost:9090/metrics | grep mcp_
```

## Prometheus Integration

If you have Prometheus Operator installed, the ServiceMonitor will automatically configure Prometheus to scrape metrics from the sidecar.

```bash
# Check if ServiceMonitor is created
kubectl get servicemonitor mcp-test

# Verify in Prometheus UI (port-forward if needed)
kubectl port-forward svc/prometheus-operated 9090:9090 -n monitoring
# Then open http://localhost:9090 and search for mcp_* metrics
```

## Cleanup

```bash
kubectl delete -f sidecar/deploy/
```

## Manifest Overview

| File | Description |
|------|-------------|
| `test-pod.yaml` | Pod with MCP server and proxy sidecar containers |
| `service.yaml` | Service exposing MCP (8080) and metrics (9090) ports |
| `servicemonitor.yaml` | Prometheus ServiceMonitor for automatic scraping |
