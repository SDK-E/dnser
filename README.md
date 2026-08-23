# DNSer

**DNSer** is a DNS management tool for local development by [SDK Enterprises](https://sdk.enterprises).

One binary turns your machine into a fully-featured local nameserver: launch projects on real domains (`myproject.test`), serve HTTPS through an auto-trusted local certificate authority, reverse-proxy traffic to your dev servers, and manage everything from a polished web dashboard — zero configuration to start, maximum control when you want it.

## Why

Editing `/etc/hosts` for every project is fragile, doesn't support wildcards, and can't do HTTPS. DNSer runs a **real DNS server** on loopback:

- `dnser link --domain=myproject.test --port=3000` — done. Every subdomain resolves instantly.
- `https://myproject.test` proxies to `localhost:3000` with a valid local TLS cert.
- Wildcards work out of the box: `api.myproject.test`, `*.staging.myproject.test`.
- Unknown queries forward upstream with caching, so normal browsing is unaffected.

## Features

| | |
|---|---|
| Real DNS server | A, AAAA, CNAME, TXT, MX, SRV, NS + wildcards, hot-reloaded |
| HTTP/HTTPS proxy | SNI-routed reverse proxy to dev server ports |
| Local CA | Auto-generated root CA, per-domain leaf certs, one-time trust |
| Web dashboard | Dark premium UI at `https://dnser.test` — records, logs, health |
| Live query log | Stream every resolution to CLI or dashboard |
| Desktop app | Tray-first GUI with one-click setup, launch-at-login, auto-update checks (macOS/Windows/Linux) |
| Port auto-detect | `package.json` / Vite / Next.js heuristics pick the right port |
| Import/export | JSON + BIND zone files |
| Cross-platform | macOS (launchd), Linux (systemd), Windows (services) |

## Quickstart

```sh
brew install sdk-e/tap/dnser   # or: curl -fsSL https://raw.githubusercontent.com/SDK-E/dnser/main/scripts/install.sh | sh

dnser                          # guided wizard: setup → link → open dashboard
```

```sh
dnser link --domain=myproject.test --port=3000
# ✓ myproject.test → localhost:3000 (wildcard *.myproject.test enabled)
open https://myproject.test
```

## Desktop app

Prefer a native window over the CLI? Grab an installer from
[Releases](https://github.com/SDK-E/dnser/releases/latest):

| Platform | Asset |
|---|---|
| macOS (Apple Silicon / Intel) | `DNSer_<version>_macOS_arm64.dmg` / `..._amd64.dmg` |
| Windows | `DNSer_<version>_windows_amd64_setup.exe` |
| Debian / Ubuntu | `dnser-desktop_<version>_linux_amd64.deb` |
| Fedora / RHEL | `dnser-desktop_<version>_linux_*.rpm` |
| Any Linux | `DNSer_<version>_linux_amd64.AppImage` |

The desktop app embeds the same daemon and dashboard — no separate CLI needed.
Closing the window keeps DNSer running in your tray; **Setup System
Integration** performs the same resolver routing + CA trust as
`dnser setup`, asking for admin permission once. The app checks GitHub for new
releases every few hours and surfaces a download link in the tray and
dashboard (`dnser update` does the same from the CLI).

## CLI

```
dnser setup            one-time OS configuration (resolver + CA trust)
dnser start|stop|restart|status
dnser link --domain=myproject.test [--port] [--tld] [--alias ...]
dnser unlink --domain=myproject.test
dnser add-record --domain=myproject.test --type=TXT --name=x --value=y
dnser list-records [domain]
dnser remove-record ...
dnser open             open the dashboard
dnser logs [-f]        live query log stream
dnser import|export    JSON or BIND zone files
dnser update           check for a newer release
dnser version
```

## Development

Requires Go 1.27+ and pnpm (for the embedded UI).

```sh
go build ./... && go test ./...
golangci-lint run
pnpm --dir web install && pnpm --dir web build
go build -o dnser ./cmd/dnser
```

Desktop app development lives behind a build tag (see `packaging/README.md`
for installer recipes and `.github/workflows/release.yml` for the release
pipeline):

```sh
pnpm --dir web build
go build -tags "desktop,gtk3" -o dnser-desktop ./cmd/dnser-desktop   # linux needs GTK3/webkit2gtk-4.1
```

See `AGENTS.md` for repository conventions.

## License

[Business Source License 1.1](LICENSE). Free to use; converts to Apache-2.0 on the Change Date.
