#!/bin/sh
# Submits a winget manifest update PR for the desktop installer.
# Requires env: WINGET_PACKAGE_ID (e.g. SDK.Enterprises.DNSer), WINGET_TOKEN.
# The package id must be registered once manually via a first PR to microsoft/winget-pkgs.
set -eu

TAG="${GITHUB_REF_NAME:?must run on a tag ref}"
VERSION="${TAG#v}"
REPO="${GITHUB_REPOSITORY:?}"
[ -n "${WINGET_PACKAGE_ID:-}" ] || { echo "WINGET_PACKAGE_ID not configured; skipping"; exit 0; }
[ -n "${WINGET_TOKEN:-}" ] || { echo "WINGET_TOKEN not configured; skipping"; exit 0; }

URL="https://github.com/${REPO}/releases/download/${TAG}/DNSer_${VERSION}_windows_amd64_setup.exe"

curl -fsSL https://aka.ms/wingetcreate/latest -o /tmp/wingetcreate.exe
chmod +x /tmp/wingetcreate.exe
/tmp/wingetcreate.exe update "$WINGET_PACKAGE_ID" \
  --version "$VERSION" \
  --url "$URL" \
  --submit-pr \
  --token "$WINGET_TOKEN"
