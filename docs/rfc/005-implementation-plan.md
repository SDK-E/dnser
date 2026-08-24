# RFC 005 — v2 Implementation Plan (single master plan)

- Status: **Proposed**
- Consumes: RFC 001 (architecture), 002 (manifest), 003 (flows),
  004 (failure containment). Where those conflict, the later RFC wins.
- Rule: every milestone ends `go build ./... && go test ./... &&
  golangci-lint run` green. No milestone merges with red gates.

## 0. Strategy

One sustained effort on a `v2` integration branch, merged to `main` only at
M9. The two architecture spikes run first because they gate design details
downstream; everything else is deterministic glue around decisions already
made. The v1 binary remains shippable from `main` until M8; after that the
deletion ledger (RFC 001 §11) executes and v1 is gone.

Effort sizing is focused-work estimates, not calendar promises.

## 1. Milestones

### M0 — Spikes & scaffolding  *(~3–4 d)*
Blocks: M2, M5.
1. **Supervisor spike**: drive identical workload (next-style app +
   external service) through process-compose REST and tenement. Score:
   macOS console-user execution, wake-on-request latency, idle-stop,
   manifest-driven domain routing fit, Caddy coexistence, resource idleness.
   Output: written verdict; loser documented.
2. **DNS spike**: ctrld vs dnsproxy behind our resolver-file writer. Score:
   idle RSS/CPU (budget RFC 001 §11.2), split-DNS wildcard behavior,
   forwarding robustness, config-as-generated-file ergonomics.
3. Scaffold: new package layout (`internal/orchestrator`,
   `internal/generator`, `internal/helper`, `internal/journal`,
   `internal/cli/*`); add deps yaml/v4, fang, huh, lipgloss, timberjack,
   cenkalti/backoff, invopop/jsonschema, godotenv; wire fang onto root cmd.
4. Freeze desktop: exclude from v2 build; record deletion in ledger.

Acceptance: two spike verdict docs in `docs/spikes/`; skeleton builds;
`dnser --help` renders through fang.

### M1 — Config core & contracts  *(~3–4 d)*
1. Schema v3 types (RFC 002): strict decode (`KnownFields`), precedence
   flag > manifest > template > default, placeholders `{port}`,
   `{port:name}`, `{domain}`, `{logs_dir}`.
2. Template registry: embedded YAML defaults + `~/.dnser/templates/`
   override; `--type` list; detection hints data-driven (replaces
   `internal/detect` recipes).
3. `invopop/jsonschema` generation for both schemas; CI job diffs emitted
   schema vs committed (breakage alarm).
4. `godotenv` for `env_file`; redaction helper used everywhere values print.
5. Port allocation cache (persisted, stable across reloads).

Acceptance: table tests for decode/precedence/placeholders; golden schemas;
`dnser init --type=…` writes valid manifests for nodejs/laravel/bash/static.

### M2 — Generator  *(~4–5 d)*
Pure functions: `(config, state) → {Caddyfile fragment, supervisor config,
resolver registrations}`. No I/O beyond writes given.
1. Caddy adapter: site blocks per declared name, on-demand internal issuer
   restricted to registered suffixes (permission callback into dnser),
   reverse_proxy to pinned ports, branded 503 page for sleeping projects.
2. Supervisor adapter per M0 winner (process-compose YAML or tenement
   config), including availability tiers, readiness, env injection
   (PORT/DNSER_*), service entries.
3. Atomic tmp+rename emission; `caddy validate` + supervisor config check
   before swap; last-known-good retention (invariant I6).
4. Golden-file tests across manifest matrix (minimal → full RFC 002 §6).

Acceptance: generator is side-effect-free besides writes; goldens stable;
invalid manifests can never reach disk-swapped configs (forced-failure
tests).

### M3 — DNS layer  *(~2–3 d)*
1. Listener lifecycle for M0 DNS winner; health probe gate.
2. Resolver writer: per-suffix files, content verified at every start
   (`ResolverDomainStale` semantics), watchdog removes dead entries (I1),
   suffix warning on public TLDs (R5).
3. Fallback mode without elevation: high-port listener, no resolver writes,
   explicit notice (never blocks).

Acceptance: forced-crash tests prove I1 (kill listener → entry removed →
lookups stop hanging); public-suffix warning test.

### M4 — Privilege helper & journal  *(~4–5 d)*
1. Journal format + fsync-before-step wrapper (RFC 004 §1); capture
   pre-state (files bytes+perms, NIC DNS, enable-states, unset-marker).
2. Helper contract: one elevated invocation applying an atomic plan
   (resolver files, CA trust via mkcert/caddy, service install); bounded
   timeouts; refusal aborts cleanly.
3. `elevate`/`unelevate` commands per RFC 003 §3.9; `journal` command;
   `doctor --fix` becomes journal-aware (complete-or-reverse interrupted
   plans).
4. Platform service definitions: brew-services hint / systemd user unit /
   Task Scheduler — declarative files, no Go renderers.

Acceptance: kill -9 mid-plan ⇒ journal resumable, `doctor --fix` converges
(I-tests); unelevate returns machine to captured pre-state byte-for-byte.

