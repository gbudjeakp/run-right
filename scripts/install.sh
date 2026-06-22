#!/usr/bin/env bash
# Download and install the runright binary.
# Env vars:
#   RUNRIGHT_VERSION  - tag to install (e.g. "v1.0.5") or "latest"
set -euo pipefail

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)       ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

VERSION="${RUNRIGHT_VERSION:-latest}"
REPO="gbudjeakp/run-right"
BIN_DIR="$HOME/.runright/bin"
mkdir -p "$BIN_DIR"

if [[ "$VERSION" == "latest" ]]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
fi

TARBALL="runright_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
curl -fsSL "$URL" -o /tmp/runright.tar.gz
tar -xzf /tmp/runright.tar.gz -C "$BIN_DIR" runright
chmod +x "$BIN_DIR/runright"
echo "$BIN_DIR" >> "$GITHUB_PATH"
echo "version=$VERSION" >> "$GITHUB_OUTPUT"
