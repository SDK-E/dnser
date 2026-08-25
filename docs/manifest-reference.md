# Manifest reference (`.dnser.yaml`)

Every project directory that dnser manages has a `.dnser.yaml`. Parsing is
strict: unknown keys are hard errors naming the offending line, so typos
never silently do nothing.

## The smallest useful manifest

```yaml
domain: auth.mycompany.internal

port: 3000      # optional; omit to detect or allocate
https: true     # optional; default true
command: npm run dev   # optional; omit for framework detection or proxy-only
```

Declaring a domain makes **the domain itself and every name ending in it**
answer locally (`api.auth.mycompany.internal` included). Wildcard entries
like `"*.preview.auth.mycompany.internal"` are documentation sugar for the
same suffix semantics.

Without any declared domain you get `<dirname>.<default_tld>` (default
TLD: `test`). Registering a public suffix like `com` is allowed but warned
about loudly at `link` time and in `doctor`, because it shadows real
internet names.

## Key reference

| Key | Type | When omitted |
|---|---|---|
| `domain` | FQDN string | falls back to `domains[0]`, else `<dirname>.<default_tld>` |
| `domains` | list of FQDN/wildcards | `[domain]` |
| `aliases` | list | none; appended after `domains` |
| `port` | int 1–65535 | `PORT` env honored → framework detection → allocate free & persist across restarts |
| `command` | string | framework recipe (e.g. `npm run dev`) → none (proxy-only mode, warning) |
| `shell` | bool \| string | `true` (a string names an alternate shell) |
| `cwd` | path | project root |
| `type` | registry name | set by `dnser init --type=…`; drives detection hints and runtime tuning |
| `https` | bool \| map[name]bool | `true` for every declared name |
| `force_https` | bool | `false` |
| `env` | map[str,str] | empty; always includes `PORT`, `DNSER_DOMAIN`, `DNSER_SERVICES_<NAME>` |
| `env_file` | path \| list | none; loaded into process env |
| `availability` | `always` \| `on_request` \| `manual` | template-provided |
| `idle_stop` | duration | `30m`; only meaningful with `availability: on_request` |
| `min_uptime` | duration | `2m` anti-thrash floor before idle stop may fire after a wake |
| `services` | map or list | none |
| `routes` | list | one implicit route: everything → `port` |
| `records` | list | none |
| `forward` | list | none |
| `process` | raw map | none; deep-merged into the supervisor entry |
| `caddy` | raw map | none; deep-merged into the generated site |

### Placeholders

Usable in `command`, `services.readiness`, `backends`, `forward.to` and
`caddy` strings: `{port}` (primary), `{port:<service>}`, `{domain}`,
`{logs_dir}`.

## Services

Auxiliary supervised processes alongside the main one:

```yaml
services:
  redis:
    image_hint: redis                  # type label only; binary must exist
    command: redis-server --port {port:redis}
    readiness: "tcp://127.0.0.1:{port:redis}"
  smtp:
    type: smtp
    host: 127.0.0.1                    # external: recorded, not managed
    port: 11025
    dns: true                          # publishes smtp.<primary domain>
```

Service ports substituted via `{port:name}` are allocated once and stay
stable across restarts.

## Routes, records, forwards

```yaml
routes:
  - path: /api/*
    port: 4000                         # split traffic to another port
  - host: admin.auth.mycompany.internal
    backend: 127.0.0.1:4001

records:
  - {name: "@",  type: TXT, value: "v=verification abc"}
  - {name: "*.", type: A,   value: 127.0.0.1}   # single-level wildcard

forward:                               # expose non-HTTP protocols on real ports
  - {proto: tcp, listen: 11025, to: 11025}
```

Wildcard records are single-level (`*.x` matches `y.x`, not `a.b.x`);
declare deeper names explicitly. Records must sit under a declared domain.

## Lifecycle: on-demand projects

`availability: on_request` starts the project lazily on first HTTP request
(~1–2 s once); every subsequent request resets the idle clock, so active
use keeps it up indefinitely. After `idle_stop` (default `30m`) of silence
it stops; the next request wakes it again.

Constraints:

- Only valid for pull protocols (HTTP). Push services — SMTP delivery, TCP
  listeners — must be `always`; a sleeping SMTP endpoint silently drops
  mail during its wake window.
- `min_uptime` prevents thrash; repeated crashes at wake back off
  exponentially and surface in `doctor`.

## Templates

`dnser init --type=laravel|symfony|nodejs|rails|go|python|bash|sh|zsh|static`
writes a starter manifest tuned for the stack: detection hints, command
conventions and runtime tuning (e.g. nodejs sets `--max-old-space-size`
unless you already set `NODE_OPTIONS`). Every applied value stays visible
in `dnser explain`. Overrides live in `~/.dnser/templates/`.

## Raw overrides

Typed keys cover the common cases; two raw blocks reach the rest:

- `process:` deep-merged into the project's process-compose entry (restart
  policy, shutdown signals, probes). Maps merge recursively, arrays
  replace.
- `caddy:` deep-merged into the generated Caddy site (headers, logging,
  matchers, extra routes). dnser's required directives (`reverse_proxy`,
  TLS policy) can be extended but never removed.

Worked progression:

```yaml
# 1) zero thought
domain: app.acme.io

# 2) next.js ignores PORT, pin it
domain: app.acme.io
command: npm run dev -- --port {port}

# 3) monorepo + sidecar + api split
domain: app.acme.io
cwd: apps/web
command: pnpm dev
services:
  db: {type: postgres, host: 127.0.0.1, port: 55432}
routes:
  - path: /api/*
    port: 4000
env_file: .env.development

# 4) full control
process: {restart: always, shutdown: {timeout_seconds: 15}}
caddy:
  header: {+X-Frame-Options: DENY}
```

Editor completion: `dnser schema > dnser.manifest.schema.json` emits the
JSON Schema generated from the same structs the binary validates against.

## Precedence

One global rule, no key invents its own:
`CLI flag > env > dotenv > manifest > detected convention > built-in default`.
`dnser explain` prints every effective value with its source.
