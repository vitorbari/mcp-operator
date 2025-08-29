# MCP Operator

A Kubernetes operator for managing Model Context Protocol (MCP) servers with enterprise-grade features including horizontal pod autoscaling, RBAC, and comprehensive configuration management.

## Description

The MCP Operator simplifies the deployment and management of MCP servers on Kubernetes clusters. It provides a declarative API through custom resources that abstract away the complexity of managing deployments, services, RBAC, and autoscaling configurations.

**Key Features:**
- **Declarative Management**: Define MCP servers using Kubernetes custom resources
- **Horizontal Pod Autoscaling**: Built-in HPA support with CPU and memory metrics
- **Enterprise Security**: RBAC integration with user and group access controls
- **Production Ready**: Health checks, resource management, and comprehensive monitoring
- **Flexible Configuration**: Support for custom environments, volumes, and networking

## Architecture

The MCP Operator introduces the `MCPServer` custom resource that declaratively manages:

- **Kubernetes Deployments**: Container orchestration with configurable replicas
- **Services**: Network exposure with customizable ports and service types
- **ServiceAccounts & RBAC**: Fine-grained security controls
- **Horizontal Pod Autoscalers**: Automatic scaling based on resource utilization
- **ConfigMaps & Secrets**: Configuration and credential management

## MCPServer Resource

### Basic Example

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: my-mcp-server
  namespace: default
spec:
  image: "my-registry/mcp-server:v1.0.0"
  replicas: 2
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
```

### Advanced Example with HPA

```yaml
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: advanced-mcp-server
spec:
  image: "my-registry/mcp-server:v1.0.0"
  replicas: 2

  # Horizontal Pod Autoscaler
  hpa:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80

  # Security Configuration
  security:
    allowedUsers:
      - "admin"
      - "developer"
    allowedGroups:
      - "mcp-users"

  # Service Configuration
  service:
    type: "ClusterIP"
    port: 8080
    targetPort: 3000

  # Health Checks
  healthCheck:
    enabled: true
    path: "/health"
    port: 3000
    initialDelaySeconds: 15
    periodSeconds: 10

  # Environment Variables
  environment:
    - name: "LOG_LEVEL"
      value: "info"
    - name: "MCP_PORT"
      value: "3000"
```

## Getting Started

### Prerequisites
- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/mcp-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/mcp-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/mcp-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/mcp-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Development

### Running Tests

Run the comprehensive test suite:

```sh
make test
```

### Running the Operator Locally

1. Install CRDs into your cluster:
```sh
make install
```

2. Run the operator locally:
```sh
make run
```

### Code Generation

After modifying API types, regenerate code and manifests:

```sh
make manifests generate
```

## API Reference

The operator provides the following custom resources:

### MCPServer

The `MCPServer` custom resource defines the desired state of an MCP server deployment.

**Key Spec Fields:**
- `image` (required): Container image for the MCP server
- `replicas`: Number of desired replicas (default: 1)
- `resources`: CPU and memory resource requirements
- `hpa`: Horizontal Pod Autoscaler configuration
- `security`: RBAC and access control settings
- `service`: Service exposure configuration
- `healthCheck`: Health check probe configuration
- `environment`: Environment variables
- `podTemplate`: Additional pod template specifications

**Status Fields:**
- `phase`: Current phase (Creating, Running, Scaling, Failed)
- `replicas`: Current replica counts
- `conditions`: Detailed status conditions
- `serviceEndpoint`: Service endpoint URL

## Contributing

We welcome contributions! Please follow these guidelines:

1. **Fork the repository** and create a feature branch
2. **Write tests** for any new functionality
3. **Run the test suite** with `make test`
4. **Update documentation** as needed
5. **Submit a pull request** with a clear description

### Architecture Decision Records (ADRs)

This project uses ADRs to document important architectural decisions:
- [ADR-001: MCPServer API Design](docs/adr-001-mcpserver-api-design.md)
- [ADR-002: MCPServer Controller Implementation](docs/adr-002-mcpserver-controller-implementation.md)
- [ADR-003: MCPServer HPA Support](docs/adr-003-mcpserver-hpa-support.md)

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Contributing

By contributing to this project, you agree that your contributions will be licensed under the same Business Source License 1.1 as the project.

### Contributor License Agreement
- All contributions must be compatible with BSL 1.1
- Contributors retain copyright of their contributions
- Contributions become part of the Licensed Work under BSL 1.1

## License

Copyright 2025 Vitor Bari.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

https://www.apache.org/licenses/LICENSE-2.0.txt

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
