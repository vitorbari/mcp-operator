#!/bin/bash

set -e

MONITOR_FILE="dist/chart/templates/prometheus/monitor.yaml"

echo "Patching ServiceMonitor template to support additional labels..."

if [ ! -f "$MONITOR_FILE" ]; then
    echo "Warning: ServiceMonitor template not found at $MONITOR_FILE"
    exit 0
fi

# Check if already patched
if grep -q "prometheus.additionalLabels" "$MONITOR_FILE"; then
    echo "ServiceMonitor already patched, skipping..."
    exit 0
fi

# Add support for prometheus.additionalLabels after control-plane label in metadata section only
# Use awk to insert after the FIRST occurrence of control-plane in labels section
awk '
/metadata:/ { in_metadata=1 }
/spec:/ { in_metadata=0 }
/control-plane: controller-manager/ && in_metadata && !done {
    print
    print "    {{- with .Values.prometheus.additionalLabels }}"
    print "    {{- toYaml . | nindent 4 }}"
    print "    {{- end }}"
    done=1
    next
}
{ print }
' "$MONITOR_FILE" > "${MONITOR_FILE}.tmp" && mv "${MONITOR_FILE}.tmp" "$MONITOR_FILE"

echo "✅ ServiceMonitor template patched successfully"
