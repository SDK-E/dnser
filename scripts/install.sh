#!/bin/sh
set -e

REPO="SDK-E/dnser"
INSTALL_DIR="${DNSER_INSTALL:-/usr/local/bin}"

echo "DNSer installer — SDK Enterprises"
echo

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "use winget or scoop on Windows"; exit 1 ;;
esac

if [ -z "$DNSER_VERSION" ]; then
  DNSER_URL="https://github.com/$REPO/releases/latest/download/dnser_Latest_${OS^}_$ARCH.tar.gz"
else
  DNSER_URL="https://github.com/$REPO/releases/download/$DNSER_VERSION/dnser_${DNSER_VERSION}_${OS^}_$ARCH.tar.gz"
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $DNSER_URL"
curl -fsSL "$DNSER_URL" | tar -xz -C "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP/dnser" "$INSTALL_DIR/dnser"
else
  echo "Installing to $INSTALL_DIR (requires sudo)"
  sudo mv "$TMP/dnser" "$INSTALL_DIR/dnser"
fi
chmod +x "$INSTALL_DIR/dnser"

echo
echo "Installed: $($INSTALL_DIR/dnser version)"
echo
echo "Next steps:"
echo "  dnser         guided first-run wizard"
echo "  dnser setup   configure system DNS + CA trust"
echo "  dnser link --domain=myproject --port=3000"
