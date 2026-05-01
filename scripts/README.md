# GoGoMio Build Scripts

This directory contains helper scripts for building and managing GoGoMio Docker images.

## build-multiarch.sh

**Purpose:** Build and push multi-architecture Docker images to Docker Hub (linux/amd64 and linux/arm64)

**Features:**

- ✅ Automatic buildx builder setup
- ✅ Multi-platform build (x86_64 and ARM64)
- ✅ Docker Hub authentication check
- ✅ Manifest verification
- ✅ Pretty formatted output
- ✅ Error checking at each step

### Usage

```bash
# Build and push 'latest' tag (default)
./scripts/build-multiarch.sh

# Build and push specific version
./scripts/build-multiarch.sh 0.2.0

# Build and push multiple versions
./scripts/build-multiarch.sh 0.2.0-rc1
```

### What It Does

1. ✅ Checks Docker and buildx availability
2. ✅ Verifies Docker Hub login
3. ✅ Creates/activates buildx builder for multi-platform support
4. ✅ Builds for both `linux/amd64` and `linux/arm64`
5. ✅ Pushes directly to Docker Hub
6. ✅ Verifies the manifest was created correctly
7. ✅ Shows supported architectures

### Prerequisites

- Docker installed with buildx support
- Logged into Docker Hub: `docker login`
- Run from project root directory

### Example Output

```
════════════════════════════════════════════════════════
Multi-Architecture Docker Build for GoGoMio
════════════════════════════════════════════════════════

1. Checking prerequisites...
✓ Docker and buildx available
✓ Logged into Docker Hub
✓ Dockerfile found

2. Setting up buildx builder...
✓ Builder 'gogomio-builder' already exists
✓ Builder activated

3. Building multi-architecture image...
Platforms: linux/amd64,linux/arm64
Image: cyanautomation/gogomio:latest

[...build output...]

✓ Build and push completed successfully

4. Verifying manifest...

✓ Manifest created successfully

Platform architectures:
  ✓ amd64
  ✓ arm64

════════════════════════════════════════════════════════
✅ Build Complete!
════════════════════════════════════════════════════════

Available images:
  ✓ cyanautomation/gogomio:latest

To use on any architecture:
  docker pull cyanautomio/gogomio:latest

To verify on your platform:
  docker run -e MOCK_CAMERA=true -p 8000:8000 cyanautomation/gogomio:latest

To inspect all architectures:
  docker manifest inspect cyanautomation/gogomio:latest
```

## Quick Commands

### Make script executable

```bash
chmod +x scripts/build-multiarch.sh
```

### Build latest

```bash
./scripts/build-multiarch.sh
```

### Build specific version

```bash
./scripts/build-multiarch.sh 0.2.0
```

### Verify existing image supports both architectures

```bash
docker manifest inspect cyanautomation/gogomio:latest
```

### Test image on your current platform

```bash
docker pull cyanautomation/gogomio:latest
docker run -e MOCK_CAMERA=true -p 8000:8000 cyanautomation/gogomio:latest
# Open http://localhost:8000
```

## Preventing Future Issues

**❌ Don't do this:**

```bash
docker build -t cyanautomation/gogomio:latest .
docker push cyanautomation/gogomio:latest
# ^ This only pushes your current architecture and breaks multi-arch support
```

**✅ Always do this:**

```bash
./scripts/build-multiarch.sh latest
# or for versioned releases:
./scripts/build-multiarch.sh 0.2.0
```

## Troubleshooting

### ARM64 emulation not available

This dev container only supports amd64 and cannot emulate ARM64. You have three options:

#### ✅ Option 1: Use GitHub Actions (RECOMMENDED)

GitHub Actions provides native multi-architecture builders. Simply push to trigger an automatic build:

```bash
# Push to main (builds latest tag)
git push origin main

# Or create a version tag (builds version tag + latest)
git tag -a v0.2.0 -m "Release 0.2.0"
git push origin v0.2.0
```

**Watch the build:**

- <https://github.com/CyanAutomation/gogomio/actions>
- Workflow: "Build Multi-Architecture Docker Image"
- Takes ~10-15 minutes total

**Setup required (one-time):**
Add Docker Hub credentials to GitHub Secrets:

1. Go to: <https://github.com/CyanAutomation/gogomio/settings/secrets/actions>
2. Add `DOCKER_USERNAME` (your Docker Hub username)
3. Add `DOCKER_PASSWORD` (Docker Hub access token or password)

#### ⚠️ Option 2: Build amd64 only (development/testing)

Only recommended for local testing - **breaks Raspberry Pi compatibility**:

```bash
docker buildx build --platform linux/amd64 -t cyanautomation/gogomio:latest --push .
```

#### ⚠️ Option 3: Run from native ARM64 system

Build on actual Raspberry Pi hardware or a system with proper virtualization:

```bash
# On Raspberry Pi
./scripts/build-multiarch.sh latest
```

### "buildx not available"

```bash
# Upgrade Docker or enable buildx manually
docker run --privileged --rm tonistiigi/binfmt --install all
```

### "Not logged into Docker Hub"

```bash
docker login
# Enter your credentials
```

### Build takes a very long time

The ARM64 build uses QEMU emulation, which is **15-20x slower** than native compilation. This is normal and expected.

- amd64 native: ~10-20 seconds
- arm64 emulated: ~3-5 minutes
- Total: ~5 minutes

### Push fails partway through

The script uses `--push` to stream layers as they build. If the connection drops:

1. Check your Docker Hub login: `docker login`
2. Try again: `./scripts/build-multiarch.sh latest`
3. Docker will resume from cached layers

## CI/CD Integration

For GitHub Actions or other CI systems:

```yaml
- name: Build and push multi-arch image
  run: |
    chmod +x scripts/build-multiarch.sh
    ./scripts/build-multiarch.sh ${{ github.ref_name }}
```

## References

- [Docker buildx documentation](https://docs.docker.com/build/architecture/)
- [Multi-architecture builds](https://docs.docker.com/build/building/multi-platform/)
- [GoGoMio MULTI_ARCH_BUILD.md](../MULTI_ARCH_BUILD.md)
