#!/bin/bash
set -euo pipefail

REPO="skarlsson/workshell"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

SUFFIX="${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/ws-${SUFFIX}"

echo "Downloading ws-${SUFFIX}..."
mkdir -p "$INSTALL_DIR"
curl -fsSL "$URL" -o "${INSTALL_DIR}/ws"
chmod +x "${INSTALL_DIR}/ws"

echo "Installed: ${INSTALL_DIR}/ws"
"${INSTALL_DIR}/ws" version

if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo ""
    echo "Add to your PATH if not already:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi

# Post-install: dependencies then keybindings
echo ""
echo "Installing dependencies..."
"${INSTALL_DIR}/ws" deps install

if command -v gsettings &>/dev/null; then
    echo ""
    echo "Applying GNOME keybindings..."
    "${INSTALL_DIR}/ws" keybindings
fi
