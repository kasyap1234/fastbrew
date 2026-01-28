# FastBrew 🚀

A lightning-fast, modern alternative interface for Homebrew (Linuxbrew).

## Features
*   **Instant Search**: Local caching of Homebrew's JSON index allows for zero-latency fuzzy search.
*   **Parallel Downloads**: Fetches bottles in parallel before installing, significantly speeding up large installations.
*   **Modern TUI**: A beautiful, keyboard-driven terminal interface powered by Bubbletea.
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

## Installation

```bash
go build -o fastbrew main.go
sudo mv fastbrew /usr/local/bin/
```
