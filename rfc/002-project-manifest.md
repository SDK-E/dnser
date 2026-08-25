# RFC 002 — Project Manifest: `.dnser.yaml` v3 Spec

- Status: **Proposed** (implements RFC 001)
- Companion to: `rfc/001-orchestrator-architecture.md`

## 1. Design rules

1. **Minimal by default, total by choice.** One key must be enough; no
   feature of the orchestrator may be unreachable from the manifest.
2. **Precedence is global and uniform:**
   `CLI flag > manifest > detected convention > built-in default`.
   No key invents its own precedence.
3. **Nothing is invisible.** `dnser explain` prints the fully resolved
   effective configuration annotated with each value's source —
   `(manifest)`, `(detected: package.json)`, `(default)` — so users can
   discover what exists and what to override.
4. **Strict parsing.** Unknown keys are hard errors naming the offending
   line (typo protection); the config file is user-facing, silent typos are
   unacceptable. New keys ship only with a schema release.
5. **Raw passthrough exists at every generated boundary.** Anything dnser
   writes into Caddy or process-compose can be overridden by a typed key or,
   failing that, a raw merge block. Typed wins are documented; raw merges
   apply last.

## 2. The entire spec on one screen

```yaml
# ---- the smallest useful manifest ----
domain: auth.mycompany.internal

# ---- the largest realistic one ----
domains:
  - auth.mycompany.internal            # primary
  - "*.preview.auth.mycompany.internal"
aliases: [auth.local]                  # sugar, appended after `domains`
port: 3000                             # omit -> detect -> allocate+persist
command: npm run dev                   # omit -> framework detection
shell: true                            # run via sh -c (default true)
cwd: packages/auth                     # relative to project root

https: true                            # or per-name map:
# https:
#   "*.preview.auth.mycompany.internal": false
force_https: false

env:
  NODE_ENV: development
env_file: [.env.local]                 # loaded into process env

services:                              # auxiliary supervised processes
  redis:
    image_hint: redis                  # type label only; binary must exist
    command: redis-server --port {port:redis}
    readiness: "tcp://127.0.0.1:{port:redis}"
  smtp:
    type: smtp
    host: 127.0.0.1                    # external: recorded, not managed
    port: 11025
    dns: true                          # publish smtp.<primary domain>

routes:                                # beyond the default whole-site mapping
  - path: /api/*
    port: 4000                         # split traffic to another port
  - host: admin.auth.mycompany.internal
    backend: 127.0.0.1:4001

records:                               # extra DNS under your domains
  - {name: "@",    type: TXT, value: "v=verification abc"}
  - {name: "*.",   type: A,   value: 127.0.0.1}     # wildcard, single level

forward:                               # expose non-HTTP protocols on real ports
  - {proto: tcp, listen: 11025, to: 11025}          # smtp.mailbox style

process:                               # RAW: deep-merged into the project's
  restart: always                      # process-compose entry (last writer)
  shutdown: {signal: SIGTERM, timeout_seconds: 10}
  availability: {restart: "exit_on_failure"}

caddy:                                 # RAW: deep-merged into the generated
  encode: gzip                         # site adapter (directives last-writer)
  header:
    +X-Powered-By: dnser
  log: {output: file, file: "{logs_dir}/access.log"}
```

## 3. Key reference

