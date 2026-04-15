# ARM64 Build Issue - Root Cause & Solution

## The Problem

Your dev container environment **cannot build ARM64 images** because:

- ❌ QEMU ARM64 emulation is not available
- ❌ This container only supports `linux/amd64`
- ❌ The buildx builder claims ARM64 support but QEMU isn't actually installed

When you tried to build, it hung indefinitely trying to emulate ARM64 cross-compilation.

## The Solution

### ✅ Recommended: Use GitHub Actions (Automatic Multi-Arch Builds)

I've created `.github/workflows/build-multiarch.yml` which:

- Builds both `linux/amd64` and `linux/arm64` automatically
- Uses GitHub Actions' native multi-arch builders
- Pushes to Docker Hub with proper manifests
- Takes ~10-15 minutes per build

**Setup required (one-time):**

1. Go to your GitHub repo settings:
   <https://github.com/CyanAutomation/gogomio/settings/secrets/actions>

2. Add two secrets:
   - **DOCKER_USERNAME** = your Docker Hub username
   - **DOCKER_PASSWORD** = your Docker Hub access token (or password)

   To get an access token:
   - Go to <https://hub.docker.com/settings/security>
   - Click "New Access Token"
   - Name: `github-actions-build`
   - Permissions: Select "Read & Write"

3. Commit and push the workflow file (already created):

   ```bash
   git add .github/workflows/build-multiarch.yml
   git commit -m "Add GitHub Actions multi-arch build workflow"
   git push origin main
   ```

**Triggering builds:**

```bash
# Automatic: Push to main (builds 'latest' tag)
git push origin main

# For releases: Create a version tag
git tag -a v0.2.0 -m "Release 0.2.0"
git push origin v0.2.0
# (This builds both v0.2.0 and latest tags)

# Manual: Go to Actions tab and click "Run workflow"
# Choose "Build Multi-Architecture Docker Image"
# Enter custom tags (comma-separated): latest,0.2.0
```

**Monitor the build:**

- <https://github.com/CyanAutomation/gogomio/actions>
- Look for "Build Multi-Architecture Docker Image" workflow

## What Gets Built

After setup, you'll have:

- ✅ `cyanautomation/gogomio:latest` (both amd64 + arm64)
- ✅ `cyanautomation/gogomio:0.2.0` (tagged releases)
- ✅ Automatic manifest handling for multi-arch support
- ✅ Works on both x86 and Raspberry Pi

## Local Development

You can still build for amd64 only locally:

```bash
# For testing on your dev machine
docker build -t cyanautomation/gogomio:dev .

# Push only amd64 (don't use for production - breaks ARM64)
docker buildx build --platform linux/amd64 -t cyanautomation/gogomio:test --push .
```

## How to Fix Your Current Production Image

Your Docker Hub image currently only has amd64. Once you set up GitHub Actions:

```bash
# Push to trigger a rebuild
git commit --allow-empty -m "Trigger multi-arch build"
git push origin main

# Wait for build to complete (~15 min)
# Then verify:
docker manifest inspect cyanautomation/gogomio:latest
# Should show both amd64 and arm64
```

## Fallback: Build on Raspberry Pi

If you have a Raspberry Pi available, you can build natively:

```bash
git clone https://github.com/CyanAutomation/gogomio.git
cd gogomio
chmod +x scripts/build-multiarch.sh
./scripts/build-multiarch.sh 0.2.0
```

(This will be slow but will work, taking 15-20 minutes)

## Quick Reference

| Method | Speed | Multi-arch | Recommended |
|--------|-------|-----------|-------------|
| **GitHub Actions** | 10-15 min | ✅ Yes | ✅ **YES** |
| Dev container local | N/A | ❌ No | ❌ Dev only |
| Raspberry Pi native | 15-20 min | ✅ Yes | ✅ Alternative |

---

**Questions?** Check `.github/workflows/build-multiarch.yml` for the full workflow definition.
