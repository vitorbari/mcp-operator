# Release Process

This document describes how to create a new release of the MCP Operator.

## Overview

Releases are automated via GitHub Actions. When you push a git tag, the workflow:
1. Builds multi-platform container images (linux/amd64, linux/arm64)
2. Pushes images to GitHub Container Registry (GHCR)
3. Generates installation manifests (`dist/install.yaml`, `dist/monitoring.yaml`)
4. Updates Helm chart versions to match the release
5. Packages and pushes Helm chart to GHCR
6. Creates a GitHub Release with installation instructions and changelog

## Prerequisites

- [ ] All tests passing (`make test`, `make test-e2e`)
- [ ] All changes committed and pushed to `main` branch
- [ ] You have write access to the repository
- [ ] GHCR package is linked to the repository (one-time setup)

## Release Workflow

> **💡 Quick Start:** You can use `make release VERSION=v0.1.0-alpha.X` to automate steps 1-3 below.
> See [Quick Reference](#quick-reference) for details.

### Step 1: Update Version in Kustomization

Edit `config/manager/kustomization.yaml` and update the `newTag` field:

```yaml
images:
- name: controller
  newName: ghcr.io/vitorbari/mcp-operator
  newTag: v0.1.0-alpha.2  # Update this
```

Or use `sed`:
```bash
# For the next alpha version
sed -i '' 's/newTag: .*/newTag: v0.1.0-alpha.2/' config/manager/kustomization.yaml
```

### Step 2: Commit the Version Bump

```bash
git add config/manager/kustomization.yaml
git commit -m "Bump version to v0.1.0-alpha.2"
git push origin main
```

### Step 3: Create and Push the Tag

```bash
# Create the tag (must match the version in kustomization.yaml)
git tag v0.1.0-alpha.2

# Push the tag (this triggers the release workflow)
git push origin v0.1.0-alpha.2
```

### Step 4: Monitor the Workflow

Watch the release workflow:
```bash
# Using GitHub CLI
gh run watch

# Or open in browser
open https://github.com/vitorbari/mcp-operator/actions
```

The workflow takes approximately 5-10 minutes.

### Step 5: Verify the Release

Once the workflow completes:

**Check the GitHub Release:**
```bash
open https://github.com/vitorbari/mcp-operator/releases
```

Verify:
- [ ] Release created with correct version
- [ ] `dist/install.yaml` attached
- [ ] `dist/monitoring.yaml` attached
- [ ] Installation instructions include both Helm and kubectl options
- [ ] Changelog is auto-generated
- [ ] Pre-release flag set correctly (for alpha/beta/rc)

**Check the Container Image:**
```bash
open https://github.com/vitorbari/mcp-operator/pkgs/container/mcp-operator
```

Verify:
- [ ] New version tag exists (e.g., `v0.1.0-alpha.13`)
- [ ] Package is public (not private)
- [ ] Both platforms available (linux/amd64, linux/arm64)

**Check the Helm Chart:**
```bash
# Pull the chart to verify it exists
helm pull oci://ghcr.io/vitorbari/mcp-operator --version 0.1.0-alpha.13

# Extract and inspect
tar -xzf mcp-operator-0.1.0-alpha.13.tgz
cat mcp-operator/Chart.yaml
```

Verify:
- [ ] Helm chart version matches release (without 'v' prefix: `0.1.0-alpha.13`)
- [ ] Chart appVersion matches release (without 'v' prefix: `0.1.0-alpha.13`)
- [ ] values.yaml has correct image tag (with 'v' prefix: `v0.1.0-alpha.13`)
- [ ] Chart pulls successfully from GHCR

**Test the kubectl Installation:**
```bash
# Create test cluster
kind create cluster --name test-release-kubectl

# Install from the release
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/v0.1.0-alpha.13/dist/install.yaml

# Wait for operator
kubectl wait --for=condition=available --timeout=120s \
  deployment/mcp-operator-controller-manager -n mcp-operator-system

# Verify image
kubectl describe deployment mcp-operator-controller-manager -n mcp-operator-system | grep Image:
# Should show: ghcr.io/vitorbari/mcp-operator:v0.1.0-alpha.13

# Test operator functionality
kubectl apply -f config/samples/02-streamable-http-basic.yaml
kubectl get mcpserver

# Cleanup
kind delete cluster --name test-release-kubectl
```

**Test the Helm Installation:**
```bash
# Create test cluster
kind create cluster --name test-release-helm

# Install via Helm
helm install test-release oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --create-namespace --namespace mcp-operator-system

# Wait for operator
kubectl wait --for=condition=available --timeout=120s \
  deployment/mcp-operator-controller-manager -n mcp-operator-system

# Verify Helm release
helm list -n mcp-operator-system

# Verify image
kubectl describe deployment mcp-operator-controller-manager -n mcp-operator-system | grep Image:
# Should show: ghcr.io/vitorbari/mcp-operator:v0.1.0-alpha.13

# Test operator functionality
kubectl apply -f config/samples/02-streamable-http-basic.yaml
kubectl get mcpserver

# Test upgrade
helm upgrade test-release oci://ghcr.io/vitorbari/mcp-operator \
  --version 0.1.0-alpha.13 \
  --reuse-values \
  --set prometheus.enable=true

# Cleanup
helm uninstall test-release -n mcp-operator-system
kind delete cluster --name test-release-helm
```

## Version Numbering

Follow semantic versioning with pre-release identifiers:

### Alpha Releases (Development)
Use for early development, breaking changes allowed:
```
v0.1.0-alpha.1
v0.1.0-alpha.2
v0.1.0-alpha.3
```

### Beta Releases (Feature Complete)
Use when feature-complete, focusing on stabilization:
```
v0.1.0-beta.1
v0.1.0-beta.2
```

### Release Candidates (Production Ready)
Use for final testing before stable release:
```
v0.1.0-rc.1
v0.1.0-rc.2
```

### Stable Releases
Use for production-ready versions:
```
v0.1.0
v0.2.0
v1.0.0
```

**Note:** Only stable releases get the `latest` Docker tag.

## Release Artifacts

For a release `v0.1.0-alpha.13`, the following artifacts are created:

**Docker Images:**
- `ghcr.io/vitorbari/mcp-operator:v0.1.0-alpha.13` (exact version with 'v' prefix)
- `ghcr.io/vitorbari/mcp-operator:latest` (stable releases only, no pre-release tags)

**Helm Chart:**
- `oci://ghcr.io/vitorbari/mcp-operator:0.1.0-alpha.13` (version without 'v' prefix)

**YAML Manifests:**
- `dist/install.yaml` (attached to GitHub release)
- `dist/monitoring.yaml` (attached to GitHub release)

**Note on versioning:**
- Docker image tags include the 'v' prefix (e.g., `v0.1.0-alpha.13`)
- Helm chart versions follow semantic versioning without 'v' prefix (e.g., `0.1.0-alpha.13`)
- Both are synchronized automatically by the release workflow

## Troubleshooting

### Workflow Fails with Permission Error

**Error:** `permission_denied: write_package`

**Solution:** The workflow uses the `GHCR_TOKEN` secret. Verify:
1. Go to: https://github.com/vitorbari/mcp-operator/settings/secrets/actions
2. Confirm `GHCR_TOKEN` exists
3. Token needs `write:packages` scope

### Tag Already Exists

**Error:** `tag v0.1.0-alpha.2 already exists`

**Solution:** Delete and recreate:
```bash
# Delete local tag
git tag -d v0.1.0-alpha.2

# Delete remote tag
git push --delete origin v0.1.0-alpha.2

# Delete GitHub release (if created)
gh release delete v0.1.0-alpha.2 --yes

# Recreate
git tag v0.1.0-alpha.2
git push origin v0.1.0-alpha.2
```

### Installation Manifest Has Wrong Image

**Error:** `dist/install.yaml` references wrong version

**Cause:** Version in `config/manager/kustomization.yaml` doesn't match the git tag

**Solution:** Ensure both match:
```bash
# Check kustomization
grep newTag config/manager/kustomization.yaml

# Check git tag
git describe --tags

# They must match!
```

### GHCR Package is Private

**Error:** Users can't pull the image or Helm chart

**Solution:** Make package public (one-time):
1. Go to: https://github.com/vitorbari/mcp-operator/pkgs/container/mcp-operator
2. Package settings → Change visibility → Public

### Helm Chart Not Found in GHCR

**Error:** `Error: failed to pull oci://ghcr.io/vitorbari/mcp-operator:0.1.0-alpha.X`

**Cause:** Helm chart wasn't pushed successfully or package is private

**Solution:**
1. Check workflow logs for Helm push errors
2. Verify package visibility is public (see above)
3. Try pulling with authentication:
   ```bash
   echo $GITHUB_TOKEN | helm registry login ghcr.io -u $GITHUB_USERNAME --password-stdin
   helm pull oci://ghcr.io/vitorbari/mcp-operator --version 0.1.0-alpha.X
   ```

### Chart Version Mismatch

**Error:** Chart.yaml version doesn't match appVersion or Docker tag

**Cause:** Workflow sed commands failed or Chart.yaml was committed with wrong version

**Solution:**
1. Verify `config/manager/kustomization.yaml` has correct tag
2. Check workflow logs for sed errors
3. Manually verify Chart.yaml after workflow:
   ```bash
   git fetch origin main
   git show origin/main:dist/chart/Chart.yaml | grep -E "^(version|appVersion):"
   ```

### values.yaml Not Taking Effect

**Error:** Custom values not applied during Helm install

**Cause:** Chart regeneration overwrote manual changes

**Solution:**
- Don't manually edit generated chart files
- Use `--set` flags or values file during installation:
  ```bash
  helm install mcp-operator oci://ghcr.io/vitorbari/mcp-operator \
    --version 0.1.0-alpha.X \
    --set controllerManager.replicas=2
  ```

## Quick Reference

### Option 1: Using Make (Recommended)

```bash
# Create and push a new release tag
make release VERSION=v0.1.0-alpha.2
```

This command will:
- Validate the version format
- Update `config/manager/kustomization.yaml`
- Commit and push the change to main
- Create and push the git tag
- Trigger the GitHub Actions release workflow

### Option 2: Manual Process

```bash
# Update version in kustomization.yaml
sed -i '' 's/newTag: .*/newTag: v0.1.0-alpha.2/' config/manager/kustomization.yaml

# Commit and push
git add config/manager/kustomization.yaml
git commit -m "Bump version to v0.1.0-alpha.2"
git push origin main

# Create and push tag
git tag v0.1.0-alpha.2
git push origin v0.1.0-alpha.2
```

## Release Checklist

Before creating a release:

- [ ] All tests passing (`make test`, `make test-e2e`)
- [ ] Chart tests passing (`.github/workflows/test-chart.yml`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated (if maintained)
- [ ] Breaking changes documented
- [ ] Version number follows semver
- [ ] Kustomization version matches git tag

After release created:

- [ ] Workflow completed successfully
- [ ] GitHub release created with Helm and kubectl instructions
- [ ] Container images pushed (both platforms)
- [ ] Helm chart pushed to GHCR
- [ ] Chart version matches operator version (without 'v')
- [ ] kubectl installation tested on clean cluster
- [ ] Helm installation tested on clean cluster
- [ ] Helm upgrade tested
- [ ] Package visibility is public (both image and chart)
- [ ] Announced (if appropriate)

## Rollback

If a release has critical issues:

### Delete the Release
```bash
gh release delete v0.1.0-alpha.2 --yes
git push --delete origin v0.1.0-alpha.2
git tag -d v0.1.0-alpha.2
```

### Delete GHCR Images (if needed)
Go to: https://github.com/vitorbari/mcp-operator/pkgs/container/mcp-operator
- Click on the version
- "Package settings" → Delete package version

### Revert Version in Repo
```bash
sed -i '' 's/newTag: .*/newTag: v0.1.0-alpha.1/' config/manager/kustomization.yaml
git add config/manager/kustomization.yaml
git commit -m "Revert to v0.1.0-alpha.1"
git push origin main
```

## Post-Release

After a successful release:

1. Update README badges (if applicable)
2. Announce in discussions/social media
3. Monitor for issues
4. Plan next release based on feedback
