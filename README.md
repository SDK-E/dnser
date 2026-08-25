<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/brand/dnser-logo-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="docs/brand/dnser-logo-light.png">
  <img src="docs/brand/dnser-logo-dark.png" alt="DNS.er" width="420">
</picture>

**Local domains, local HTTPS, zero config — for real projects.**

`dnser` gives every project on your machine a real domain (`mailbox.test`),
trusted HTTPS, and a supervisor that starts, stops, and revives your apps.
One Go binary orchestrating Caddy, a local DNS server, and process-compose.

[![CI](https://github.com/SDK-E/dnser/actions/workflows/ci.yml/badge.svg)](https://github.com/SDK-E/dnser/actions/workflows/ci.yml)
[![Release](https://github.com/SDK-E/dnser/actions/workflows/release.yml/badge.svg)](https://github.com/SDK-E/dnser/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/SDK-E/dnser?include_prereleases&sort=semver)](https://github.com/SDK-E/dnser/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/SDK-E/dnser)](go.mod)
[![License](https://img.shields.io/badge/license-BUSL--1.1-blue)](LICENSE)

</div>

---

## Install

```bash
# macOS (Homebrew) — includes Caddy, mkcert and process-compose
brew install --cask sdk-e/tap/dnser

# Linux (deb/rpm from the releases page)
# https://github.com/SDK-E/dnser/releases/latest

# From source
go install github.com/SDK-E/dnser/cmd/dnser@latest
```

Upgrading from v1? Run `dnser migrate` in each project directory.

## 60-second tour

```bash
mkdir demo && cd demo && echo '<h1>hi</h1>' > index.html

dnser init static     # scaffold .dnser.yaml for the current project type
dnser link .          # register: DNS + TLS + supervisor config, journalled & reversible
dnser up              # start dnser itself (DNS on :3533 fallback chain, Caddy, supervisor)
dnser start demo      # start the app; readiness-gated, so "ready" means ready
open https://demo.test
```

Every command speaks JSON when you do: `dnser status -o json`, `dnser doctor -o json`.
Exit codes are stable (`0` ok, `2` usage, `4` needs setup with remediation printed, `10` doctor found issues).

## Everyday commands

| Command | What it does |
|---|---|
| `dnser link .` | Wire the current project into DNS/TLS/supervision (safe to re-run) |
| `dnser start / stop / restart <project>` | Lifecycle, gated by a TCP readiness probe |
| `dnser status [-o json]` | Projects, phases, ports, DNS state |
| `dnser logs <project> [-f]` | App logs via the supervisor |
| `dnser explain <project>` | Where every config value came from (flag > env > dotenv > manifest > default) |
| `dnser doctor` | Diagnose resolver drift, stale plans, upstream DNS; prints fixes |
| `dnser journal` | Every mutation dnser ever made; `journal revert <id>` undoes one |
| `dnser dashboard` | Local web UI (loopback + token) |
| `dnser elevate` | Opt-in system integration (`/etc/resolver`); never implicit |
| `dnser uninstall` | Purge everything, verified against the mutation journal |

## Examples

Ready-to-link projects live in [`examples/`](examples/) — clone, `cd`, `link`, `up`:

- [`hello-static`](examples/hello-static) — plain HTML served by Caddy with local HTTPS (no process at all)
- [`node-api`](examples/node-api) — Node API honoring `$PORT`
- [`python-api`](examples/python-api) — Python stdlib server with `availability: on_request`
- [`mailbox-style`](examples/mailbox-style) — realistic v1-style manifest after `dnser migrate`: multi-backend routes, SMTP side-service, custom DNS records

## How it fits together

```
        dnser (one binary, CLI-on-demand — no idle daemon)
   ┌─────────────┬──────────────────┬───────────────────┐
   │ ctrld/DNS   │ Caddy            │ process-compose   │
   │ *.test →    │ TLS (mkcert CA)  │ start/stop/wake,  │
   │ 127.0.0.1   │ reverse proxy    │ readiness probes  │
   └─────────────┴───────────────────┴───────────────────┘
        every change journalled → dnser journal / revert / uninstall
```

- **Zero-config first run**: bare `dnser` guides you; nothing privileged without `elevate`.
- **Fallback mode**: no `/etc/resolver` writes; projects resolve while your normal browsing stays untouched. Unhandled queries always forward upstream.
- **Privileged ports degrade gracefully**: 53→5353→35353, 80→8080, 443→8443.
- **Your processes never run as root.**

## Development

```bash
go build ./... && go test ./... && golangci-lint run
```

Architecture and decisions live in [`docs/rfc/`](docs/rfc). Brand assets drop into [`docs/brand/`](docs/brand).

## License

[BUSL-1.1](LICENSE) © SDK Enterprises · hicham@sdk.enterprises
