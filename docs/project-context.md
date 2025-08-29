# MCP Operator Project Context

## Project Vision
Kubernetes-native operator for managing MCP servers with enterprise features

## Architecture Decisions
- Built as Kubernetes operator using Kubebuilder
- Business Source License for commercial protection
- Target: Enterprise platform engineering teams
- Differentiator: True Kubernetes-native vs existing solutions

## Competitive Landscape
- Microsoft MCP Gateway (runs on K8s, not K8s-native)
- Lunar.dev MCP Gateway (commercial)
- Kong AI Gateway (adding MCP support)
- Multiple open source projects emerging

## Technical Stack
- Go + Kubebuilder
- Domain: mcp-operator.io
- CRDs: MCPServer, MCPGateway
- Integration: Service mesh, GitOps, HPA
