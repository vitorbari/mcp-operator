# MCP Metrics Sidecar Makefile

# Image URL to use for building/pushing the sidecar image
IMG ?= ghcr.io/vitorbari/mcp-proxy:latest

# Version to embed in the binary (set via ldflags)
VERSION ?= dev

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint against code.
	golangci-lint run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix.
	golangci-lint run --fix ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test -v -race ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage.
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

##@ Build

.PHONY: build
build: fmt vet ## Build the mcp-proxy binary.
	go build -ldflags="-X main.Version=$(VERSION)" -o bin/mcp-proxy ./cmd/mcp-proxy

.PHONY: run
run: fmt vet ## Run the mcp-proxy from source.
	go run ./cmd/mcp-proxy

.PHONY: clean
clean: ## Clean build artifacts.
	rm -rf bin/
	rm -f coverage.out coverage.html

##@ Container

.PHONY: docker-build
docker-build: ## Build docker image.
	$(CONTAINER_TOOL) build --build-arg VERSION=$(VERSION) -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for cross-platform support.
	- $(CONTAINER_TOOL) buildx create --use
	$(CONTAINER_TOOL) buildx build --build-arg VERSION=$(VERSION) --push --platform linux/amd64,linux/arm64 --tag ${IMG} .

# PLATFORMS defines the target platforms for the sidecar image.
PLATFORMS ?= linux/arm64,linux/amd64
.PHONY: docker-buildx-local
docker-buildx-local: ## Build docker image for cross-platform support (local only, no push).
	- $(CONTAINER_TOOL) buildx create --use
	$(CONTAINER_TOOL) buildx build --build-arg VERSION=$(VERSION) --platform $(PLATFORMS) --tag ${IMG} .
