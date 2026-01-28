#!/bin/bash

# FastBrew Installer
# Usage: curl -sL https://raw.githubusercontent.com/yourusername/fastbrew/main/install.sh | bash

set -e

REPO="fastbrew" 
VERSION="0.1.0"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [ "$ARCH" == "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" == "aarch64" ]; then
    ARCH="arm64"
fi

echo "🚀 Installing FastBrew v${VERSION}..."

# In a real scenario, this would download from GitHub Releases.
# For now, we'll simulate the build/install process or assume a binary URL.

echo "Downloading binary..."
# URL="https://github.com/yourusername/fastbrew/releases/download/v${VERSION}/fastbrew_${OS}_${ARCH}.tar.gz"
# curl -L $URL | tar xz

# Since we are local, let's just build it if go is installed, otherwise warn.
if command -v go >/dev/null 2>&1; then
    echo "Go found, building from source..."
    go build -o fastbrew main.go
else
    echo "❌ Go not found. Please download the pre-compiled binary from the releases page."
    exit 1
fi

echo "Installing to /usr/local/bin..."
sudo mv fastbrew /usr/local/bin/

echo "✅ FastBrew installed!"
echo "Try: fastbrew search python"
