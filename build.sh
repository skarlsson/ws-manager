#!/bin/bash
set -euo pipefail

export PATH=$PATH:/usr/local/go/bin

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS="-s -w -X github.com/skarlsson/workshell/cmd.Version=${VERSION} -X github.com/skarlsson/workshell/cmd.BuildTime=${BUILD_TIME}"

echo "Building ws ${VERSION}..."
go build -ldflags "${LDFLAGS}" -o ws .

echo "Built: ./ws"
./ws version

if [ "${1:-}" = "install" ]; then
    INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
    mkdir -p "$INSTALL_DIR"
    cp ws "$INSTALL_DIR/ws"
    echo "Installed: $INSTALL_DIR/ws"
fi
