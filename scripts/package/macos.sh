#!/bin/sh
# Builds DNSer.app and a DMG from the desktop binary.
# Usage: scripts/package/macos.sh <version> [output-dir]
# Requires: macOS with Xcode CLT (hdiutil, iconutil), pnpm, go
set -eu

VERSION="${1:?version required, e.g. 1.2.3}"
OUT_DIR="${2:-dist}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
APP_NAME="DNSer.app"
STAGING="$OUT_DIR/macos-staging"

cd "$ROOT"

echo "==> building web UI"
pnpm --dir web install --frozen-lockfile --silent
pnpm --dir web build

echo "==> building dnser-desktop (darwin/$(uname -m))"
ARCH="$(uname -m)"
case "$ARCH" in
  arm64) GOARCH=arm64 ;;
  x86_64) GOARCH=amd64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac
mkdir -p "$OUT_DIR/bin"
CGO_ENABLED=1 GOARCH=$GOARCH go build -tags desktop -trimpath \
  -ldflags "-s -w -X github.com/SDK-E/dnser/internal/cli.version=$VERSION -X main.version=$VERSION" \
  -o "$OUT_DIR/bin/dnser-desktop" ./cmd/dnser-desktop

echo "==> assembling $APP_NAME"
rm -rf "$STAGING"
APP="$STAGING/$APP_NAME"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

sed "s/{{VERSION}}/$VERSION/g" packaging/macos/Info.plist > "$APP/Contents/Info.plist"

ICONSET="$OUT_DIR/dnser.iconset"
rm -rf "$ICONSET"
mkdir -p "$ICONSET"
cp packaging/assets/icon_016.png "$ICONSET/icon_16x16.png"
cp packaging/assets/icon_032.png "$ICONSET/icon_16x16@2x.png"
cp packaging/assets/icon_032.png "$ICONSET/icon_32x32.png"
cp packaging/assets/icon_064.png "$ICONSET/icon_32x32@2x.png"
cp packaging/assets/icon_128.png "$ICONSET/icon_128x128.png"
cp packaging/assets/icon_256.png "$ICONSET/icon_128x128@2x.png"
cp packaging/assets/icon_256.png "$ICONSET/icon_256x256.png"
cp packaging/assets/icon_512.png "$ICONSET/icon_256x256@2x.png"
cp packaging/assets/icon_512.png "$ICONSET/icon_512x512.png"
cp packaging/assets/icon_1024.png "$ICONSET/icon_512x512@2x.png"
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/dnser.icns"
rm -rf "$ICONSET"

cp "$OUT_DIR/bin/dnser-desktop" "$APP/Contents/MacOS/dnser-desktop"
chmod +x "$APP/Contents/MacOS/dnser-desktop"

codesign --force --deep --sign - "$APP" >/dev/null 2>&1 || true

echo "==> creating DMG"
ln -sfn /Applications "$STAGING/Applications"
DMG="$OUT_DIR/DNSer_${VERSION}_macOS_${GOARCH}.dmg"
rm -f "$DMG"
hdiutil create -volname "DNSer $VERSION" -srcfolder "$STAGING" -ov -format UDZO "$DMG" >/dev/null

rm -rf "$STAGING"
echo "==> done: $DMG"
