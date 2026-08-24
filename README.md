# DNSer

**DNSer** is a DNS management tool for local development by [SDK Enterprises](https://sdk.enterprises).

One binary turns your machine into a fully-featured local nameserver: launch projects on real domains (`myproject.test`), serve HTTPS through an auto-trusted local certificate authority, reverse-proxy traffic to your dev servers, and manage everything from a polished web dashboard — zero configuration to start, maximum control when you want it.

## Why

Editing `/etc/hosts` for every project is fragile, doesn't support wildcards, and can't do HTTPS. DNSer runs a **real DNS server** on loopback:

- `dnser link ~/code/myproject` — done. It detects the stack, picks a free port, starts your dev server, and puts it on `https://myproject.test`.
- Every subdomain resolves instantly: `api.myproject.test`, `pr-42.preview.myproject.test`.
- Multiple backends per hostname get round-robin load balancing with health-checked failover; raw TCP forwarding works too.
- Unknown queries forward upstream with caching, so normal browsing is unaffected.

## Features

| | |
|---|---|
| Real DNS server | A, AAAA, CNAME, TXT, MX, SRV, NS + wildcards, hot-reloaded |
| HTTP/HTTPS proxy | SNI-routed reverse proxy with backend pools, round-robin and health failover |
| Path routing | Longest-prefix `paths` per hostname with fallback (`/api` → one pool, everything else → another) |
| TCP & UDP forwarding | Per-route raw listeners for databases, queues, DNS — any service type |
| Linkable services | Declare any number of services per project (redis, postgres, smtp…) — dnser-managed processes or external endpoints, each with its own port and optional `<name>.<domain>` record |
| Managed dev servers | `dnser link` runs Next.js, Nuxt, Vite, SvelteKit, Astro, Angular, Remix, Laravel, Symfony, Spring Boot, Django, Rails, Go, Rust — restarts crashed apps with backoff; commands resolve through an augmented login-shell PATH (no launchd-PATH breakage) |
| Local CA | Auto-generated root CA, per-domain leaf certs, one-time trust |
| Web dashboard | Dark premium UI at `https://dnser.test` — project cards, route & service editors, live logs, health dots, ⌘K palette, doctor panel |
| Live query log | Stream every resolution and app output line to CLI or dashboard |
| Desktop app | Tray-first GUI: per-project tray menu, one-click setup, launch-at-login, auto-update checks (macOS/Windows/Linux) |
| Import/export | JSON + BIND zone files |
| Cross-platform | macOS (launchd), Linux (systemd), Windows (services) |

## Quickstart

```sh
brew install sdk-e/tap/dnser   # or: curl -fsSL https://raw.githubusercontent.com/SDK-E/dnser/main/scripts/install.sh | sh

dnser                          # guided wizard: setup → link → open dashboard
```

```sh
cd ~/code/my-project
dnser link .
# ✓ linked my-project.test
#   stack: Next.js · runs: pnpm run dev · port: 52123
open https://my-project.test
```

The dev server is now managed: it starts with the daemon, restarts on crash,
and its output lands in the dashboard and `~/.dnser/logs/`. Prefer to run it
yourself? Add `--no-run --port 3000` and DNSer only proxies.

### Routes: load balancing, path routing, TCP/UDP

Routes are the source of truth in `~/.dnser/dnser.json`:

```json
{
  "routes": [
    { "host": "@", "backends": ["localhost:3001", "localhost:3002"], "https": true },
    { "host": "@", "backends": ["localhost:4000"], "paths": ["/api"] },
    { "host": "dbx", "tcp": true, "listen": 55432, "backends": ["localhost:5433"] }
  ]
}
```

Requests round-robin across healthy backends; dead ones are skipped
automatically. Path prefixes route by longest match with fallback to the
route without paths. TCP/UDP routes bind an explicit local listen port and
forward raw traffic to any host. See [docs/routes.md](docs/routes.md).

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
dnser link <path>      detect stack, allocate port, manage the dev server
                       flags: --domain --command --no-run --port --wildcard
                              --alias --tld --no-https --force-https
dnser unlink <path|domain> [--keep-dns]
dnser service add|list|remove <domain> [name]   declare managed or external services
dnser route add|list|remove <domain>            http/https/tcp/udp routes, path prefixes
dnser settings [set <key> <value>]              force_https, path_refresh_minutes, ports, …
dnser schema [--project]                        JSON Schema for dnser.json / .dnser.yaml
dnser ps               managed dev servers and their state
dnser doctor           diagnose ports, upstreams, resolver, dependencies
dnser add-record --domain=myproject.test --type=TXT --name=x --value=y
dnser list-records [domain]
dnser remove-record ...
dnser open             open the dashboard
dnser logs [-f]        live query + app log stream
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

### Project overrides

A `.dnser.yaml` in the project root overrides detection and can declare
additional services, routes and ports:

```yaml
command: pnpm dev --port {port}
services:
  redis:
    type: redis
    command: redis-server --port {port}
routes:
  - host: api
    paths: [/api]
    https: true
    backends: [127.0.0.1:{port}]
  - host: dbx
    tcp: true
    listen: 55432
    backends: [127.0.0.1:{port:redis}]
```

`{port}` is replaced by the persistent port allocated at link time;
`{port:<service>}` by a named service's port.
Precedence: `--command` flag > `.dnser.yaml` > built-in recipe; stored config
wins over the file on conflicts.

Dependencies are never installed automatically — if `node_modules`, `vendor`
or similar is missing, `dnser ps` and the dashboard's doctor panel tell you
the exact command to run.

## Documentation

| Document | Contents |
|---|---|
| [docs/configuration.md](docs/configuration.md) | `dnser.json` settings reference (force_https, PATH TTL, ports) |
| [docs/projects.md](docs/projects.md) | Linking, `.dnser.yaml` manifest, services & multi-port model |
| [docs/routes.md](docs/routes.md) | Routing semantics — hosts, path prefixes, TLS, TCP/UDP |
| [docs/cli.md](docs/cli.md) | Full CLI reference |
| [docs/troubleshooting.md](docs/troubleshooting.md) | PATH resolution & exit-127 loops, ports, resolver, CA |

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