| Key | Type | When omitted |
|---|---|---|
| `domain` | FQDN string | falls back to `domains[0]`, else `<dirname>.<default_tld>` |
| `domains` | list of FQDN/wildcards | `[domain]`; wildcards legal at any level |
| `aliases` | list | none; appended to effective names |
| `port` | int 1–65535 | `PORT` env honored → framework detection → allocate free & persist to state |
| `command` | string | framework recipe (e.g. `npm run dev`) → none (proxy-only) |
| `shell` | bool \| string | `true` (string names an alternate shell) |
| `cwd` | path | project root |
| `https` | bool \| map[name]bool | `true` for every declared name |
| `force_https` | bool | `false` |
| `env` | map[str,str] | empty; always includes `PORT`, `DNSER_DOMAIN`, `DNSER_SERVICES_<NAME>` |
| `env_file` | path \| list | none |
| `services` | map or list | none; see RFC 001 §4 for dns publishing |
| `routes` | list | one implicit route: everything → `port` |
| `records` | list | none |
| `forward` | list | none; `{proto, listen, to}` for tcp/udp frontends |
| `process` | raw map | none; see §5.1 |
| `caddy` | raw map | none; see §5.2 |
| `availability` | `always` \| `on_request` \| `manual` | template-provided (`always` for services with `dns: true`) |
| `idle_stop` | duration (`30m` default) | never; only meaningful with `availability: on_request` |
| `min_uptime` | duration (`2m` default) | anti-thrash floor before idle stop may fire after a wake |
| `type` | registry name | set by `dnser init --type=…`; drives detection hints + runtime tuning (§ Templates) |

Lifecycle semantics (`availability: on_request`) — designed against
thrash, not per-request spawn. One correctness rule first: **on_request is
valid only for pull protocols** (HTTP). Services that receive inbound push
over non-HTTP transports — SMTP delivery, TCP listeners, mDNS — must
declare `availability: always`; a sleeping SMTP endpoint silently drops
mail during its wake window:

- First request wakes the project asynchronously; the edge holds that
  request until the readiness port accepts (~1–2 s, once).
- **Every subsequent request resets the idle clock** — under active use the
  project stays up indefinitely; stopping happens only after sustained
  silence.
- `idle_stop` (default `30m`) fires after that quiet period; the next
  request wakes it again.
- Anti-thrash guards: `min_uptime` (default `2m`) — idle stop may never
  fire sooner than this after a wake; repeated crashes at wake trigger
  exponential wake-backoff and surface in `doctor` instead of flapping.

Traffic-aware lifecycle is a solved problem elsewhere — Phase 3 must spike
against prior art instead of hand-rolling first:

- **tenement** (github.com/russellromney/tenement): process hypervisor with
  subdomain routing, wake-on-request (<1 s), `idle_timeout`, health checks
  with exponential backoff, process-group cleanup; ships Caddyfile +
  service-unit generation. Closest full match to these semantics.
- **DynaPM**: request-intercepting gateway, ~25 ms wake overhead, idle stop
  with SSE/WebSocket connection tracking. Rejected as default: requires a
  Node.js runtime alongside dnser.
- **Sablier / traefik-lazyload**: scale-to-zero middleware, but
  container-centric.
- Lineage worth knowing: systemd socket activation (Linux-only, no idle
  stop), launchd sockets (wake without stop), inetd (per-request spawn —
  wrong granularity).

If the tenement spike passes platform checks (macOS console-user execution,
manifest-driven domain routing, Caddy coexistence per LocalRun's import-
fragment pattern), it replaces both hand-rolled lifecycle glue *and* parts
of the supervisor story; otherwise fall back to Caddy request metrics →
process-compose REST glue (~200 LOC) with the same semantics.

### Templates and runtime tuning

Runtimes beyond Node are first-class. `dnser init
--type=laravel|symfony|nodejs|rails|go|python|bash|sh|zsh|static` writes a
starter `.dnser.yaml` tuned for that stack. A type is an entry in a
user-extensible registry (repo defaults shipped, overrides in
`~/.dnser/templates/`) carrying three things:

1. **Detection hints** — how to find the framework and its port.
2. **Command conventions** — what `command:` to suggest (`php artisan serve
   --host {domain} --port {port}`, `npm run dev -- --port {port}`, plain
   `./serve.sh` with signal-forwarding wrapper for bash/sh/zsh…).
3. **Runtime environment tuning** — stack-appropriate bounds applied via
   the normal precedence chain (flag > manifest > template > default):
   nodejs sets `--max-old-space-size` unless `NODE_OPTIONS` is user-set;
   laravel/symfony set `PHP_CLI_SERVER_WORKERS`, `memory_limit`, opcache
   flags for built-server/FPM; go/python/static need nothing.

