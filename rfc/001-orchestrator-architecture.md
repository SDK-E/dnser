# RFC 001 — dnser v2: Orchestrator Architecture

- Status: **Proposed**
- Author: engineering, with owner sign-off pending
- Supersedes: current single-binary internal implementation (see Migration)
- Companion: `002-project-manifest.md` — full `.dnser.yaml` v3 spec

## 1. Context

dnser today is a single Go binary that hand-implements nearly every layer it
needs: a YAML-subset parser (~620 LOC), a DNS forwarding/cache layer, SNI TLS
routing, an internal CA, a process supervisor, log tee/rotation, launchd/
systemd/schtasks management, and a bespoke React UI kit. A 2026 audit found
maintained open-source replacements for every one of these layers, and found
that several of our hand-rolled choices depend on abandoned upstreams.

Operational failures traced to this architecture during August 2026:

- Stale `/etc/resolver/test` pointing at a dead fallback port: setup trusted a
  persisted flag instead of verifying disk state.
- Project dev servers spawned by the privileged daemon ran as **root**, so
  Next.js build artifacts (`​.next/`) became root-owned and broke the user's
  toolchain.
- Idle-daemon resource usage high enough to degrade the host.
- Configuration surface (TLD + relative route labels + services + routes)
  required multiple relink cycles to express "run this folder on this domain".

## 2. Goals

- G1 — dnser becomes a **thin orchestrator** of proven tools; we write glue,
  not infrastructure.
- G2 — One-command install per OS; dependencies pulled by the OS package
  manager. Single-binary is explicitly **not** a goal anymore.
- G3 — Uninstall purges **everything**: services, system DNS changes, CA
  trust, state, caches, temp data. Verified against a recorded manifest of
  every mutation.
- G4 — Domains are **arbitrary user-owned strings** with wildcard support.
  The fixed global TLD becomes a zero-config default only.
- G5 — Day-to-day operation never runs as root (one-time elevation only).
- G6 — Every phase of migration leaves `go build ./... && go test ./... &&
  golangci-lint run` green.

## 3. Non-goals

- Competing with production ingress proxies (no ACME/public certs in v2 scope).
- Multi-host clustering.
- Replacing Docker-based workflows (Lerd/DDEV territory).

## 4. Domain model (G4)

Domains stop being "label + global TLD" and become fully qualified,
user-chosen strings. The user decides where the "domain" ends and subdomains
begin by simply writing the full name.

```yaml
# .dnser.yaml — the common case is now the whole file
domain: auth.mycompany.internal
port: 3000          # optional; omitted = detect or allocate
https: true         # optional; default true
```

Advanced form (unchanged keys, new semantics):

```yaml
domains:
  - auth.mycompany.internal
  - "*.preview.auth.mycompany.internal"
  - "*.a.b.c.d.e.f"          # c.d.e.f is "the domain", a/b are subdomains —
                             # the split point is wherever the user says
routes: []                  # opt-in escape hatch, unchanged shape
services: {}                # opt-in, unchanged shape
```

### Resolution semantics

- Declaring a domain D makes **D itself and every name ending in `.D`**
  answer locally (dnsmasq `address=/D/` suffix semantics). `*.x` in config is
  documentation sugar; `x` alone already implies the subtree.
- Unknown names under a registered suffix: the local DNS listener **forwards
  them upstream** rather than NXDOMAIN-ing them (preserves the release-blocker
  invariant "unhandled queries MUST forward upstream").
- Names outside every registered suffix are forwarded untouched.
- Registering a public suffix (`com`, `io`, …) is allowed but emits a loud
  warning at `link` time and in `doctor`: it shadows real internet names.

### Zero-config default

`settings.default_tld` (default `test`) is used **only** when a project
declares no domain: bare `dnser link` then yields `<dirname>.test`. It no
longer restricts anything.

### System integration per suffix

- macOS: one `/etc/resolver/<suffix>` file per registered root suffix
  (e.g. `/etc/resolver/internal`), pointing at the local listener with its
  actual port. Files are content-verified on every daemon start (fixes the
  stale-port bug class; see `ResolverDomainStale` precedent in v1 code).
- Linux/Windows: the single local DNS listener owns split routing internally;
  platform resolv/NIC changes are recorded and reversible.

## 5. Component decisions

