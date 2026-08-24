# CLI Reference

All commands accept `--home <dir>` (default `~/.dnser`) and `--config <path>`.
`-V/--verbose` enables verbose output.

## Lifecycle

| Command | Description |
|---|---|
| `dnser` | Interactive first-run wizard (setup → link → open). |
| `dnser setup` | One-time OS integration: resolver routing + CA trust. Revert exactly with `dnser unsetup`. |
| `dnser start` | Start the daemon. `--foreground` runs in the current terminal (used by launchd/desktop). |
| `dnser stop` / `restart` | Stop or restart the daemon/service. |
| `dnser status [domain]` | Daemon health, listener ports, project overview. |
| `dnser open` | Open the dashboard (`https://dnser.test`). |
| `dnser update` | Check GitHub releases for a newer version. |
| `dnser version` | Print version. |

## Projects

| Command | Description |
|---|---|
| `dnser link <path>` | Link a directory: detect stack, allocate a persistent port, manage the dev server.<br>`--domain` · `--command '<cmd>'` · `--port N` · `--wildcard` · `--alias a --alias b`<br>`--tld` · `--no-https` · `--force-https` · `--no-run` (proxy only, no supervision) |
| `dnser unlink <path\|domain>` | Stop management; `--keep-dns` retains routes/records, drops the run config. |
| `dnser ps` | Managed apps table: state, port, PID, restarts. Crash reasons include an actionable hint when the command binary is missing from the daemon PATH. |

## Services (any protocol)

| Command | Description |
|---|---|
| `dnser service add <domain> <name>` | Declare a service.<br>Managed: `--command 'redis-server --port {port}' [--type redis] [--transport tcp\|udp] [--port N] [--no-dns]`<br>External: `--host db.internal [--type postgres] --port 5432` (well-known types default their port)<br>`--dns` publishes `<name>.<domain>` (A for managed/IP, CNAME for hostnames). |
| `dnser service list <domain>` | Table of declared services: type, mode, endpoint, DNS name. |
| `dnser service remove <domain> <name>` | Remove the declaration (aliases: `rm`). |

## Routes

| Command | Description |
|---|---|
| `dnser route add <domain>` | `--host @\|*\|label` (default `@`) · `--backend host:port …` (repeatable, `{port}`/`{port:<svc>}` allowed) · `--path /api …` · `--https` · `--force-https` · `--tcp` / `--udp` with `--listen N`. Same host+paths replaces an existing route. |
| `dnser route list <domain>` | Hostname, frontend kind/port, paths, backends, TLS, redirect state (aliases: `ls`). |
| `dnser route remove <domain> --host <label>` | Remove whole routes; `--path /v2` strips individual prefixes instead. |

## DNS records

| Command | Description |
|---|---|
| `dnser add-record --domain=D --type=A --name=x --value=1.2.3.4 [--ttl N]` | Add a record (types: A, AAAA, CNAME, TXT, MX, SRV). |
| `dnser list-records [domain]` | List records for all/one zone. |
| `dnser remove-record --domain=D --name=x [--type T] [--value V]` | Remove matching record(s). |

## Settings & schemas

| Command | Description |
|---|---|
| `dnser settings` | Show effective settings. |
| `dnser settings set <key> <value>` | Change one key — `tld`, `bind`, `upstreams`, `autostart`, `force_https`, `path_refresh_minutes`, `ports.dns/http/https/ui`. Validated then hot-reloaded. |
| `dnser schema` / `schema --project` | JSON Schema (Draft 2020-12) for `dnser.json` / `.dnser.yaml`. |

## Diagnostics

| Command | Description |
|---|---|
| `dnser doctor` | Checks: dns-port, http-port, https-port, upstreams, resolver, projects (dependencies), commands (every managed command resolves in the daemon's effective PATH). Exit hints point to [Troubleshooting](troubleshooting.md). |
| `dnser logs [-f]` | Recent or streamed query + app events. |

## Import / export

```sh
dnser export --format bind --out zone.db myproject.test
dnser import --file backup.json
```
