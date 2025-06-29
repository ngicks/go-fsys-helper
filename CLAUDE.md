# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Mandatory

- If I'm asking for explanation, e.g. "explain", "why ~ ?"(case insensitive), I am just in need of explanation. DO NOT FIX at this point.

## Commands

- **Change directory**: Before executing any `go` command, `cd` into that `go module`. Don't run them on `./`. Each time after command is done, move back to root dir.
- **Testing**: Use `go test -coverprofile=./.coverinfo -timeout 2s ./...` to run all tests
- **Converage Report**: Use `go tool cover -html=./.coverinfo -o .coverage.html` after test is called.
- **Race testing**: Use `go test -race -timeout 2s ./...` to run tests with race detection (slower but finds timing issues)
- **Single test**: Use `go test -timeout 2s -run TestName -v ./path/to/package` to run a specific test
- **Single test with race detection**: Use `go test -race -timeout 2s -run TestName -v ./path/to/package` to run a specific test with race detection
- **Formatting / Clean imports**: Use `goimports -w .` to clean imports. Everytime you add or remove lines, run this.
- **Type checking**: Use `go vet ./...` to check for static analysis issues

## Architecture

This is a Go library (`github.com/ngicks/go-fsys-helper`) that provides filesystem helper utilities across multiple independent modules. Each major package is its own Go module with isolated dependencies.

### Active Packages