| Concern | Choice | Role | Evidence |
|---|---|---|---|
| TLS proxy, SNI, HTTP routing | **Caddy** (orchestrated binary) | Binds 80/443, internal PKI, on-demand leaf certs restricted by permission module to registered suffixes, reverse proxy to project ports | Powers LocalRun & production globally; local CA via Smallstep libs; `caddy trust`/`untrust`; Apache-2 |
| DNS listener | **ctrld** or **dnsproxy** (decide in Phase 3 spike) | Split DNS: registered suffixes → local answer set; everything else → upstream (plain/DoH/DoT) | ctrld: policy wildcards `--domains=*.internal`, service mode on all OSes; dnsproxy: AdGuard-maintained lib+binary, cache/fallbacks built in |
| Dev-server supervision | **process-compose** (orchestrated binary) — pending Phase 3 spike vs **tenement** | Runs project processes as **the console user**; restart policies; health checks; dependency ordering; REST API consumed by both CLI and dashboard. Tenement (github.com/russellromney/tenement) additionally provides wake-on-request, idle scale-to-zero, and backoff natively — if the spike validates it on macOS with manifest-driven domains, it supersedes process-compose for projects and deletes the lifecycle glue layer | Single Go binaries; actively developed; eliminates ~800 LOC of supervisor and the root-spawn bug class |
| CLI layer | Keep **cobra**, add **charmbracelet/fang** + **huh**/**lipgloss** | Researched 2026-07 (danilchenko.dev Go CLI comparison; adaptive-enforcement-lab.com framework selection): multi-command trees with shared config = Cobra's exact case; fang layers styled help/version/manpages/completions onto Cobra with one call; huh v2 (Charm, active 2026, accessible-mode forms) powers `dnser init` wizards and elevation prompts; lipgloss for themed status output matching brand colors | All actively maintained (bubbles v2.1.1 Jul 2026); replaces hand-rolled help/prompt/print code |
| Dashboard UI | **Mantine v9** (+ `mantine-datatable`); supersedes earlier un-researched shadcn pick. Design language: Apple-grade professional — system font stack, generous whitespace, restrained low-alpha depth, quiet motion; logo and brand colors untouched, everything else reinventable | Problem-first comparison for THIS use case (solo maintainer, localhost-served dashboard where bundle size is irrelevant, tables/logs/forms/toasts/dark-mode required, minimize maintained code): 2026 sources converge that batteries-included libraries beat copy-and-own kits for admin/dashboard work — mantine-datatable gives sorting/pagination/selection that costs ~150–200 LOC of TanStack wiring under shadcn; notifications/date-pickers/forms built in (5–8 fewer deps). Brand maps via MantineProvider tokens (`#082003`/`#2cdb16`). Tradeoffs accepted: ~40–50 KB gzipped (void on localhost), breaking majors between releases (small dashboard surface mitigates), source not owned (aligned with maintain-less goal). Revisit only if pixel-level brand design becomes a product requirement | github.com/mantinedev/mantine (v9, active); mantine-datatable; 2026 comparisons: shadcndeck.com/blog/mantine-vs-shadcn, devforgedev.hashnode.dev, woodcp.com 8-library benchmark, 21st.dev dashboard-libraries |
| Dashboard data layer | **TanStack Query** retained (kit-independent) | Fetch/cache/retry against process-compose REST + dnser API regardless of component library | Ecosystem default |
| CA trust install/remove | **mkcert** (`-install`/`-uninstall`) or `caddy trust`/`untrust` | System keychain/NSS/Java stores, reversible | FiloSottile confirmed maintained Feb 2026; covers stores our hand-rolled darwin.go partially missed (NSS/Firefox) |
| Manifest parsing | `go.yaml.in/yaml/v4` | Real YAML for `.dnser.yaml` + generated configs | yaml.v3 archived Apr 2025; official YAML-org continuation |
| Log rotation | `timberjack` | Project log files | Maintained lumberjack lineage (lumberjack itself stale since 2023) |
| Retry/backoff (orchestrator-internal) | `cenkalti/backoff/v5` | Tool supervision retries | Actively maintained |

Deleted as a result: `internal/proxyd`, `internal/certs`, `internal/runner`
(supervisor core), `internal/logstream` (process-compose logs + SSE bridge),
`internal/service`, the custom resolver-file writers, `dotdnser.go`,
`ui.tsx` primitives. Survives: CLI orchestration, domain registry/config,
elevation helper contract, doctor, installer/uninstaller logic, embedded UI.

## 6. Privilege model (G5)

A privileged helper performs exactly three mutation classes, once:

1. write/remove `/etc/resolver/*` (macOS),
2. install/untrust the CA,
3. install/remove the background service definition.

Everything else — Caddy, DNS listener, process-compose, project processes —
runs as the console user. Elevation is requested through the OS mechanism
(osascript dialog / pkexec / UAC), applied idempotently by `dnser elevate`,
reversible via `dnser unelevate`. `dnser doctor` verifies disk state against
expectations and offers one-click repair (pattern proven by Yerd/KTStack).

Fallback mode: if elevation is refused, DNS runs on an unprivileged high port
and `/etc/resolver` cannot be written — projects stay reachable at
`http://localhost:<port>` with an explicit notice (Yerd pattern), never hard
failure.

## 7. Install (G2)

| OS | Command | Dependency handling |
|---|---|---|
| macOS | `brew install sdk-e/tap/dnser` | Formula `depends_on ["caddy", "process-compose", "mkcert"]`; brew installs them automatically |
| Debian/Ubuntu, Fedora | `curl -fsSL https://get.dnser.dev | sh` → apt/dnf repo | deb/rpm `Depends:`/`Requires:` pull caddy/process-compose |
| Arch | AUR package | `depends=(caddy process-compose)` |
| Windows | `scoop install dnser` | Scoop manifest `depends:` installs runtime deps automatically |

Note: **winget dependency support is documented as experimental/not
implemented** (winget-pkgs schema notes; #163 spec) — Windows ships via scoop
and/or a bundled installer, not winget promises.

## 8. Uninstall / purge (G3)

`dnser uninstall --purge` executes against the recorded mutation journal
(`~/.dnser/mutations.json`, append-only, written by every mutating step):

1. Stop all supervised projects (process-compose shutdown), remove its
   project config.
2. Stop/disable/remove the background service (platform-native: brew
   services, systemd unit, Task Scheduler entry).
3. `caddy untrust` / `mkcert -uninstall`: remove root CA from system store,
   NSS profiles, Java store.
4. Remove `/etc/resolver/*` entries we wrote; restore NIC/system DNS settings
   captured at elevate time.
5. Delete `~/.dnser/` entirely (config, certs, logs, caches, temp,
   mutations journal last) plus XDG cache/data dirs on Linux.
6. Print package-manager cleanup: `brew uninstall dnser && brew autoremove`
   (autoremove is required — brew does not auto-remove deps on uninstall),
   `sudo apt remove dnser && sudo apt autoremove`, `scoop uninstall dnser`.

Purge verification: after step 5 the command re-checks each journal entry and
reports any residue instead of exiting silently.

## 9. Migration plan

| Phase | Scope | Risk |
|---|---|---|
| 0 | This RFC merged; bug-fix-only freeze on affected layers | none |
| 1 | Library swaps inside current binary: yaml/v4 manifests, timberjack logs, backoff lib, credential drop before spawning projects (kills root-owned artifacts early) | low |
| 2 | Privilege boundary: extract helper contract, move Caddy/process-compose/DNS behind orchestrator flags behind an experimental env var | medium |
| 3 | Default-on tool orchestration; delete `proxyd`, `certs`, runner internals; config schema v3 (`domains[]`, deprecated `settings.tld`) with v2 auto-migration | medium |
| 4 | UI rebuild (Mantine v9 + mantine-datatable + TanStack Query), dashboard reads process-compose REST + dnser API | low |
| 5 | Packaging: tap/repo/scoop manifests, `uninstall --purge`, mutation journal backfill | low |

Schema v2 → v3 migration: `projects[].routes[].backends` ports become
`site.port`; route `host` labels resolve against the project's declared
`domains[0]`; `settings.tld` → `settings.default_tld` advisory only.

## 10. Open questions

- ctrld vs dnsproxy final pick — Phase 3 spike benchmarks idle CPU (the v1
  complaint) and wildcard-answer latency.
- tenement vs process-compose for project supervision/lifecycle — Phase 3
  spike: macOS console-user execution, manifest-driven domains, Caddy
  coexistence (import-fragment pattern), wake latency under `idle_stop`.
  See RFC 002 lifecycle section for the full candidate list.
- Desktop shell: keep Wails wrapper, or converge on menu-bar-first (KTStack
  pattern)? Defer to Phase 4.
- Windows service supervision without kardianos (dead upstream): WinSW vs
  schtasks vs sc.exe — decided alongside Phase 2 helper work.
- Whether `dnser elevate` may reuse sudo timestamps to avoid repeated
  prompts within a session.

## 11. Simplification ledger and resource budget

### 11.1 Code ledger

| Component (v1) | LOC (Go, non-test) | v2 fate |
|---|---|---|
| `proxyd`, `certs` | ~800 | deleted — Caddy |
| `dnscore` | ~700 | deleted — ctrld/dnsproxy |
| `runner` (supervisor, logs, ports, pathenv, signals) | ~1,150 | deleted — supervision tool + timberjack; `pathenv` existed solely because root spawned children |
| `dotdnser.go` | 623 | deleted — go.yaml.in/yaml/v4 |
| `service/` OS renderers | ~600 | deleted — platform-native service mgmt |
| daemon reconcile/state-mirroring | ~900 | replaced by deterministic manifest→config generator (~400 LOC); manifests are the only source of truth |
| imperative API/runner control | ~800 | thin aggregation; process ops behind supervisor REST, routing introspection via Caddy admin API |
| `detect` recipes-as-code | 362 | data-driven `--type` template registry (~120 LOC); accepts stock `Procfile` |
| `transfer`, imperative route/record commands | ~630 | deleted — manifests only |
| `setup/` scattered elevation | ~850 | one idempotent `helper apply/revert` on a recorded plan (~350) |
| Desktop/Wails + `-tags desktop` matrix | n/a | frozen/deleted — dashboard served by Caddy |
| Bespoke UI kit | 1,962 TS | Mantine v9 (maintained upstream owns the components) |

Target: **~13.3k → ~3k Go LOC (~75–80 % reduction)** and removal of the
CGO/webkit build dimension entirely.

### 11.2 Runtime rules

- **dnser leaves the data path**: browser → Caddy → app, one hop, always.
  The daemon is control plane only (manifest watcher, generator, wake
  hook, journal).
- Listener-free daemon where possible: dashboard virtual-hosted by Caddy;
  daemon IPC over a unix socket.
- Event-driven, not polled: debounced fsnotify + one metrics scrape per
  awake `on_request` project; v1's 150 ms/300 ms/750 ms/5 s tickers die.
- `GOMEMLIMIT` ≈ 48 MiB soft cap on the daemon enforces the RSS promise.
- Supervisor runs headless (`--tui=false`), file-backed logs, explicit
  DNS cache sizing, DoH bootstrap only when encrypted upstreams configured.
- Wake-hook design must never insert dnser into a working proxy path.

Steady-state targets on Apple Silicon that the Phase 3 component spike must
meet; missing any row reopens that component's choice:

| dnser-owned component       | Idle RSS | Idle CPU |
|-----------------------------|----------|----------|
| dnser orchestrator          | ≤ 40 MB  | ≈ 0 %    |
| Caddy                       | ≤ 30 MB  | ≈ 0 %    |
| DNS listener (ctrld/dnsproxy)| ≤ 15 MB | ≈ 0 %    |
| process-compose daemon      | ≤ 40 MB  | ≈ 0 %    |
| **Infrastructure total**    | **≤ 125 MB** | **≤ 1 %** |

User workloads deliberately sit outside this budget because they dominate it
in practice: one `next dev` instance typically holds ~400–800 MB (cold
compile spikes beyond 1 GB); four concurrent instances commit multiple GB
regardless of orchestrator. Consequences:

- The dashboard exposes per-process RSS/CPU so users can tell dnser's share
  from their apps' share.
- Per-project start/stop stays first-class (`dnser stop <project>`) so users
  pay only for what they actively develop.
- **On-demand lifecycle** cuts steady-state cost further: projects declare
  `availability: on_request` (started lazily on first request through a
  Caddy interception hook, ~1–2 s first-hit penalty) plus `idle_stop`
  (auto-stop after N minutes without traffic). A four-app setup where two
  are actively developed drops from multi-GB to roughly the sum of the two
  live apps plus MB-scale infrastructure.
- Health polling defaults off unless a service declares readiness (v1
  polled dead backends every 5 s — silent timeout churn).

## 12. Research references

- go-yaml archived; YAML-org fork `go.yaml.in/yaml/v4`: github.com/go-yaml/yaml, github.com/yaml/go-yaml
- goccy/go-yaml (alternative parser): github.com/goccy/go-yaml
- process-compose: github.com/F1bonacc1/process-compose
- Supervisor landscape incl. Overitall/Pitchfork (2026): tracyatteberry.com/posts/process_management
- Caddy automatic HTTPS + local CA (Smallstep): caddyserver.com/docs/automatic-https
- certmagic on-demand issuance + DecisionFunc: github.com/caddyserver/certmagic
- dnsproxy: github.com/AdguardTeam/dnsproxy · ctrld: github.com/Control-D-Inc/ctrld
- kardianos/service maintenance 0/10 (deps.dev Jul 2026): deps.dev/project/github/kardianos%2fservice · fork github.com/iyear/sysvc
- lumberjack staleness → timberjack: github.com/natefinch/lumberjack/issues/227, github.com/DeRuina/timberjack
- mkcert maintenance statement (Feb 2026): github.com/FiloSottile/mkcert/discussions/658
- Category prior art: KTStack, Yerd (yerd.app), Grove, Lerd (lerd.sh), LocalRun (github.com/mindantic/localrun), Hatch
- winget dependencies experimental: github.com/microsoft/winget-pkgs installer schema 1.5.0; scoop `depends`: ScoopInstaller wiki
- Go CLI frameworks 2026: danilchenko.dev/posts/go-cli-frameworks (Cobra vs urfave/cli vs Bubble Tea); adaptive-enforcement-lab.com/build/go-cli-architecture/framework-selection; charmbracelet/fang; charm.land ecosystem (bubbletea v2, huh v2, lipgloss, bubbles v2.1.1)
- Dashboard UI research 2026: shadcndeck.com/blog/mantine-vs-shadcn; woodcp.com 8-library bundle benchmark; devforgedev.hashnode.dev Mantine-for-dashboards; 21st.dev/blog/dashboard-component-libraries; codevup.com Tailwind-v4 alternatives survey

## 13. Researched decisions (was: spike candidates)

Each item below was decided from retrieved sources against this project's
requirements; links recorded. Re-open any of them only with new evidence.

### 13.1 JSON-Schema generation → `invopop/jsonschema`
Our two hand-maintained schema JSONs become build-time output generated from
the config structs (single source of truth; editors keep live completion).
Evidence: alecthomas/jsonschema is ARCHIVED and redirects maintenance to
invopop/jsonschema (MIT), which moved to draft 2020-12, is at v0.14.0
(Apr 2026), and is production-hardened inside Invopok's GOBL. Runtime
manifest validation stays strict-YAML-decode + cross-field checks (no
validator dependency); if schema-driven validation is ever needed,
google/jsonschema-go (official, Aug 2025, validation + inference) is the
candidate — noted, not adopted.

### 13.2 dotenv loading → `joho/godotenv`
De facto standard (10k★) whose Ruby/Node-compatible semantics users already
expect; supports ordered multi-file loads and existing-env precedence,
matching RFC 002 `env_file`. Accepted risk: an open issue questions release
cadence (#256) — but .env parsing is a frozen format, so stagnation means
zero-churn rather than rot. Documented fallback: `subosito/gotenv`
(active, StrictParse, identical semantics).

### 13.3 Updates → detect-and-defer, no blind self-update
Research consensus (sota-cli-ux distribution rules; SpecScore CLI self-update
spec Jun 2026; upsun/cli PR#107; go-tool-base series): overwriting a
package-manager-owned binary corrupts its bookkeeping. Therefore
`dnser update`: classify install source by executable path + nfpm marker
file (brew Cellar / scoop shims / deb-rpm marker) → **managed installs print
the exact manager upgrade command and exit**; curl/script installs get
checksum-verified (goreleaser `checksums.txt`) atomic in-place replacement;
`--check` mode is read-only; ambiguous detection always defers to guidance.
~150 LOC over GitHub Releases; no updater framework dependency.

### 13.4 Release/packaging automation → goreleaser OSS (+ nfpm)
One `.goreleaser.yaml` replaces repo scripts: multi-platform builds +
checksums + GitHub Releases + **homebrew formula with
`dependencies:`** (tap auto-published on tag) + **nfpms deb/rpm with
package `dependencies:`** (caddy/process-compose/mkcert pulled by apt/dnf).
Verified OSS scope; caveat found: **scoop manifest generation is
GoReleaser Pro-only** — Windows ships a small hand-maintained bucket JSON
(revisit Pro if Windows effort grows). Bonus capability recorded: brew
*casks* support `uninstall`/`zap` blocks (launchctl/trash/delete) — the
macOS purge-uninstall requirement maps natively if we ever ship an app.
Evidence: goreleaser.com customization docs (nfpm/homebrew_casks/scoop,
2026); mcginniscommawill.com personal-tap walkthrough (Mar 2026);
github.com/goreleaser/nfpm own `.goreleaser.yml`.

### 13.5 Terminal tables → `lipgloss/table`
We adopt lipgloss regardless (fang/huh pull it), so its table sub-package
adds zero new dependencies and styles per-cell with the same brand theme.
Alternatives evaluated: olekukonko/tablewriter (revived v1.x, Mar 2026;
richest features incl. streaming/markdown/SVG) and jedib0t/go-pretty (very
active, v6.8.1 Jun 2026; tables+progress+text). Both noted as upgrades if
output needs outgrow lipgloss tables — swapping is localized to status/explain
renderers.
