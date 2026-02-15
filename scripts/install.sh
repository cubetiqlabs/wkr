#!/bin/sh
set -e

# Cubis Workers CLI installer
# Usage: curl -fsSL https://raw.githubusercontent.com/cubetiqlabs/wkr/main/scripts/install.sh | sh

REPO="cubetiqlabs/wkr"
BINARY="wkr"
INSTALL_DIR="/usr/local/bin"

# Detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Windows detection (Git Bash / MSYS)
case "$OS" in
  mingw*|msys*|cygwin*) OS="windows" ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  ASSET="${ASSET}.exe"
fi

# Get latest wkr-cli release tag
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=20" \
  | grep -o '"tag_name": *"wkr-cli-v[^"]*"' | head -1 | grep -o 'wkr-cli-v[^"]*')

if [ -z "$LATEST" ]; then
  echo "Error: could not find a wkr-cli release"
  exit 1
fi

URL="https://github.com/${REPO}/releases/download/${LATEST}/${ASSET}"

echo "Installing ${BINARY} ${LATEST} (${OS}/${ARCH})..."

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL -o "${TMP}/${BINARY}" "$URL"
chmod +x "${TMP}/${BINARY}"

# Install — try sudo if needed
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Need sudo to install to ${INSTALL_DIR}"
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "✓ Installed ${BINARY} ${LATEST} to ${INSTALL_DIR}/${BINARY}"
echo "  Run 'wkr help' to get started."
