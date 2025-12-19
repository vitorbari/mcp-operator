# MCPServer Examples

These examples demonstrate different configuration patterns for MCPServer resources.

## Simple Wikipedia Server

**File:** `simple-wikipedia.yaml`

A minimal example using the Wikipedia MCP server with SSE transport and strict validation mode.

```bash
kubectl apply -f simple-wikipedia.yaml
```

Features:
- SSE (Server-Sent Events) transport
- Auto-detection enabled
- Strict validation mode
- Session management


Features:
- Streamable HTTP transport (modern MCP protocol)
- Horizontal Pod Autoscaler (2-10 replicas)
- Resource requests and limits
- Session management
- Pod security settings

## Verify Deployment

After applying an example:

```bash
# Watch the server come up
kubectl get mcpserver -n <namespace> -w

# Check validation status
kubectl get mcpserver <name> -n <namespace> -o jsonpath='{.status.validation}' | jq

# View detailed information
kubectl describe mcpserver <name> -n <namespace>
```

## More Examples

For additional examples, see the [config/samples](https://github.com/vitorbari/mcp-operator/tree/main/config/samples) directory in the repository:

- `mcp-complete-example.yaml` - Shows all available CRD fields
- Custom transport configurations
- Advanced security settings
- Ingress configurations