Tuning is per-template data, not orchestrator hardcoding, and every applied
value is visible via `dnser explain` (rule 3, §1).

Placeholders usable in `command`, `backends`, `readiness`,
`forward.to`, `caddy` strings: `{port}` (primary),
`{port:<service>}`, `{domain}`, `{logs_dir}`.

## 4. Inference rules (what omission means)

| Omitted | Resolution order |
|---|---|
| `port` | explicit here → `PORT` observed at spawn → detector (`next dev` default, Vite 5173, …) → allocate free port, persisted across reloads |
| `command` | this file's `command` → detector (`package.json` scripts, Procfile, Makefile targets) → proxy-only mode with warning |
| `https` cert | Caddy internal issuer; leaf issued on-demand per declared name (RFC 001 §5) |
| `service.port` | `{port:name}` substitution target allocated once, stable across restarts |

Detection results are never silently trusted twice: what detection chose is
pinned into daemon state on first link, then re-evaluated only when the
manifest changes (prevents surprise re-allocations).

## 5. Override surface — "basically anything"

### 5.1 Process layer (`process:`)
Deep-merged into the project's process-compose process definition after all
typed fields are applied. Covers restart policy, shutdown semantics,
liveness/exec probes, replicas, resource niceness, working directory —
everything process-compose supports. Merge rules: recursive for maps;
**arrays replace**; scalar replaces. Conflicts between typed keys and
`process:` are validation errors except where explicitly delegated
(e.g. `process.availability.restart` supersedes nothing typed).

### 5.2 Proxy layer (`caddy:`)
Deep-merged into the generated Caddy site adapter for this project's
names: handlers, headers, encoding, logging, timeouts, matchers, extra
sub-routes. String values receive placeholder substitution. This is the
documented last-writer; dnser guarantees its own required directives
(reverse_proxy upstream, tls internal policy) cannot be removed by a merge,
only extended — attempts to remove them fail validation with a pointer to
the reserved-key list.

### 5.3 DNS layer
Extra `records` merge into the auto-generated apex/A records. Wildcards are
single-level (`*.x` matches `y.x`, not `a.b.x`) matching DNS norms; declare
deeper names explicitly when needed. Records live inside suffixes the
resolver registered (RFC 001 §4); attempting records outside declared
domains is a validation error.

### 5.4 What stays out of reach (deliberately)
Global system mutations — resolver registration, CA trust, service install,
privileged ports — belong to `dnser elevate` / settings, not to a project
manifest. A project can request them (`requires_elevation: true` surfaces a
doctor hint) but never perform them from `.dnser.yaml`.

## 6. Worked progression

```yaml
# 1) zero thought
domain: app.acme.io

# 2) next.js needs a pin because it ignores PORT
domain: app.acme.io
command: npm run dev -- --port {port}

# 3) monorepo + sidecar + api split
domain: app.acme.io
cwd: apps/web
command: pnpm dev
services:
  db: {type: postgres, host: 127.0.0.1, port: 55432}
routes:
  - path: /api/*, port: 4000
env_file: .env.development

# 4) full control
process: {restart: always, shutdown: {timeout_seconds: 15}}
caddy:
  header: {+X-Frame-Options: DENY}
  @denied: {respond: ["nope", 403]}
  handle: [{path: [/assets/*], root: "apps/web/public"}]
```

## 7. Validation and errors

- Strict decode: unknown/misspelled keys error with line numbers.
- Cross-field checks: `routes[].port` targets must exist or be services;
  `forward.listen` must be free or owned by this project; wildcard records
  must sit under a declared domain.
- `dnser doctor` re-validates live state against the manifest and reports
  drift with one-click fixes where elevation permits.

## 8. Migration from v2 manifests

- `settings.tld`-relative labels: rewritten at load time to FQNs against the
  project's effective primary domain; a deprecation notice lists every
  rewrite performed.
- Old top-level keys (`command`, `services`, `routes`, `records`) keep their
  shapes; only `host:` label resolution changes (RFC 001 §9).
- `dnser migrate` performs the rewrite in place, shows a unified diff, and
  writes the mutation journal entry.