**vroot/**: Virtual root filesystem with security constraints (Go 1.25rc1)

- **Primary package** with comprehensive filesystem abstraction
- `Fs`: Core filesystem interface with OS-like operations (Chmod, Create, Open, etc.)
- `Rooted`: Strict containment preventing path traversal and symlink escapes
- `Unrooted`: Allows symlink escapes but blocks path traversal
- Multiple implementations:
  - `osfs/`: OS filesystem wrappers (Rooted and Unrooted)
  - `synthfs/`: Synthetic filesystem combining different data sources
  - `memfs/`: In-memory filesystem for testing
  - `overlayfs/`: Union mount with copy-on-write semantics
- Security models for different containment needs
- Extensive acceptance testing framework

**tarfs/**: TAR archive filesystem implementation (Go 1.24.0)

- Read-only filesystem interface for TAR archives
- Implements `fs.FS` for TAR file contents
- Handles sparse files, symlinks, and hardlinks
- Files implement `io.ReaderAt` and `io.Seeker`
- **Status**: Work-in-progress but functional

**fsutil/**: Filesystem abstraction utilities (Go 1.24.0)

- Interoperable utilities for filesystem abstraction libraries
- Compatible with afero, go-billy, hackpadfs, and vroot
- Interface definitions for filesystem operations
- Utility functions for path operations and error handling
- Safe write operations with temporary files and hooks
- Filesystem copying utilities with permission preservation

**stream/**: Stream helpers (Go 1.22.0)

- Utilities for `io.Reader`/`io.Writer` operations
- `NewCancellable`: Context-cancellable readers
- `NewMultiReadAtSeekCloser`: Virtual concatenation of `io.ReaderAt`
- `NewByteRepeater`: Infinite repeating byte reader
- No external dependencies (pure stdlib)

### Abandoned Packages

**aferofs/**: Afero filesystem adapters (Go 1.23.0)

- **Status**: FROZEN/ABANDONED - author is moving away from afero
- Previously bridged between afero and other filesystem abstractions
- Excluded from CI pipeline

**Testing Strategy**:

- `acceptancetest/`: Common test scenarios and filesystem fixtures
- Individual `*_test.go` files test specific components
- Tests use temporary directories with predefined file structures
- Use `./working` directory for temporary test files when needed

## Coding Preferences

**Test Code Style**:

- Use `must(osfs.NewRooted(path))` pattern instead of error handling for test setup that should always succeed
- Define named `testCase` struct types instead of anonymous struct literals in test tables
- Panic on fundamental test setup failures rather than using `t.Fatalf`

**Cross-Platform Testing**:

- When testing paths with platform-specific behavior (like absolute paths), use `runtime.GOOS` to provide different test cases for Windows vs Unix-like systems
- Pattern to follow:
  ```go
  if runtime.GOOS == "windows" {
      tests = append(tests, testCase{
          name:  "windows specific test",
          input: "C:\\path\\to\\file",
          expected: []string{"C:\\", "C:\\path", "C:\\path\\to", "C:\\path\\to\\file"},
      })
  } else {
      tests = append(tests, testCase{
          name:  "unix specific test",
          input: "/path/to/file",
          expected: []string{"/", "/path", "/path/to", "/path/to/file"},
      })
  }
  ```
- Always import `"runtime"` when using this pattern
- Remember that Windows uses backslashes (`\`) and drive letters (e.g., `C:\`) while Unix-like systems use forward slashes (`/`) and start from root (`/`)

**Cross-Platform Permissions**:

- Windows has different permission behavior than Unix-like systems
- **Unix-like systems**: Permissions are whatever you set, but newly created files are affected by umask (often system default is `0o022` but may vary)
- **Windows**: Often widens permissions regardless of what you set:
  - Directories: permissions often become `0o777` on Windows
  - Files: permissions often become `0o444` (read-only) or `0o666` (read-write) on Windows
- Pattern for permission tests:
  ```go
  expectedPerm := fs.FileMode(0o644)  // Unix: respects the set value (subject to umask)
  if runtime.GOOS == "windows" {
      expectedPerm = 0o666  // Windows: typically widens to read-write
  }
  if info.Mode().Perm() != expectedPerm {
      t.Errorf("permission mismatch: expected %o, got %o", expectedPerm, info.Mode().Perm())
  }
  ```
- Be aware that umask on Unix systems can affect the final permissions of newly created files
- On Windows, the permission you set is often ignored and replaced with Windows-appropriate values

## Path Handling

- Wrap every path with `filepath.FromSlash` when passing paths to functions.
- Wrap every path with `filepath.ToSlash` when using as test results (including `// Output:` comment in Example tests).

## Resource Management

- Close every opened resource right after becoming unused.

## Dependency Management

- Run `go mod tidy` after calling `go get` or removing dependency entries entirely from the module.

## Modern Go Practices

- Never use `sort` package. Use `slices.Sort` or `slices.SortFunc` instead.
- Use `fs.Stat` when taking stat from `fs.FS`.

## Error Handling

- Never use `os.IsNotExist` or similar functions. Instead, use `errors.Is(err, fs.ErrNotExist)` or similar `fs` errors.

## Test Assertions

- For tests, when comparing actual value and expected value, preferred message is `"not equal: expected(%q) != actual(%q)"`. If value is expected to print long message, use `"not equal:\nexpected: %#v\nactual: %#v\n"`.
- For error comparison, prefer using `"errors.Is(err, %v) does not satisfied:\nactual = %v\ndetailed = %#v"`. If message is expected to be long line, split each section by `"\n"`.

## Runtime and System Interaction

- Never use `runtime.GOROOT`. Instead, call `go env GOROOT` via `exec.CommandContext`.

## After You Think You Are Done

- Run test for an entire module to check implementation is ok.
- Run tests with race detection (`go test -race -timeout 2s ./...`) to catch timing issues and race conditions
- Run `../govet.sh` to check for static analysis issues and type checking problems **for all platforms**
  - return code of the command is almost always 0. So don't expect exit code other than 0 is failure.
  - If you can't solve cross platform problems by yourself, ask help for the user.
- If test fails and it is by the code you've made, fix the code.
- If failure is happening from user-written code, alert user about that and receive further instruction.
- Check `./.converinfo` and if converage is less than 90%, increase coverage.
- Check line count for each test files. If it's more than 400 lines, split test files to 300 - 400 lines per a file.
