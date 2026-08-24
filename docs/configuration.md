# Configuration Reference

DNSer is configured through `~/.dnser/dnser.json` (override with `--home` or `--config`).
The file is user-facing: field order is stable, writes are atomic, and every change
hot-reloads a running daemon — no restart required.

Machine-readable schema (Draft 2020-12): `dnser schema` or
`GET /api/v1/schema/config`.

## Document structure

```jsonc
{
  "version": 2,
  "settings": { /* daemon-wide settings */ },
  "projects": [ /* linked projects */ ]
}
```

| Field | Type | Description |
|---|---|---|
| `version` | int | Schema version. `2` is current; v1 files migrate automatically on open. |
| `settings` | object | Global settings, see below. |
| `projects` | array | Linked projects, see [Projects](projects.md). |

## settings

```jsonc
{
  "tld": "test",
  "bind": "127.0.0.1",
  "upstreams": ["9.9.9.9", "149.112.112.112", "1.1.1.1"],
  "autostart": true,
  "force_https": false,
  "path_refresh_minutes": 1440,
  "ports": { "dns": 53, "http": 80, "https": 443, "ui": 4500 }
}
```

| Field | Default | Description |
|---|---|---|
| `tld` | `test` | Development TLD served by the resolver. Project domains are `<name>.<tld>`. |
| `bind` | `127.0.0.1` | IP address all listeners bind to. |
| `upstreams` | Quad9 + Cloudflare | Upstream resolvers for forwarded queries (at least one required). |
| `autostart` | `true` | Start the daemon at login (desktop installs). |
| `force_https` | `false` | **Global default redirect**: when `true`, every route that has `https: true` also redirects plain HTTP → HTTPS. A per-route `force_https: true` overrides independently of this flag. |
| `path_refresh_minutes` | `1440` | TTL for the cached login-shell `PATH` used to launch managed commands and services. Recaptured when missing, older than the TTL, or when `$SHELL` changed. `0` = default. |
| `ports.dns` / `ports.http` / `ports.https` / `ports.ui` | `53/80/443/4500` | Listener ports. Privileged ports fall back (`53→5353→35353`, `80→8080`, `443→8443`) with a warning instead of failing. |

Change any of these without editing JSON:

```sh
dnser settings                     # show effective values
dnser settings set force_https true
dnser settings set path_refresh_minutes 360
dnser settings set ports.dns 5353
```

Or via API: `GET/PUT /api/v1/settings`. Or in the dashboard:
**Settings** in the header.

## projects

See [Projects & .dnser.yaml](projects.md) for the full project model
(`path`, `run`, `services`, `routes`, `records`) including the per-project
manifest format.

## Validation

Every write passes through validation; invalid documents are rejected loudly
with the offending path, e.g.
`projects[0] (app.test).routes[1]: tcp listen 40000 already used by …`.
Rules include:

- unique project zones; unique resolved route hostnames; unique listen ports across all TCP/UDP routes and core listeners;
- backends must be `host:port`; the port may be a `{port}` / `{port:<service>}` placeholder resolved at reconcile time;
- `tcp`/`udp` routes require `listen`; `https` is invalid on them;
- `force_https` requires `https`;
- services are either managed (`command`) or external (`host`), never both.

## Related

- [Routes](routes.md) — HTTP/HTTPS/TCP/UDP routing semantics, path prefixes, redirects.
- [Troubleshooting](troubleshooting.md) — PATH resolution, port fallbacks, doctor checks.
