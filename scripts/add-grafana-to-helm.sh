#!/bin/bash

set -e

CHART_DIR="dist/chart"
DASHBOARD_JSON="config/grafana/dashboard.json"

echo "Adding Grafana dashboard template..."
mkdir -p "$CHART_DIR/templates/grafana"
mkdir -p "$CHART_DIR/dashboards"

# Validate dashboard source file exists
if [ ! -f "$DASHBOARD_JSON" ]; then
    echo "Error: Dashboard file not found: $DASHBOARD_JSON" >&2
    exit 1
fi

# Copy dashboard JSON to chart
cp "$DASHBOARD_JSON" "$CHART_DIR/dashboards/"
echo "Copied dashboard.json to $CHART_DIR/dashboards/"

# Copy Grafana dashboard template
cp "config/grafana/dashboard-helm-template.yaml" "$CHART_DIR/templates/grafana/dashboard.yaml" 

# Add Grafana configuration to values.yaml if not present
if ! grep -q "^grafana:" "$CHART_DIR/values.yaml"; then
    cat >> "$CHART_DIR/values.yaml" <<'EOF'

# [GRAFANA] Grafana Dashboard Configurations
grafana:
  # -- Enable creation of grafana dashboards. Can be disabled if not using grafana in your cluster.
  enabled: false
EOF
    echo "Added grafana configuration to values.yaml"
fi

echo "Grafana dashboard template added successfully"
