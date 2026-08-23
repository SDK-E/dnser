# Packaging recipes

Locally reproducible installer builds for the desktop app. All scripts take a
plain version string (no `v` prefix) and an optional output directory
(default `dist/`).

| Target | Script | Tooling | Output |
|---|---|---|---|
| macOS | `scripts/package/macos.sh 1.2.3` | native (`hdiutil`, `iconutil`) | `dist/DNSer_<v>_macOS_<arch>.dmg` |
| Windows | `scripts/package/windows.sh 1.2.3` | `makensis` (optional; binary-only without it) | `dist/DNSer_<v>_windows_amd64_setup.exe` |
| Linux deb/rpm | `scripts/package/linux.sh 1.2.3 dist amd64` | `nfpm` | `dist/dnser-desktop_<v>_linux_<arch>.{deb,rpm}` |
| Linux AppImage | same script, on Linux | `appimagetool` | `dist/DNSer_<v>_linux_<arch>.AppImage` |

Tool installation:

```sh
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest   # deb/rpm
brew install makensis                                      # windows installer (or apt/choco)
# appimagetool: downloaded in CI, see release workflow
```

Design decisions:

- GoReleaser OSS handles CLI archives only — its DMG/NSIS support is Pro-tier,
  so GUI installers are assembled by these scripts instead.
- Icons are generated, never hand-drawn: `go run scripts/package/icons.go`
  renders the dark-surface + green-dot mark from brand palette into
  `packaging/assets/` (PNG set, `.ico`). The macOS `.icns` is derived at
  package time via `iconutil`.
- The macOS bundle is ad-hoc codesigned (`codesign --sign -`) so Gatekeeper
  accepts local runs; real Developer ID signing is a documented add-later seam
  (release workflow env: `MACOS_CERTIFICATE`).
- Windows NSIS installs per-user under `%LOCALAPPDATA%` — no UAC prompt,
  matching the tray-first unprivileged design. WebView2 ships with Win11 and
  auto-installs on Win10 via Evergreen; the app surfaces a clear message if
  missing.
- Linux packages declare webkit2gtk-4.1 deps (deb names differ from rpm) and
  install the binary plus hicolor icons and a desktop entry. AppImage bundles
  everything and is built only on Linux (CI job), since appimagetool is a
  Linux ELF tool.
- CGO is required for Wails on macOS/Linux. Linux desktop builds pass
  `-tags "desktop,gtk3"` so Wails links against GTK3 + webkit2gtk-4.1 (far
  more widely packaged than its GTK4/webkitgtk-6.0 default); runtime deps in
  the deb/rpm match (`libwebkit2gtk-4.1-0` / `webkit2gtk4.1`).
- AppImage assembly needs a native Linux host: the AppImage runtime is a
  static ELF that refuses to exec under qemu-user emulation, so macOS/Windows
  hosts can't produce one locally — CI runs that step natively.

## Release pipeline

Pushing a tag `vX.Y.Z` runs `.github/workflows/release.yml`:

1. `cli-archives` — GoReleaser builds and publishes the GitHub Release with
   CLI archives (`tar.gz`/`zip`, all three OSes) plus `checksums.txt`.
2. In parallel, native runners build GUI installers via the scripts above:
   DMGs on macos-15 (arm64) + macos-15-intel, NSIS setup on windows-latest,
   deb/rpm/AppImage on ubuntu amd64+arm64 runners.
3. `attach-installers` uploads every installer plus
   `DNSer_installers_checksums.txt` onto the same release.

### Channels

Auto-publish jobs run after attach, each gated on repository configuration so
they no-op cleanly when unset:

| Channel | Needs | Script |
|---|---|---|
| Homebrew tap | var `HOMEBREW_TAP_REPO` (`owner/tapname`) + secret `HOMEBREW_TOKEN` | `scripts/channels/homebrew.sh` |
| AUR | secret `AUR_SSH_KEY` (base64); packages `dnser-bin` / `dnser-desktop-bin` must be **created once manually** | `scripts/channels/aur.sh` |
| winget | var `WINGET_PACKAGE_ID` + secret `WINGET_TOKEN`; id must be **registered once** via an initial manual PR | `scripts/channels/winget.sh` |

First-time setup notes:

- Create the tap repo with `Formula/` and `Casks/` dirs (empty is fine); the
  workflow pushes rendered `packaging/homebrew/*.tmpl` into it with real
  SHA256s fetched from the release assets.
- winget: submit the initial manifest PR by hand once (wingetcreate new),
  then set the package id variable — subsequent releases update automatically.
- Signing is intentionally absent (ad-hoc macOS, unsigned Windows). When
  Developer ID + EV certs are procured, add signing steps inside the
  packaging scripts before the artifact upload steps.