### M5 — Project lifecycle  *(~4–5 d)*
Per M0 winner: start/stop/restart, restart policies, readiness gating,
availability tiers, `on_request` wake (Caddy interception → wake endpoint,
request held ≤ ready), `idle_stop` + `min_uptime` timers, crash backoff,
stray sweep at startup (R6). UID assert before every spawn (I5/R1).
Wake-hook rule: never inserted once project is up (one hop guarantee).

Acceptance: lifecycle state-machine table tests; on_request end-to-end
(first hit wakes, subsequent hits direct); idle timer fires exactly after
quiet window; SMTP-class service cannot be `on_request` (validation error).

### M6 — CLI surface completion  *(~4–5 d)*
All RFC 003 commands with their exact flows: init/link/up/down/
start|stop|restart/status/logs/explain/doctor/update/migrate/uninstall/
schema/elevate/unelevate/journal. Global: `-o text|json|ndjson`, `--fields`,
exit-code table, confirmation envelope (exit 3 + `--confirm`),
severe-typing for purge, single-instance flock, no fuzzy suggestions,
progress on stderr TTY-gated, 100 ms first output.

Acceptance: CLI-contract regression suite (golden JSON per command, exit
codes asserted); non-TTY runs never prompt (forced-pipe tests); every
command's `--help` carries examples + when-not-to-use.

### M7 — Failure containment integration  *(~2–3 d)*
Wire RFC 004 playbook into every flow: plan/capture wrappers on link/up/
migrate/uninstall; residue verification pass printing leftovers (exit 1);
update detect-and-defer implementation (path classification + checksummed
replace for manual installs); degraded modes (503 page, fallback DNS).

Acceptance: chaos tests — kill -9 daemon/helper/supervisor at randomized
steps ⇒ invariants I1–I7 hold, journal resumable, browsing unaffected.

### M8 — Dashboard (Mantine)  *(~4–6 d)*
Mantine v9 + mantine-datatable + TanStack Query; Apple-grade theme tokens
(system font stack, whitespace scale, low-alpha shadows); brand tokens
`#082003`/`#2cdb16`, logo untouched. Views: projects table (state/pid/
RSS/CPU/domains), log viewer (NDJSON stream), settings, doctor report with
one-click fixes, per-process resource display. Served by Caddy vhost;
auth = loopback-only + token.

Acceptance: feature parity checklist vs v1 panels; lighthouse-ish sanity
(no layout thrash on log stream); brand review vs logo/colors.

### M9 — Packaging, purge, release  *(~3–4 d)*
1. `.goreleaser.yaml`: builds+checksums+GH release; tap formula with
   `depends_on [caddy, process-compose, mkcert]`; nfpms deb/rpm with
   `Depends:`; hand-held scoop bucket JSON; cask deferred until GUI exists.
2. `uninstall [--purge]` full flow (RFC 003 §3.12) with residue
   verification exit-1 semantics.
3. `dnser update` shipped (detect-and-defer).
4. Docs: README v2 quickstart, migration guide (`dnser migrate`), man pages
   via fang.

Acceptance: clean-machine install scripts for macOS/Linux/Windows-scoop;
purge leaves zero journal-verified residue; release = `git tag && push`.

### M10 — E2E, hardening, cut-over  *(~3–4 d)*
1. Port e2e suite: real binaries, sandboxed home, high ports, fake
   upstream; loud-skip gate asserting `/etc/resolver` ↔ bound-port match
   (exit 77 pattern).
2. Invariant regression pack: scripted I1–I7 violations must be contained.
3. Perf budget run: idle infra RSS/CPU vs RFC 001 §11.2 table; fail =
   component reopen.
4. Deletion day: remove `proxyd`, `certs`, `dnscore`, `runner`, `service`,
   `logstream`, `dotdnser.go`, `transfer`, imperative route/records
   commands, desktop tree, bespoke UI kit; ledger updated with final LOC.
5. Merge `v2` → `main`; tag rc.

Acceptance: full gates green post-deletion; budget met; rc installable via
all three channels; `dnser migrate` round-trips v1 projects (mailbox/auth/
redisk fixtures).

## 2. Dependency graph

```
M0 ─┬─→ M2 ─→ M5 ─┐
    ├─→ M3 ───────┤
    └─→ M1 ─→ M4 ─┼─→ M6 ─→ M7 ─→ M10 ─→ merge
                  └─→ M8 ──────────┘
M9 ────────────────────────────────┘ (needs M6/M7 surfaces; UI optional)
```

## 3. Standing risks (condensed; full registers in RFC 003 §2 / 004)

| Risk | Trigger | Response |
|---|---|---|
| Tenement fails macOS checks | M0 | fall back to process-compose + lifecycle glue (~200 LOC) already spec'd |
| dnsproxy can't answer authoritative wildcards cleanly | M0 | ctrld path; worst case keep tiny answer-zone shim over miekg/dns (documented exception) |
| goreleaser OSS gaps bite (scoop) | M9 | static bucket JSON; Pro purchase decision deferred |
| Mantine major-version churn | M8+ | pin minor; dashboard surface small |
| Scope creep during deletion day | M10 | ledger is the contract; additions require RFC amendment |

## 4. Total estimate

~30–45 focused days solo, dominated by M2/M5/M6/M8. First externally usable
rc lands at M10; internally usable `up/status/link` loop exists from M6.
