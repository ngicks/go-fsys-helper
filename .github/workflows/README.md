# GitHub Actions Workflows

This directory contains GitHub Actions workflows for the go-fsys-helper monorepo.

## Workflows

### 1. Test (`test.yml`)

**Triggers:**
- Push to `main` branch
- Pull requests to `main` branch

**What it does:**
- Detects which Go modules have been changed
- Runs tests across multiple platforms and architectures:
  - Linux AMD64 (ubuntu-latest)
  - Linux ARM64 (ubuntu-latest with QEMU emulation)
  - macOS AMD64 (macos-latest)
  - macOS ARM64 (macos-14)
  - Windows AMD64 (windows-latest)
- Only tests affected modules to save CI time
- Runs tests with race detection (except ARM64 emulation)
- Verifies dependencies with `go mod verify`
- Runs `go vet` for static analysis
- Checks code formatting with `go fmt`

### 2. Security (`security.yml`)

**Triggers:**
- Push to `main` branch
- Pull requests to `main` branch
- Weekly schedule (Sundays at 00:00 UTC)

**What it does:**
- Scans for security vulnerabilities using Gosec
- Checks for known vulnerabilities with govulncheck
- Uploads SARIF reports to GitHub Security tab
- For scheduled runs, scans all modules
- For PRs/pushes, only scans affected modules

## Module Detection

All workflows use intelligent module detection:

1. **Find all modules:** Searches for `go.mod` files in the repository
2. **Exclude frozen modules:** Automatically excludes `aferofs` (frozen/deprecated)
3. **Detect changes:** Compares current commit with base to find changed files
4. **Map to modules:** Determines which modules contain the changed files
5. **Fallback logic:** If root-level Go files change, tests all active modules

### Active Modules

The following modules are actively tested:
- `fsutil` - Filesystem utilities
- `stream` - Streaming utilities  
- `tarfs` - TAR filesystem implementation
- `vroot` - Virtual root filesystem abstraction

### Excluded Modules

- `aferofs` - Frozen/deprecated module (excluded from all workflows)

## Configuration

### Basic Linting

The workflows use standard Go tools:
- `go vet` for static analysis
- `go fmt` for code formatting
- `go mod verify` for dependency verification

### Platform Support

- **Linux AMD64:** Native execution on ubuntu-latest
- **Linux ARM64:** Emulated execution with QEMU (slower, longer timeout)
- **macOS AMD64:** Native execution on macos-latest
- **macOS ARM64:** Native execution on macos-14 (Apple Silicon)
- **Windows AMD64:** Native execution on windows-latest

## Performance Optimizations

- **Change detection:** Only affected modules are tested
- **Dependency caching:** Go modules are cached based on go.sum files
- **Parallel execution:** Multiple modules and platforms run concurrently
- **Fail-fast disabled:** One failure doesn't stop other tests

## Customization

To modify the workflows:

1. **Add/remove platforms:** Edit the matrix in `test.yml`
2. **Change linting rules:** Modify `.golangci.yml`
3. **Adjust timeouts:** Update timeout values in workflow files
4. **Add security tools:** Extend the security workflow

## Status Checks

All workflows provide summary steps that:
- Report when no modules are affected
- Provide clear success/failure indicators
- Enable easy status checking in PR reviews

## Go Version

All workflows use Go 1.22 for active modules. Update the `go-version` field in all workflow files to change this. The frozen `aferofs` module uses Go 1.23 but is excluded from workflows.