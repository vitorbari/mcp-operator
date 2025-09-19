# Development Guide

This guide covers development workflows, building, testing, and contributing to the MCP Operator.

## Prerequisites

- go version v1.24.0+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster

## Development Workflow

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

## Building and Deployment

### Building the Image

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/mcp-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don't work.

### Deploying to Cluster

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

### Cleanup

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
- [ADR-004: Transport Configuration Support](docs/adr-004-transport-configuration-support.md)
- [ADR-005: Prometheus Metrics and Ingress Observability](docs/adr-005-observability.md)
- [ADR-006: Custom Metrics and Dashboard Implementation](docs/adr-006-customer-metrics-and-dashboard.md)

### Code Quality

Run linting and formatting:

```sh
make lint
```

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

By contributing to this project, you agree that your contributions will be licensed under the same Apache License 2.0 as the project.

### Contributor License Agreement
- All contributions must be compatible with Apache License 2.0
- Contributors retain copyright of their contributions
- Contributions become part of the Licensed Work under Apache License 2.0