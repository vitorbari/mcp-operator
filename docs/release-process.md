# Release Process

This document describes how to create a new release of the MCP Operator.

## Overview

Releases are automated via GitHub Actions. When you push a git tag, the workflow:
1. Builds multi-platform container images (linux/amd64, linux/arm64)
2. Pushes images to GitHub Container Registry (GHCR)
3. Generates installation manifests (`dist/install.yaml`, `dist/monitoring.yaml`)
4. Creates a GitHub Release with installation instructions and changelog

## Prerequisites

- [ ] All tests passing (`make test`, `make test-e2e`)
- [ ] All changes committed and pushed to `main` branch
- [ ] You have write access to the repository
- [ ] GHCR package is linked to the repository (one-time setup)

## Release Workflow

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
- [ ] Installation instructions are present
- [ ] Changelog is auto-generated
- [ ] Pre-release flag set correctly (for alpha/beta/rc)

**Check the Container Image:**
```bash
open https://github.com/vitorbari/mcp-operator/pkgs/container/mcp-operator
```

Verify:
- [ ] New version tag exists
- [ ] Package is public (not private)
- [ ] Both platforms available (linux/amd64, linux/arm64)

**Test the Installation:**
```bash
# Create test cluster
kind create cluster --name test-release

# Install from the release
kubectl apply -f https://raw.githubusercontent.com/vitorbari/mcp-operator/v0.1.0-alpha.2/dist/install.yaml

# Wait for operator
kubectl wait --for=condition=available --timeout=120s \
  deployment/controller-manager -n mcp-operator-system

# Verify image
kubectl describe deployment controller-manager -n mcp-operator-system | grep Image:
# Should show: ghcr.io/vitorbari/mcp-operator:v0.1.0-alpha.2

# Test operator functionality
kubectl apply -f config/samples/mcp-everything-server.yaml
kubectl get mcpserver

# Cleanup
kind delete cluster --name test-release
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

## Container Image Tags

For a release `v0.1.0-alpha.2`, the following tags are created:

- `ghcr.io/vitorbari/mcp-operator:v0.1.0-alpha.2` (exact version)
- `ghcr.io/vitorbari/mcp-operator:latest` (stable releases only)

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

**Error:** Users can't pull the image

**Solution:** Make package public (one-time):
1. Go to: https://github.com/vitorbari/mcp-operator/pkgs/container/mcp-operator
2. Package settings → Change visibility → Public

## Quick Reference

```bash
# Complete release process
sed -i '' 's/newTag: .*/newTag: v0.1.0-alpha.2/' config/manager/kustomization.yaml
git add config/manager/kustomization.yaml
git commit -m "Bump version to v0.1.0-alpha.2"
git push origin main
git tag v0.1.0-alpha.2
git push origin v0.1.0-alpha.2
gh run watch
```

## Release Checklist

Before creating a release:

- [ ] All tests passing
- [ ] Documentation updated
- [ ] CHANGELOG.md updated (if maintained)
- [ ] Breaking changes documented
- [ ] Version number follows semver
- [ ] Kustomization version matches git tag

After release created:

- [ ] Workflow completed successfully
- [ ] GitHub release created
- [ ] Container images pushed
- [ ] Installation tested on clean cluster
- [ ] Package visibility is public
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
