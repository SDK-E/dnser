#!/bin/sh
# Builds deb/rpm via nfpm and, when running on Linux with appimagetool
# available, an x86_64 AppImage.
# Usage: scripts/package/linux.sh <version> [output-dir] [amd64|arm64]
#
# The GUI binary needs CGO against GTK/WebKit, so the compile step must run
# on Linux (native or in a container). Set SKIP_BUILD=1 to package a binary
# already present at dist/bin/dnser-desktop-linux-<arch>.
set -eu

VERSION="${1:?version required}"
OUT_DIR="${2:-dist}"
GOARCH="${3:-amd64}"

case "$GOARCH" in
  amd64|arm64) NFPM_ARCH="$GOARCH" ;;
  arm) GOARCH=arm64; NFPM_ARCH=arm64 ;;
  *) echo "unsupported arch: $GOARCH"; exit 1 ;;
esac

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
BIN="$ROOT/$OUT_DIR/bin/dnser-desktop-linux-$GOARCH"

if [ "${SKIP_BUILD:-0}" != "1" ]; then
  echo "==> building web UI"
  pnpm --dir web install --frozen-lockfile --silent
  pnpm --dir web build

  echo "==> building dnser-desktop (linux/$GOARCH)"
  mkdir -p "$OUT_DIR/bin"
  CGO_ENABLED=1 GOOS=linux GOARCH="$GOARCH" go build -tags "desktop,gtk3" -trimpath \
    -ldflags "-s -w -X github.com/SDK-E/dnser/internal/cli.version=$VERSION -X main.version=$VERSION" \
    -o "$BIN" ./cmd/dnser-desktop
fi

if [ ! -x "$BIN" ]; then
  echo "error: $BIN missing; the linux GUI build requires CGO and must run" >&2
  echo "on Linux (native or container). Build it there first, or set SKIP_BUILD=1" >&2
  echo "with the binary already in place." >&2
  exit 1
fi

if ! command -v nfpm >/dev/null 2>&1; then
  echo "==> nfpm not found (install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest); skipping deb/rpm"
else
  echo "==> building deb + rpm ($NFPM_ARCH)"
  NFPM_CFG="$OUT_DIR/nfpm.yaml"
  sed \
    -e "s|\${BIN_PATH}|$BIN|g" \
    -e "s|\${VERSION}|$VERSION|g" \
    -e "s|\${ARM}|$NFPM_ARCH|g" \
    -e "s|\${DESKTOP_FILE}|$ROOT/packaging/linux/dnser-desktop.desktop|g" \
    -e "s|\${ICON_256}|$ROOT/packaging/assets/icon_256.png|g" \
    -e "s|\${ICON_512}|$ROOT/packaging/assets/icon_512.png|g" \
    packaging/linux/nfpm.yaml.tmpl > "$NFPM_CFG"
  nfpm package -f "$NFPM_CFG" -p deb -t "$OUT_DIR/dnser-desktop_${VERSION}_linux_${NFPM_ARCH}.deb"
  nfpm package -f "$NFPM_CFG" -p rpm -t "$OUT_DIR/dnser-desktop_${VERSION}_linux_${NFPM_ARCH}.rpm"
fi

APPDIR="$OUT_DIR/DNSer.AppDir"

if [ "$(uname -s)" != "Linux" ]; then
  echo "==> AppImage skipped (requires Linux; CI builds it natively)"
  exit 0
fi

echo "==> assembling AppDir"
rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/icons/hicolor/256x256/apps"
cp "$BIN" "$APPDIR/usr/bin/dnser-desktop"
cp packaging/linux/dnser-desktop.desktop "$APPDIR/dnser.desktop"
cp packaging/assets/icon_256.png "$APPDIR/usr/share/icons/hicolor/256x256/apps/dnser.png"
cp packaging/assets/icon_256.png "$APPDIR/.DirIcon"

if command -v appimagetool >/dev/null 2>&1; then
  TOOL=appimagetool
elif [ -x "$OUT_DIR/appimagetool" ]; then
  TOOL="$OUT_DIR/appimagetool"
  chmod +x "$TOOL"
else
  echo "==> appimagetool not found; AppDir left at $APPDIR"
  exit 0
fi
VERSION="$VERSION" ARCH="$NFPM_ARCH" "$TOOL" --comp xz "$APPDIR" \
  "$OUT_DIR/DNSer_${VERSION}_linux_${GOARCH}.AppImage"
rm -rf "$APPDIR"
echo "==> done: $OUT_DIR/DNSer_${VERSION}_linux_${GOARCH}.AppImage"
