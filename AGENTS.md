# AGENTS.md - FastBrew Development Guide

## Project Overview

FastBrew is a high-performance CLI wrapper for Homebrew written in Go. It features parallel downloads, instant fuzzy search via local caching, a modern TUI interface, and services management.

## Build Commands

### Building
```bash
# Build the binary
make build

# Build with custom version
go build -ldflags "-X fastbrew/cmd.Version=v1.0.0" -o fastbrew main.go

# Build all packages
go build -v ./...

# Install locally
make install
go install
```

### Running
```bash
# Run the binary
make run
./fastbrew

# Run with arguments
./fastbrew install python node
```

### Testing
```bash
# Run all tests
go test ./...

# Run all tests verbosely
go test -v ./...

# Run a single test
go test -run TestFunctionName ./...

# Run tests in a specific package
go test -v ./internal/config/...

# Run tests matching a pattern
go test -run "Test.*" ./...

# Run tests with coverage
go test -cover ./...
```

### Linting & Formatting
```bash
# Format code (built into Go)
gofmt -w .

# Run go vet
go vet ./...

# Tidy go.mod
go mod tidy
```

### Release
```bash
# Check goreleaser config
make release-check

# Build snapshot release locally
make release-snapshot
```

## Code Style Guidelines

### General
- **Language**: Go 1.26+
- **Testing**: Standard Go `testing` package
- **CLI Framework**: spf13/cobra
- **TUI Framework**: charmbracelet/bubbletea

### Package Structure
```
cmd/           - CLI command implementations
internal/      - Private application code
  brew/        - Homebrew client and index management
  config/      - Configuration management
  httpclient/  - HTTP client with retry logic
  progress/    - Download progress tracking
  resume/      - Resumable download support
  retry/       - Retry utilities
  services/    - Systemd/launchd service management
  bundle/      - Bundle support
  tui/         - Terminal UI components
```

### Imports
Group imports in the following order (blank line between groups):
1. Standard library
2. External packages (github.com, etc.)

```go
import (
    "bufio"
    "bytes"
    "fmt"
    "os"
    "path/filepath"

    "fastbrew/internal/config"
    "fastbrew/internal/progress"

    "github.com/charmbracelet/bubbletea"
    "github.com/spf13/cobra"
)
```

### Naming Conventions
- **Files**: lowercase with underscores (e.g., `index_update_test.go`)
- **Packages**: lowercase, short names (e.g., `brew`, `config`)
- **Exported functions/types**: PascalCase (e.g., `NewClient`, `PackageInfo`)
- **Unexported functions/variables**: camelCase (e.g., `getMaxParallel`)
- **Constants**: PascalCase or camelCase for unexported (e.g., `DefaultConfig`)
- **Struct fields**: camelCase for JSON serialization compatibility

### Error Handling
- Use `fmt.Errorf` with `%w` for wrapped errors
- Return errors from functions; handle at call site
- For CLI commands, print error and call `os.Exit(1)`
- Use sentinel errors for known error types

```go
// Good: wrapping errors
if err != nil {
    return nil, fmt.Errorf("could not find brew prefix: %w", err)
}

// Good: CLI error handling
if err := client.InstallNative(args); err != nil {
    fmt.Printf("Error installing packages: %v\n", err)
    os.Exit(1)
}
```

### Struct Tags
Use struct tags for JSON fields with camelCase:

```go
type PackageInfo struct {
    Name        string `json:"name"`
    Description string `json:"desc"`
    Homepage    string `json:"homepage,omitempty"`
    Installed   bool   `json:"installed"`
    Version     string `json:"version"`
    IsCask      bool   `json:"is_cask"`
}
```

### Testing Patterns
- Test files: `*_test.go` in same package
- Table-driven tests when appropriate
- Use `t.Fatalf` for setup failures, `t.Errorf` for assertion failures
- Use `t.TempDir()` for temporary test files
- Use `t.Setenv()` to set environment variables

```go
func TestExample(t *testing.T) {
    t.Setenv("HOME", t.TempDir())
    
    result, err := someFunction()
    if err != nil {
        t.Fatalf("Function failed: %v", err)
    }
    if result != expected {
        t.Errorf("Expected %v, got %v", expected, result)
    }
}
```

### Concurrency
- Use `sync.Once` for one-time initialization
- Use `sync.WaitGroup` for goroutine synchronization
- Use channels for communication between goroutines

### CLI Commands
- Use cobra for CLI structure
- Define commands in separate files under `cmd/`
- Use `Args` validators (e.g., `cobra.MinimumNArgs(1)`)
- Provide short and long descriptions

```go
var installCmd = &cobra.Command{
    Use:   "install [package...]",
    Short: "Install packages with parallel downloading",
    Args:  cobra.MinimumNArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        // implementation
    },
}
```

## Configuration
- Store config in `~/.fastbrew/config.json`
- Use JSON for configuration files
- Provide sensible defaults

## Dependencies
Key external packages:
- `github.com/spf13/cobra` - CLI framework
- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/charmbracelet/lipgloss` - Terminal styling
- `github.com/sahilm/fuzzy` - Fuzzy search
- `github.com/klauspost/compress` - Compression (zstd)
