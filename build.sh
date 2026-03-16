#!/bin/bash
set -euo pipefail

export PATH=$PATH:/usr/local/go/bin

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS="-s -w -X github.com/skarlsson/workshell/cmd.Version=${VERSION} -X github.com/skarlsson/workshell/cmd.BuildTime=${BUILD_TIME}"

# Bundle window-calls GNOME extension for Wayland support
EMBED_DIR="internal/deps/embed"
mkdir -p "$EMBED_DIR"
WINDOW_CALLS_ZIP="$EMBED_DIR/window-calls.zip"
if [ ! -f "$WINDOW_CALLS_ZIP" ]; then
    echo "Downloading window-calls GNOME extension..."
    TMPDIR=$(mktemp -d)
    trap "rm -rf $TMPDIR" EXIT
    BASE_URL="https://raw.githubusercontent.com/ickyicky/window-calls/master"
    curl -fsSL -o "$TMPDIR/extension.js" "$BASE_URL/extension.js"
    curl -fsSL -o "$TMPDIR/metadata.json" "$BASE_URL/metadata.json"
    (cd "$TMPDIR" && zip -q "$OLDPWD/$WINDOW_CALLS_ZIP" extension.js metadata.json)
    echo "Bundled: $WINDOW_CALLS_ZIP"
fi

echo "Building ws ${VERSION}..."
go build -ldflags "${LDFLAGS}" -o ws .

echo "Built: ./ws"
./ws version

if [ "${1:-}" = "install" ]; then
    INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
    mkdir -p "$INSTALL_DIR"
    rm -f "$INSTALL_DIR/ws"
    mv ws "$INSTALL_DIR/ws"
    echo "Installed: $INSTALL_DIR/ws"
fi
