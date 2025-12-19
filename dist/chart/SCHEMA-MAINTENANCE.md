# Values Schema Maintenance Guide

## Overview

The `values.schema.json` file provides validation and documentation for the Helm chart's configuration values. This appears on ArtifactHub and helps users understand what values are available and valid.

## Current Approach: Manual Maintenance

The `values.schema.json` is currently **manually maintained** due to tooling limitations on ARM Macs. Auto-generation tools like `helm schema-gen` don't support ARM architecture yet.

## When to Update

Update `values.schema.json` whenever you:
- Add new configuration options to `values.yaml`
- Remove configuration options from `values.yaml`
- Change the type or structure of existing values
- Want to add/update validation rules (min/max, patterns, enums)
- Want to improve descriptions

## How to Update

### 1. Update values.yaml with Documentation Comments

Always use the `-- ` comment syntax (note the space) before each value:

```yaml
controllerManager:
  # -- Number of operator replicas to run
  replicas: 1
  container:
    image:
      # -- Container image repository
      repository: ghcr.io/vitorbari/mcp-operator
      # -- Container image tag (should match operator version)
      tag: v0.1.0-alpha.13
```

These comments are used by `helm-docs` to generate documentation tables.

### 2. Update values.schema.json

Add corresponding JSON Schema entries:

```json
{
  "properties": {
    "controllerManager": {
      "type": "object",
      "properties": {
        "replicas": {
          "type": "integer",
          "description": "Number of operator replicas to run",
          "minimum": 1,
          "default": 1
        },
        "container": {
          "type": "object",
          "properties": {
            "image": {
              "type": "object",
              "properties": {
                "repository": {
                  "type": "string",
                  "description": "Container image repository",
                  "default": "ghcr.io/vitorbari/mcp-operator"
                },
                "tag": {
                  "type": "string",
                  "description": "Container image tag (should match operator version)",
                  "pattern": "^v[0-9]+\\.[0-9]+\\.[0-9]+(-[a-zA-Z0-9.]+)?$",
                  "default": "v0.1.0-alpha.13"
                }
              }
            }
          }
        }
      }
    }
  }
}
```

### 3. Validate the Schema

Test that the schema is valid JSON and works with Helm:

```bash
# Validate JSON syntax
jq '.' dist/chart/values.schema.json > /dev/null && echo "Valid JSON" || echo "Invalid JSON"

# Test with Helm (requires Helm 3.4.1+)
helm lint dist/chart --strict

# Test installation with validation
helm install test-schema dist/chart --dry-run --debug
```

### 4. Test on ArtifactHub

After pushing a new chart version:
1. Go to https://artifacthub.io/packages/helm/mcp-operator/mcp-operator
2. Check that the "Values" tab shows your schema documentation
3. Verify field types, defaults, and descriptions are correct

## Schema Validation Features

Add validation rules to help users:

**Type validation:**
```json
{"type": "string"}
{"type": "integer"}
{"type": "boolean"}
{"type": "object"}
{"type": "array"}
```

**Numeric constraints:**
```json
{
  "type": "integer",
  "minimum": 1,
  "maximum": 10
}
```

**String patterns:**
```json
{
  "type": "string",
  "pattern": "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
}
```

**Enums:**
```json
{
  "type": "string",
  "enum": ["RuntimeDefault", "Localhost", "Unconfined"]
}
```

**Required fields:**
```json
{
  "type": "object",
  "required": ["image", "tag"],
  "properties": { ... }
}
```

## Automation (Future)

When ARM-compatible tools become available, we can automate with:

1. **helm-docs** (current - only for README, not schema)
2. **helm schema-gen** (when ARM support is added)
3. **helm-values-schema-json** (alternative tool)
4. **Custom script** (if needed)

For now, manual maintenance ensures:
- ✅ Schema stays in sync with values.yaml
- ✅ High-quality descriptions and validation
- ✅ Works on all platforms (including ARM Macs)
- ✅ Complete control over validation rules

## Checklist for Values Changes

When modifying `values.yaml`:

- [ ] Update the value in `values.yaml`
- [ ] Add `-- ` comment documentation
- [ ] Update `values.schema.json` with:
  - [ ] Correct type
  - [ ] Description (matching the comment)
  - [ ] Default value
  - [ ] Validation rules (min/max/pattern/enum if applicable)
- [ ] Run `helm lint dist/chart --strict`
- [ ] Test with `helm install --dry-run`
- [ ] Commit both files together
- [ ] After release, verify on ArtifactHub

## Resources

- [JSON Schema Reference](https://json-schema.org/understanding-json-schema/)
- [Helm Values Schema Docs](https://helm.sh/docs/topics/charts/#schema-files)
- [ArtifactHub Values Display](https://artifacthub.io/docs/topics/repositories/#values-schema)
- [helm-docs Documentation](https://github.com/norwoodj/helm-docs)
