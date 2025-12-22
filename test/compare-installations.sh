#!/bin/bash
set -e

# Compare resources created by kubectl vs Helm installation
# This helps verify that both installation methods produce equivalent resources

echo "=== Comparing kubectl Bundle vs Helm Chart Resources ==="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

compare_resource() {
    local resource_type=$1
    local resource_name=$2
    local namespace=$3
    local field=$4

    echo -n "Checking $resource_type/$resource_name $field... "

    kubectl_value=$(kubectl get "$resource_type" "$resource_name" -n "$namespace" -o jsonpath="$field" 2>/dev/null || echo "NOT_FOUND")
    helm_value=$(kubectl get "$resource_type" "$resource_name" -n "$namespace" -o jsonpath="$field" 2>/dev/null || echo "NOT_FOUND")

    if [ "$kubectl_value" = "$helm_value" ]; then
        echo -e "${GREEN}✓ Match${NC}"
        return 0
    else
        echo -e "${RED}✗ Differ${NC}"
        echo "  kubectl: $kubectl_value"
        echo "  helm: $helm_value"
        return 1
    fi
}

echo "Prerequisites:"
echo "1. Create two Kind clusters: 'test-kubectl' and 'test-helm'"
echo "2. Install operator via kubectl in 'test-kubectl'"
echo "3. Install operator via Helm in 'test-helm'"
echo ""
echo "Example setup:"
echo "  kind create cluster --name test-kubectl"
echo "  kubectl config use-context kind-test-kubectl"
echo "  kubectl apply -f dist/install.yaml"
echo ""
echo "  kind create cluster --name test-helm"
echo "  kubectl config use-context kind-test-helm"
echo "  helm install test oci://ghcr.io/vitorbari/mcp-operator --version 0.1.0-alpha.13 --namespace mcp-operator-system --create-namespace"
echo ""
read -p "Press enter when both clusters are ready..."

# Switch to kubectl cluster
echo ""
echo "=== Extracting kubectl bundle resources ==="
kubectl config use-context kind-test-kubectl
kubectl get all,servicemonitor,configmap -n mcp-operator-system -o yaml > /tmp/kubectl-resources.yaml
echo "Saved to /tmp/kubectl-resources.yaml"

# Switch to helm cluster
echo ""
echo "=== Extracting Helm chart resources ==="
kubectl config use-context kind-test-helm
kubectl get all,servicemonitor,configmap -n mcp-operator-system -o yaml > /tmp/helm-resources.yaml
echo "Saved to /tmp/helm-resources.yaml"

echo ""
echo "=== Key Differences to Expect ==="
echo ""
echo -e "${YELLOW}Expected differences (these are intentional):${NC}"
echo "1. ServiceMonitor labels (prometheus.additionalLabels templating)"
echo "2. Deployment labels/annotations (Helm adds helm.sh/* labels)"
echo "3. ConfigMap presence (Grafana dashboard conditional in Helm)"
echo "4. Resource metadata (Helm adds managed-by label)"
echo ""

echo -e "${YELLOW}Should be identical (core functionality):${NC}"
echo "1. CRD definition and schema"
echo "2. Container image (when using same version)"
echo "3. Container command and args"
echo "4. ServiceAccount, ClusterRole, ClusterRoleBinding permissions"
echo "5. Service ports and selectors"
echo "6. Deployment replica count (with default values)"
echo "7. Resource requests/limits (with default values)"
echo ""

echo "=== Comparing Core Resources ==="
echo ""

# Compare CRD
echo -n "CRD mcpservers.mcp.mcp-operator.io... "
kubectl config use-context kind-test-kubectl > /dev/null
kubectl_crd=$(kubectl get crd mcpservers.mcp.mcp-operator.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema}' | md5sum)
kubectl config use-context kind-test-helm > /dev/null
helm_crd=$(kubectl get crd mcpservers.mcp.mcp-operator.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema}' | md5sum)

if [ "$kubectl_crd" = "$helm_crd" ]; then
    echo -e "${GREEN}✓ Identical${NC}"
else
    echo -e "${RED}✗ Different${NC}"
fi

# Compare Deployment
echo ""
echo "Deployment comparison:"
kubectl config use-context kind-test-kubectl > /dev/null
kubectl_replicas=$(kubectl get deployment mcp-operator-controller-manager -n mcp-operator-system -o jsonpath='{.spec.replicas}')
kubectl_image=$(kubectl get deployment mcp-operator-controller-manager -n mcp-operator-system -o jsonpath='{.spec.template.spec.containers[0].image}')

kubectl config use-context kind-test-helm > /dev/null
helm_replicas=$(kubectl get deployment mcp-operator-controller-manager -n mcp-operator-system -o jsonpath='{.spec.replicas}')
helm_image=$(kubectl get deployment mcp-operator-controller-manager -n mcp-operator-system -o jsonpath='{.spec.template.spec.containers[0].image}')

echo -n "  Replicas... "
if [ "$kubectl_replicas" = "$helm_replicas" ]; then
    echo -e "${GREEN}✓ Match ($kubectl_replicas)${NC}"
else
    echo -e "${RED}✗ Differ (kubectl: $kubectl_replicas, helm: $helm_replicas)${NC}"
fi

echo -n "  Image... "
if [ "$kubectl_image" = "$helm_image" ]; then
    echo -e "${GREEN}✓ Match${NC}"
else
    echo -e "${YELLOW}⚠ Differ (expected if using different versions)${NC}"
    echo "    kubectl: $kubectl_image"
    echo "    helm: $helm_image"
fi

# Compare Service
echo ""
echo "Service comparison:"
kubectl config use-context kind-test-kubectl > /dev/null
kubectl_service_port=$(kubectl get service mcp-operator-controller-manager-metrics -n mcp-operator-system -o jsonpath='{.spec.ports[0].port}')

kubectl config use-context kind-test-helm > /dev/null
helm_service_port=$(kubectl get service mcp-operator-controller-manager-metrics -n mcp-operator-system -o jsonpath='{.spec.ports[0].port}')

echo -n "  Metrics port... "
if [ "$kubectl_service_port" = "$helm_service_port" ]; then
    echo -e "${GREEN}✓ Match ($kubectl_service_port)${NC}"
else
    echo -e "${RED}✗ Differ (kubectl: $kubectl_service_port, helm: $helm_service_port)${NC}"
fi

# Compare RBAC
echo ""
echo "RBAC comparison:"
kubectl config use-context kind-test-kubectl > /dev/null
kubectl_rules=$(kubectl get clusterrole mcp-operator-manager-role -o jsonpath='{.rules}' | md5sum)

kubectl config use-context kind-test-helm > /dev/null
helm_rules=$(kubectl get clusterrole mcp-operator-manager-role -o jsonpath='{.rules}' | md5sum)

echo -n "  ClusterRole rules... "
if [ "$kubectl_rules" = "$helm_rules" ]; then
    echo -e "${GREEN}✓ Identical${NC}"
else
    echo -e "${RED}✗ Different${NC}"
fi

echo ""
echo "=== Manual Comparison ==="
echo "For detailed comparison, use:"
echo "  diff -u /tmp/kubectl-resources.yaml /tmp/helm-resources.yaml | less"
echo ""
echo "Or use a visual diff tool:"
echo "  code --diff /tmp/kubectl-resources.yaml /tmp/helm-resources.yaml"
echo ""
