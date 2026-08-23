#!/bin/sh
# Cross-builds the Windows desktop binary and, when makensis is available,
# produces DNSer_<version>_windows_amd64_setup.exe.
# Usage: scripts/package/windows.sh <version> [output-dir]
set -eu

VERSION="${1:?version required}"
OUT_DIR="${2:-dist}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

cd "$ROOT"

echo "==> building web UI"
pnpm --dir web install --frozen-lockfile --silent
pnpm --dir web build

echo "==> building dnser-desktop (windows/amd64)"
mkdir -p "$OUT_DIR/bin"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags desktop -trimpath \
  -ldflags "-s -w -X github.com/SDK-E/dnser/internal/cli.version=$VERSION -X main.version=$VERSION" \
  -o "$OUT_DIR/bin/dnser-desktop.exe" ./cmd/dnser-desktop

if ! command -v makensis >/dev/null 2>&1; then
  echo "==> makensis not found; skipping installer (binary at $OUT_DIR/bin/dnser-desktop.exe)"
  exit 0
fi

WIN_VERSION="$(echo "$VERSION" | sed 's/[^0-9.].*$//' | awk -F. '{printf "%d.%d.%d.%d", $1, $2, $3, ($4 == "" ? 0 : $4)}')"

echo "==> building NSIS installer"
makensis \
  -DVERSION="$VERSION" \
  -DWIN_VERSION="$WIN_VERSION" \
  -DEXE_PATH="$(cygpath -w "$ROOT/$OUT_DIR/bin/dnser-desktop.exe")" \
  -DICON_PATH="$(cygpath -w "$ROOT/packaging/assets/icon.ico")" \
  -DOUT_FILE="$(cygpath -w "$ROOT/$OUT_DIR/DNSer_${VERSION}_windows_amd64_setup.exe")" \
  packaging/windows/installer.nsi

echo "==> done: $OUT_DIR/DNSer_${VERSION}_windows_amd64_setup.exe"
