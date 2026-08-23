#!/bin/sh
# Updates AUR packages for the tagged release.
# Requires env: AUR_SSH_KEY (base64-encoded private key with push access).
# Expects repos: dnser-bin and dnser-desktop-bin under the key's account;
# first-time package creation is manual (see packaging/README.md).
set -eu

TAG="${GITHUB_REF_NAME:?must run on a tag ref}"
VERSION="${TAG#v}"
REPO="${GITHUB_REPOSITORY:?}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
[ -n "${AUR_SSH_KEY:-}" ] || { echo "AUR_SSH_KEY not configured; skipping"; exit 0; }

KEY=$(mktemp)
printf '%s\n' "$AUR_SSH_KEY" | base64 -d > "$KEY"
chmod 600 "$KEY"
trap 'rm -f "$KEY"' EXIT

mkdir -p ~/.ssh
cat > ~/.ssh/config <<EOF
Host aur.archlinux.org
  User aur
  IdentityFile $KEY
  StrictHostKeyChecking accept-new
EOF

URL_BASE="https://github.com/${REPO}/releases/download/${TAG}"

fetch_sha() {
  tmp=$(mktemp)
  curl -fsSL "$1" -o "$tmp"
  sha256sum "$tmp" | awk '{print $1}'
  rm -f "$tmp"
}
SHA_AMD64=$(fetch_sha "${URL_BASE}/dnser_${VERSION}_Linux_amd64.tar.gz")
SHA_ARM64=$(fetch_sha "${URL_BASE}/dnser_${VERSION}_Linux_arm64.tar.gz")
SHA_APP_AMD64=$(fetch_sha "${URL_BASE}/DNSer_${VERSION}_linux_amd64.AppImage")
SHA_APP_ARM64=$(fetch_sha "${URL_BASE}/DNSer_${VERSION}_linux_arm64.AppImage")

render_and_push() {
  name="$1"
  tmpl="$ROOT/packaging/aur/${name}.PKGBUILD.tmpl"
  work=$(mktemp -d)

  git clone --depth 1 "aur@aur.archlinux.org:${name}.git" "$work/$name" >/dev/null 2>&1
  sed -e "s|^pkgver=.*|pkgver=${VERSION}|" \
      -e "s|{{VERSION}}|${VERSION}|g" \
      -e "s|{{URL_BASE}}|${URL_BASE}|g" \
      -e "s|{{SHA_AMD64}}|$( [ "$name" = dnser-bin ] && echo "$SHA_AMD64" || echo "$SHA_APP_AMD64" )|g" \
      -e "s|{{SHA_ARM64}}|$( [ "$name" = dnser-bin ] && echo "$SHA_ARM64" || echo "$SHA_APP_ARM64" )|g" \
      "$tmpl" > "$work/$name/PKGBUILD"

  docker run --rm -v "$work/$name":/pkg -w /pkg archlinux:base-devel \
    sh -c 'makepkg --printsrcinfo > .SRCINFO'

  cd "$work/$name"
  git add PKGBUILD .SRCINFO
  if git diff --cached --quiet; then
    echo "$name already at $VERSION"
    return 0
  fi
  git -c user.name="dnser-release-bot" -c user.email="hicham@sdk.enterprises" commit -m "update to $VERSION"
  git push origin master >/dev/null 2>&1
  echo "pushed $name $VERSION"
  cd /
  rm -rf "$work"
}

( set -e; render_and_push dnser-bin )
( set -e; render_and_push dnser-desktop-bin )
