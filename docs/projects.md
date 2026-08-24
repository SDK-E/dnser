# Projects & .dnser.yaml

A *project* is a directory linked to a DNS zone. Linking assigns a domain,
allocates a persistent port, and — when a dev command is known — supervises
the process.

```sh
dnser link ~/code/myproject            # myproject.test, stack auto-detected
dnser link . --domain=api --alias=www  # explicit naming + extra hostname
dnser unlink ~/code/myproject          # or: dnser unlink myproject.test
```

Precedence for the managed command:
`--command` flag → `.dnser.yaml` `command:` → built-in stack recipe.

## The project manifest (`.dnser.yaml`)

A `.dnser.yaml` in the project root is read **live** by the daemon on every
reload. It supports three top-level keys:

```yaml
command: pnpm dev --port {port}

services:
  redis:
    type: redis
    command: redis-server --port {port}
  queue:
    type: nats
    command: nats-server -p {port}

routes:
  - host: api
    paths: [/api, /v2]
    https: true
    backends:
      - 127.0.0.1:{port}
  - host: dbx
    tcp: true
    listen: 55432
    backends: [127.0.0.1:{port:redis}]
```

Schema (JSON Schema for editor tooling): `dnser schema --project`.

### command

Primary dev-server command, executed via `/bin/sh -c` (`cmd /C` on Windows)
from the project directory.

| Placeholder | Resolves to |
|---|---|
| `{port}` | the project's persistent primary port |
| `{port:<service>}` | the port of the named service declared below |

The primary port is allocated automatically, persisted to `run.port`, and
re-allocated only if taken at daemon start.

### services

Any number of named services of any type — `redis`, `postgres`, `smtp`,
`memcached`, a second web app; the label is free-form metadata shown in `ps`,
the dashboard and doctor output. Two forms are accepted:

```yaml
services:
  redis:                       # mapping style
    type: redis
    command: redis-server --port {port}
```

```yaml
services:
  - name: redis                # list style (equivalent)
    type: redis
    command: redis-server --port {port}
```

Each service is either **managed** or **external**:

| Field | Managed service | External service |
|---|---|---|
| `command` | required — supervised like the primary app | must be absent |
| `host` | must be absent | required — IP or hostname of the remote endpoint |
| `port` | local bind port; `0`/absent = auto-allocate & persist | remote port; required |
| `type` | free-form label | free-form label |
| `transport` | `tcp` (default) or `udp` | same |
| `dns` | publish `<name>.<domain>` as an A record → `127.0.0.1`; defaults to `true` in `.dnser.yaml`, `false` via CLI/API unless requested | publish record: A for IPs, CNAME for hostnames |

Managed services appear as separate apps in `dnser ps`
(keyed `myproject.test/<service>`) with independent start/stop/restart,
crash-loop backoff and log tee (`~/.dnser/logs/<domain>.log`). Each gets
`PORT=<its port>` plus `DNSER_SERVICE=<name>` in its environment.

Examples:

```yaml
services:
  postgres:                    # external database — no process management
    type: postgres
    host: db.internal
    port: 5432                 # reachable as postgres.myproject.test (CNAME)
```

```yaml
services:
  mailpit:                     # managed sidecar
    type: smtp
    command: mailpit --smtp {port}
    transport: tcp             # expose it later via a tcp route if needed
```

### routes

Full route model — see [Routes](routes.md) for semantics. Any host label,
any backend host (not limited to `127.0.0.1`), any port, HTTP/HTTPS/TCP/UDP,
path prefixes. Backend strings may use `{port}` / `{port:<service>}`.

## Merge semantics

`.dnser.yaml` fills gaps; stored configuration wins:

- routes whose resolved hostname already exists in `dnser.json` are ignored;
  new ones are imported into `dnser.json` (visible/editable everywhere);
- services already declared in `dnser.json` win by name; file-only services are imported;
- service DNS records are added only when missing — never removed;
- edits made via dashboard/API/CLI persist across reloads.

To fully re-import from the file: edit/remove the affected entries in
`~/.dnser/dnser.json` (or relink).

## Multi-port projects

Ports exist wherever they are declared:

| Port kind | Declared via | Persisted in |
|---|---|---|
| Primary app port | `run.port` (auto) | `run.port` |
| Service port | service entry (`{port}` inside its command) | `services[].port` |
| TCP/UDP frontend | route `listen` | route `listen` |
| External endpoint | service `host:port` | service fields |

All of them participate in collision avoidance: allocations exclude every
other declared port, core listeners, and each other.

## Managing from UI / CLI / config

Every mutation surface writes the same document:

```sh
dnser service add myproject.test cache --type=redis --host=db.internal --port=6379
dnser service add myproject.test worker --command='node worker.js'
dnser service list myproject.test
dnser service remove myproject.test worker

dnser route add myproject.test --host=api --backend=127.0.0.1:{port} \
    --path=/api --https --force-https
dnser route add myproject.test --host=dbx --tcp --listen=55432 \
    --backend=127.0.0.1:{port:redis}
dnser route list myproject.test
dnser route remove myproject.test --host=dbx
```

The dashboard exposes the same operations under a project's
**Routes** / **Services** tabs.
