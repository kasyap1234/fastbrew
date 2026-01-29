#!/bin/bash
# FastBrew Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/kasyap1234/fastbrew/main/install.sh | bash

set -e

REPO="kasyap1234/fastbrew"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
    linux)  OS="Linux" ;;
    darwin) OS="Darwin" ;;
    *)      echo "❌ Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
    x86_64)  ARCH="x86_64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "🚀 Installing FastBrew..."

# Get latest release version from GitHub API
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
    echo "❌ Could not determine latest version"
    exit 1
fi

echo "📦 Version: v${VERSION}"
echo "🖥️  Platform: ${OS}/${ARCH}"

# Construct download URL
TARBALL="fastbrew_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${TARBALL}"

# Create temp directory
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

# Download and extract
echo "⬇️  Downloading from ${URL}..."
curl -fsSL "$URL" -o "$TMP_DIR/$TARBALL"
tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"

# Install
echo "📂 Installing to ${INSTALL_DIR}..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/fastbrew" "$INSTALL_DIR/"
else
    sudo mv "$TMP_DIR/fastbrew" "$INSTALL_DIR/"
fi

chmod +x "$INSTALL_DIR/fastbrew"

echo ""
echo "✅ FastBrew v${VERSION} installed successfully!"
echo ""
echo "Try: fastbrew search python"
echo "     fastbrew install cowsay"
