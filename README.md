# FastBrew 🚀

A lightning-fast, modern alternative interface for Homebrew (Linuxbrew).

## Features

*   **Instant Search**: Local caching of Homebrew's JSON index allows for zero-latency fuzzy search.
*   **Parallel Downloads**: Fetches bottles in parallel before installing, significantly speeding up large installations.
*   **Resume Downloads**: Automatically resumes interrupted downloads using HTTP Range requests.
*   **Modern TUI**: A beautiful, keyboard-driven terminal interface powered by Bubbletea.
*   **Services Management**: Start, stop, restart, and list services (launchd on macOS, systemd on Linux).
*   **Package Pinning**: Pin packages to prevent upgrades, unpin when ready.
*   **Shell Completions**: Native completions for bash, zsh, fish, and PowerShell.
*   **Autoremove**: Clean up orphaned dependencies that are no longer needed.
*   **Configuration**: Customize behavior via `~/.fastbrew/config.json`.
*   **Compatibility**: Uses `brew` under the hood for final installation, ensuring full compatibility with your existing system.

## Usage

### CLI Mode

```bash
# Instant search
fastbrew search python

# Parallel install
fastbrew install python nodejs go
```

### Interactive Mode (TUI)

Just run `fastbrew` to open the interactive dashboard.
*   Type to filter packages.
*   Press `Enter` to install the selected package.
*   `Ctrl+C` to quit.

### Services Management

```bash
# List all services
fastbrew services list

# Start/stop/restart a service
fastbrew services start postgresql
fastbrew services stop postgresql
fastbrew services restart postgresql
```

### Package Pinning

```bash
# Pin a package to prevent upgrades
fastbrew pin node

# List pinned packages
fastbrew pinned

# Unpin a package
fastbrew unpin node
```

### Cleanup

```bash
# Remove orphaned dependencies
fastbrew autoremove

# Preview what would be removed
fastbrew autoremove --dry-run
```

### Configuration

```bash
# Show current configuration
fastbrew config show

# Set configuration values
fastbrew config set parallel_downloads 20
fastbrew config set show_progress true
```

Configuration is stored at `~/.fastbrew/config.json`.

### Shell Completions

```bash
# Bash
fastbrew completion bash > /etc/bash_completion.d/fastbrew

# Zsh
fastbrew completion zsh > "${fpath[1]}/_fastbrew"

# Fish
fastbrew completion fish > ~/.config/fish/completions/fastbrew.fish

# PowerShell
fastbrew completion powershell > fastbrew.ps1
```

## Installation

### Method 1: Homebrew (Recommended)

```bash
brew tap kasyap1234/homebrew-tap
brew install fastbrew
```

### Method 2: Curl One-Liner

```bash
curl -fsSL https://raw.githubusercontent.com/kasyap1234/fastbrew/main/install.sh | bash
```

### Method 3: From Source

Requires [Go](https://go.dev/doc/install) 1.21+

```bash
git clone https://github.com/kasyap1234/fastbrew.git
cd fastbrew
go build -o fastbrew main.go
sudo mv fastbrew /usr/local/bin/
```

