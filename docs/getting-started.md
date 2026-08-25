# Getting started

## Install

```bash
# macOS (Homebrew cask) — pulls in Caddy, mkcert and process-compose
brew install --cask sdk-e/tap/dnser

# Linux — deb/rpm from the releases page
# https://github.com/SDK-E/dnser/releases/latest

# From source (you must provide caddy, process-compose, mkcert yourself)
go install github.com/SDK-E/dnser/cmd/dnser@latest
```

Verify what is on your PATH at any time:

```bash
dnser doctor
```

## Zero-config first run

Bare `dnser` guides you to a working setup. Nothing privileged ever happens
without your explicit opt-in: resolver files under `/etc/resolver`, CA trust
and service installation are applied only by

```bash
dnser elevate
```

which shows exactly what will change before requesting administrator
privileges once. Undo it entirely with `dnser unelevate`.

### Fallback mode (no elevation)

If you decline elevation, everything still works with one difference: names
resolve through dnser's listener only while it is running, on unprivileged
fallback ports instead of 53/80/443. Your normal browsing is untouched —
unhandled queries always forward upstream.

## The 60-second tour

```bash
mkdir demo && cd demo && echo '<h1>hi</h1>' > index.html

dnser init static     # scaffold .dnser.yaml for the current project type
dnser link .          # register: DNS + TLS + supervisor config, journalled
dnser up              # start infrastructure and linked projects
dnser start demo      # start the app; readiness-gated, "ready" means ready
open https://demo.test
```

Without a declared `domain:` you get `<dirname>.test`. Declare any
user-owned FQDN instead — `auth.mycompany.internal`, `app.acme.io` — and
every name under it answers locally.

## Everyday commands

```bash
dnser status                # daemon, DNS, Caddy, per-project state
dnser logs <project> -f     # follow app logs via the supervisor
dnser explain <project>     # every effective config value and its source
dnser doctor                # health checks; prints exact fixes
dnser journal               # every change dnser ever made
dnser dashboard             # web UI, loopback-only and token-gated
```

All commands are safe to re-run (idempotent) and speak JSON when piped or
asked: `dnser status -o json`.
