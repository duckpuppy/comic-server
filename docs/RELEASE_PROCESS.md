# Release Process

This document describes the automated release process for comic-server.

## Overview

comic-server uses **automated releases** with:
- **Release Please**: Automated versioning and changelog generation based on Conventional Commits
- **GoReleaser**: Multi-platform binary builds
- **GitHub Actions**: CI/CD automation
- **Docker**: Automated container image builds

## How It Works

### 1. Commit with Conventional Commits Format

All commits should follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```bash
git commit -m "feat: add new feature"
git commit -m "fix: resolve bug"
git commit -m "docs: update documentation"
git commit -m "test: add tests"
git commit -m "chore: maintenance task"
```

**Commit Types:**

| Type | Description | Version Bump | In Changelog |
|------|-------------|--------------|--------------|
| `feat:` | New feature | Minor (0.x.0) | ✅ Yes |
| `fix:` | Bug fix | Patch (0.0.x) | ✅ Yes |
| `perf:` | Performance improvement | Patch | ✅ Yes |
| `refactor:` | Code refactoring | Patch | ✅ Yes |
| `docs:` | Documentation | None | ✅ Yes |
| `test:` | Test changes | None | ❌ No |
| `chore:` | Maintenance | None | ❌ No |
| `ci:` | CI/CD changes | None | ❌ No |
| `build:` | Build system changes | None | ❌ No |

**Breaking Changes:**

For breaking changes, add `!` after the type or include `BREAKING CHANGE:` in the commit body:

```bash
# Option 1: ! suffix
git commit -m "feat!: change API response format"

# Option 2: BREAKING CHANGE footer
git commit -m "feat: change API response format

BREAKING CHANGE: The /api/devices endpoint now returns an object instead of an array."
```

Breaking changes trigger a **major version bump** (x.0.0).

### 2. Release Please Creates PR

When you push commits to `master`:

1. Release Please GitHub Action runs automatically
2. It analyzes commits since the last release
3. It creates/updates a **Release PR** with:
   - Version bump (based on commit types)
   - Updated CHANGELOG.md
   - Updated version in `.release-please-manifest.json`

**Example Release PR:**

```markdown
Title: chore(master): release 0.9.0

This PR was automatically created by Release Please.

## 0.9.0 (2025-11-17)

### Features

* implement reverse sync for reading state and user metadata
* add comprehensive tests for reverse sync
* add manual testing guide

### Documentation

* document reverse sync feature in README and FEATURES.md
```

### 3. Review and Merge Release PR

1. Review the changelog in the PR
2. Verify the version bump is correct
3. Check that all commits are properly categorized
4. Merge the PR when ready

### 4. Release Please Creates Tag and GitHub Release

When the Release PR is merged:

1. Release Please creates a git tag (e.g., `v0.9.0`)
2. Creates a GitHub Release with the changelog
3. Triggers the GoReleaser workflow

### 5. GoReleaser Builds and Publishes

The GoReleaser workflow automatically:

1. **Builds binaries** for multiple platforms:
   - Linux: amd64, arm64
   - macOS: amd64 (Intel), arm64 (Apple Silicon)
   - Windows: amd64

2. **Creates archives** (.tar.gz for Linux/macOS, .zip for Windows):
   - Includes: binary, README.md, CLAUDE.md, CHANGELOG.md, docs/

3. **Generates checksums** (SHA256)

4. **Attaches artifacts to GitHub Release**

### 6. Docker Workflow Builds Images

The Docker workflow (separate) automatically:

1. Builds multi-platform container images
2. Publishes to GitHub Container Registry (ghcr.io)
3. Tags images with:
   - `latest` - Latest release
   - `vX.Y.Z` - Specific version (e.g., `v0.9.0`)
   - `X.Y` - Major.minor version (e.g., `0.9`)

## Release Workflow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│ Developer: Commit with conventional commits                 │
│   git commit -m "feat: add reverse sync"                    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions: Release Please runs on push to master       │
│   - Analyzes commits                                        │
│   - Creates/updates Release PR                              │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ Developer: Review and merge Release PR                      │
│   - Check changelog                                         │
│   - Verify version bump                                     │
│   - Merge when ready                                        │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions: Release Please creates tag & release        │
│   - Creates git tag (e.g., v0.9.0)                          │
│   - Creates GitHub Release                                  │
│   - Triggers GoReleaser workflow                            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions: GoReleaser builds and publishes             │
│   - Runs tests (go test ./...)                              │
│   - Builds multi-platform binaries                          │
│   - Creates archives                                        │
│   - Generates checksums                                     │
│   - Attaches to GitHub Release                              │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions: Docker workflow builds images               │
│   - Builds multi-platform images                            │
│   - Publishes to ghcr.io                                    │
│   - Tags: latest, vX.Y.Z, X.Y                               │
└─────────────────────────────────────────────────────────────┘
```

## Configuration Files

### Release Please Configuration

**`.release-please-manifest.json`** - Current version:
```json
{
  ".": "0.8.0"
}
```

**`release-please-config.json`** - Release configuration:
```json
{
  "packages": {
    ".": {
      "release-type": "go",
      "package-name": "comic-server",
      "include-component-in-tag": false,
      "bump-minor-pre-major": true,
      "changelog-sections": [
        {"type": "feat", "section": "Features"},
        {"type": "fix", "section": "Bug Fixes"},
        {"type": "docs", "section": "Documentation"},
        // ...
      ]
    }
  }
}
```

### GoReleaser Configuration

**`.goreleaser.yml`** - Build configuration:

Key settings:
- **Platforms**: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- **Binaries**: Statically linked (CGO_ENABLED=0)
- **Version info**: Embedded via ldflags
- **Archives**: Includes README, CHANGELOG, docs
- **Tests**: Runs before building (go test ./...)

### GitHub Actions Workflows

**`.github/workflows/release-please.yml`**:
- Runs on push to master
- Creates/updates Release PR
- Requires: `RELEASE_PLEASE_TOKEN` secret

**`.github/workflows/release.yml`**:
- Runs when GitHub Release is published
- Builds and publishes artifacts
- Uses: `GITHUB_TOKEN` (automatic)

**`.github/workflows/docker-publish.yml`**:
- Runs on tag push (e.g., `v*`)
- Builds multi-platform Docker images
- Publishes to ghcr.io

## Local Testing

### Test GoReleaser Configuration

Install GoReleaser:
```bash
# Using Homebrew (macOS/Linux)
brew install goreleaser

