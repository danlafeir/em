#!/bin/sh
set -e

REPO=danlafeir/devctl-em
BINARY=devctl-em
INSTALL_DIR=~/.local/bin

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux) OS=linux ;;
  darwin) OS=darwin ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect ARCH
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Find the latest binary for this OS/ARCH by querying the GitHub API
# Builds use the pattern: devctl-em-<os>-<arch>-<git hash>
API_URL="https://api.github.com/repos/$REPO/contents/bin/"
FILENAME=$(curl -sSL "$API_URL" | grep -o '"name": *"'$BINARY'-'$OS'-'$ARCH'-[a-zA-Z0-9]*"' | sed 's/.*: *"//;s/"//' | sort | tail -n1)
if [ -z "$FILENAME" ]; then
  echo "Could not find a release binary for $OS/$ARCH." >&2
  exit 1
fi
URL="https://raw.githubusercontent.com/$REPO/main/bin/$FILENAME"

TMP=$(mktemp)
echo "Downloading $URL ..."
curl -sSLfL "$URL" -o "$TMP"
chmod +x "$TMP"

# Move to install dir (may require sudo)
echo "Installing to $INSTALL_DIR/$BINARY ..."
mv "$TMP" "$INSTALL_DIR/$BINARY"

if command -v $BINARY >/dev/null 2>&1; then
  echo "$BINARY installed successfully!"
  echo "Ensure $INSTALL_DIR is in your PATH."
else
  echo "Install failed: $BINARY not found in PATH." >&2
  exit 1
fi
