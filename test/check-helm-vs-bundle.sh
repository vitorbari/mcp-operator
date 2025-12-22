#!/bin/bash
set -e

# Quick check to see what resources Helm created vs what's in the YAML bundle
# Run this on a cluster with Helm-installed operator

NAMESPACE="mcp-operator-system"

echo "=== Helm-Created Resources vs YAML Bundle ==="
echo ""

echo "Resources created by Helm:"
kubectl get all,servicemonitor,configmap -n "$NAMESPACE" -o name | sort

echo ""
echo "=== Checking for Helm-specific additions ==="

# Check for Grafana dashboard
echo -n "Grafana dashboard ConfigMap... "
if kubectl get configmap mcp-operator-grafana-dashboard -n "$NAMESPACE" &>/dev/null; then
    echo "✓ Present (Helm-specific, controlled by grafana.enabled)"
else
    echo "✗ Not found (set grafana.enabled=true to create)"
fi

# Check ServiceMonitor labels
echo -n "ServiceMonitor custom labels... "
if kubectl get servicemonitor -n "$NAMESPACE" &>/dev/null; then
    labels=$(kubectl get servicemonitor mcp-operator-controller-manager-metrics -n "$NAMESPACE" -o jsonpath='{.metadata.labels}' 2>/dev/null || echo "{}")
    echo ""
    echo "$labels" | jq -r 'to_entries[] | "  \(.key): \(.value)"'
    echo "  (Helm allows customizing these via prometheus.additionalLabels)"
else
    echo "✗ Not found (set prometheus.enable=true to create)"
fi

echo ""
echo "=== Checking Deployment Configuration ==="

# Get deployment details
replicas=$(kubectl get deployment mcp-operator-controller-manager -n "$NAMESPACE" -o jsonpath='{.spec.replicas}')
image=$(kubectl get deployment mcp-operator-controller-manager -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}')
cpu_limit=$(kubectl get deployment mcp-operator-controller-manager -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].resources.limits.cpu}')
mem_limit=$(kubectl get deployment mcp-operator-controller-manager -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')

echo "Current values (from Helm):"
echo "  Replicas: $replicas (values.yaml: controllerManager.replicas)"
echo "  Image: $image (values.yaml: controllerManager.container.image.repository:tag)"
echo "  CPU limit: $cpu_limit (values.yaml: controllerManager.container.resources.limits.cpu)"
echo "  Memory limit: $mem_limit (values.yaml: controllerManager.container.resources.limits.memory)"

echo ""
echo "YAML bundle would have hardcoded values from kustomization build time."
echo "Helm allows runtime configuration via values.yaml or --set flags."

echo ""
echo "=== Helm Release Info ==="
helm list -n "$NAMESPACE"

echo ""
echo "To see all Helm values currently applied:"
echo "  helm get values mcp-operator -n $NAMESPACE --all"

echo ""
echo "To compare Helm-generated manifests with YAML bundle:"
echo "  helm get manifest mcp-operator -n $NAMESPACE > /tmp/helm-manifest.yaml"
echo "  diff -u dist/install.yaml /tmp/helm-manifest.yaml | less"