# Or using Go
go install github.com/goreleaser/goreleaser/v2@latest
```

Validate configuration:
```bash
goreleaser check
```

Build snapshot (no publish):
```bash
# Build for current platform only
goreleaser build --snapshot --clean --single-target

# Build for all platforms
goreleaser release --snapshot --clean --skip=publish

# Check artifacts
ls -lh dist/
```

### Test Release Please Configuration

Release Please validation:
```bash
# Install Release Please CLI (optional)
npm install -g release-please

# Test configuration (requires GitHub token)
export GITHUB_TOKEN=your_token
release-please manifest-pr \
  --repo-url=duckpuppy/comic-server \
  --dry-run
```

## Manual Release (Emergency)

If automation fails, you can manually create a release:

### 1. Update CHANGELOG.md

Manually add release notes following the existing format:

```markdown
## [0.9.0](https://github.com/duckpuppy/comic-server/compare/v0.8.0...v0.9.0) (2025-11-17)

### Features

* implement reverse sync

### Documentation

* add release process guide
```

### 2. Update Version Manifest

Edit `.release-please-manifest.json`:
```json
{
  ".": "0.9.0"
}
```

### 3. Commit Changes

```bash
git add CHANGELOG.md .release-please-manifest.json
git commit -m "chore(release): 0.9.0"
git push origin master
```

### 4. Create Tag

```bash
git tag -a v0.9.0 -m "Release v0.9.0"
git push origin v0.9.0
```

### 5. Trigger GoReleaser

The tag push will automatically trigger the GoReleaser workflow.

Alternatively, manually run GoReleaser:
```bash
# Requires GITHUB_TOKEN
export GITHUB_TOKEN=your_token
goreleaser release --clean
```

## Versioning Strategy

comic-server follows [Semantic Versioning](https://semver.org/):

**Format**: `MAJOR.MINOR.PATCH` (e.g., `0.9.0`)

- **MAJOR**: Breaking changes (e.g., API changes, incompatible library format)
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes, documentation, tests

**Pre-1.0 Rules**:
- Currently on `0.x.x` (pre-release)
- MINOR bumps may include breaking changes
- PATCH bumps are backward compatible

**Post-1.0 Rules**:
- `1.0.0` = First stable release
- Strict semantic versioning
- MAJOR bumps for breaking changes only

## Release Checklist

Before merging a Release PR:

- [ ] Review CHANGELOG entries
- [ ] Verify version bump is correct
- [ ] Check that breaking changes are documented
- [ ] Ensure all CI checks pass
- [ ] Test locally if possible (snapshot build)
- [ ] Update documentation if needed

After release:

- [ ] Verify GitHub Release was created
- [ ] Check that artifacts are attached
- [ ] Verify Docker images are published
- [ ] Test download and installation
- [ ] Announce release (optional)

## Troubleshooting

### Release Please PR Not Created

**Check**:
1. Commits follow Conventional Commits format
2. Commits are on `master` branch
3. `RELEASE_PLEASE_TOKEN` secret is set
4. GitHub Actions are enabled

**Debug**:
```bash
# View workflow logs
# Go to: https://github.com/duckpuppy/comic-server/actions

# Check recent commits
git log --oneline -10
```

### GoReleaser Build Fails

**Common Issues**:
1. Tests failing (`go test ./...`)
2. Missing dependencies (`go mod tidy`)
3. Invalid version format
4. Platform-specific build errors

**Debug**:
```bash
# Run tests locally
just test

# Test build locally
goreleaser build --snapshot --clean --single-target
```

### Docker Build Fails

**Common Issues**:
1. Dockerfile syntax errors
2. Multi-platform build issues
3. Registry authentication

**Debug**:
```bash
# Build locally
docker build -t comic-server:test .

# Multi-platform build
docker buildx build --platform linux/amd64,linux/arm64 .
```

## See Also

- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [Release Please Documentation](https://github.com/googleapis/release-please)
- [GoReleaser Documentation](https://goreleaser.com/)
- [CLAUDE.md](../CLAUDE.md) - Development guide
- [CHANGELOG.md](../CHANGELOG.md) - Release history
