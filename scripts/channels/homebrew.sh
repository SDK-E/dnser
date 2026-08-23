#!/bin/sh
# Updates the Homebrew tap: CLI formula + GUI cask for the tagged release.
# Requires env: HOMEBREW_TAP_REPO ("owner/repo"), HOMEBREW_TOKEN (PAT with repo scope).
set -eu

TAG="${GITHUB_REF_NAME:?must run on a tag ref}"
VERSION="${TAG#v}"
REPO="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY missing}"
TAP="${HOMEBREW_TAP_REPO:?HOMEBREW_TAP_REPO not configured}"
TOKEN="${HOMEBREW_TOKEN:?HOMEBREW_TOKEN not configured}"

BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

fetch_sha() {
  url="$1"
  tmp=$(mktemp)
  curl -fsSL "$url" -o "$tmp"
  sha256sum "$tmp" | awk '{print $1}'
  rm -f "$tmp"
}

SHA_ARM=$(fetch_sha "${BASE_URL}/dnser_${VERSION}_Darwin_arm64.tar.gz")
SHA_INTEL=$(fetch_sha "${BASE_URL}/dnser_${VERSION}_Darwin_amd64.tar.gz")
SHA_DMG_ARM=$(fetch_sha "${BASE_URL}/DNSer_${VERSION}_macOS_arm64.dmg")
SHA_DMG_INTEL=$(fetch_sha "${BASE_URL}/DNSer_${VERSION}_macOS_amd64.dmg")

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
git clone --depth 1 "https://x-access-token:${TOKEN}@github.com/${TAP}.git" "$WORK/tap" >/dev/null 2>&1

mkdir -p "$WORK/tap/Formula" "$WORK/tap/Casks"

sed \
  -e "s|{{VERSION}}|$VERSION|g" \
  -e "s|{{URL_BASE}}|$BASE_URL|g" \
  -e "s|{{SHA_ARM}}|$SHA_ARM|g" \
  -e "s|{{SHA_INTEL}}|$SHA_INTEL|g" \
  packaging/homebrew/dnser.rb.tmpl > "$WORK/tap/Formula/dnser.rb"

sed \
  -e "s|{{VERSION}}|$VERSION|g" \
  -e "s|{{URL_BASE}}|$BASE_URL|g" \
  -e "s|{{SHA_DMG_ARM}}|$SHA_DMG_ARM|g" \
  -e "s|{{SHA_DMG_INTEL}}|$SHA_DMG_INTEL|g" \
  packaging/homebrew/dnser-desktop.rb.tmpl > "$WORK/tap/Casks/dnser-desktop.rb"

cd "$WORK/tap"
git add Formula Casks
if git diff --cached --quiet; then
  echo "tap already up to date at $VERSION"
  exit 0
fi
git -c user.name="dnser-release-bot" -c user.email="hicham@sdk.enterprises" \
  commit -m "dnser $VERSION"
git push origin HEAD >/dev/null 2>&1
echo "pushed formula + cask for $VERSION to $TAP"
