# Spike A — Project supervision: process-compose vs tenement

- Status: **Decided** (M0, RFC 005 §1)
- Verdict: **process-compose** (embedded as orchestrated binary, headless, REST-driven)
- Loser: tenement — documented below; reopen only with new evidence
- Date: 2026-08-24 · Platform measured: macOS 15 (darwin/arm64), console user uid 501

## 1. Method

Identical workload driven through each candidate per RFC 005 M0.1: one HTTP
server process + one idle sidecar under a supervisor config generated the way
dnser's generator will emit it. Measured on this machine with `ps` sampling;
REST/control-plane latencies timed with `curl`. tenement could not be executed
locally — see §4.

## 2. Measured results

| Criterion (RFC 001 §11.2 budget) | process-compose v1.122.0 | budget |
|---|---|---|
| Idle RSS (30–60 s window, 3 samples) | **6.0 / 7.8 / 2.8 MB** (settled ~2.8–6.5 MB) | ≤ 40 MB ✅ |
| Idle CPU | **0.0 %** throughout | ≈ 0 % ✅ |
| Runs as console user (I5) | verified uid 501, no elevation | required ✅ |
| Stop→start roundtrip via REST | **32 ms stop / 9 ms start** | n/a |
| Managed app answer after restart | HTTP 200 in 18.8 ms through supervisor | n/a |
| Process-group teardown on project stop | child reaped, port released | R6 ✅ |

Control-plane surface verified live (OpenAPI at `/swagger/doc.json`):
`POST/GET /process/{start,stop,restart}/{name}`, `PATCH /process/signal/{name}/{signal}`,
`PATCH /process/scale/{name}/{scale}`, `GET /process/logs/{name}/{endOffset}/{limit}`,
WebSocket streams `/process/states/ws` and `/process/logs/ws`,
`POST /project/configuration` (hot config swap without daemon restart),
namespaces start/stop/restart. This is exactly the contract M5/M8 consume.

tenement author-published wake numbers (not reproducible here): Python ~65 ms,
Node ~105 ms, Go ~140 ms cold wake — excellent, and moot given §4.

## 3. Distribution/maintenance evidence (retrieved 2026-08-24)

- process-compose v1.122.0 released 2026-08-17/18, Apache-2.0; 71 releases in
  ~4 years (~22 d cadence). https://github.com/F1bonacc1/process-compose/releases
- Official brew tap: `brew install f1bonacc1/tap/process-compose`
  (NOT homebrew-core — verified locally: core tap lacks it)
  https://f1bonacc1.github.io/process-compose/installation/
- darwin/arm64 release binary downloaded and exercised (43 MB, single static Go binary).
- Features relevant to RFC 002 lifecycle: health/readiness probes, restart
  policies incl. `exit_on_failure`, dependencies/ordering, envsubst env,
  log caching, replicas, scheduled processes, file-watch restarts with
  cascade (v1.122.0), token-authenticated REST+WS+UDS, headless `-t=false`.
  https://f1bonacc1.github.io/process-compose/launcher/

## 4. Why tenement loses

Retrieved facts (2026-08-24):

1. **Wrong language for the dependency story (G2 blocker).** tenement is a
   **Rust** binary ("It's a Rust binary…" — https://russellromney.com/blog/building-tenement,
   2026-04-03; repo Cargo.toml). dnser installs deps via brew/deb/rpm/scoop
   `depends_on`; tenement ships **no prebuilt assets** (release v0.1.4 asset
   list is empty — https://api.github.com/repos/russellromney/tenement/releases/latest,
   checked) and no brew formula exists (homebrew search returned none).
   Adopting it forces a Rust toolchain into dnser's build/release pipeline or
   an unofficial binary host.
2. **Routing model conflicts with locked decisions.** tenement IS the router:
   it reverse-proxies `<tenant>.<service>.<domain>` subdomains itself. RFC 001
   §11.2 locks "dnser leaves the data path: browser → Caddy → app, one hop"
   and §5 locks Caddy for TLS/SNI/routing. Using tenement either inserts a
   second proxy hop or displaces Caddy (reopening locked decisions).
3. **Tenant primitive ≠ arbitrary user domains (G4).** Its unit of routing is
   service:tenant instances under one wildcard domain; dnser projects declare
   arbitrary FQDNs/wildcards each. Mapping requires contortion.
4. **macOS second-class.** PID/mount namespace isolation is Linux-only; "On
   macOS it falls back to bare processes" (author blog). Our primary platform
   gets its least-hardened mode.
5. **Maturity risk.** "Experimental. Actively developed. APIs may change."
   (repo README); v0.1.x, 7 stars, 102 commits, single maintainer.
   https://github.com/russellromney/tenement
6. **Could not be executed locally** (no release assets, no cargo toolchain),
   so even the spike criteria it was favored on (wake latency) rest on
   author-published numbers only.

## 5. Consequence (plan of record)

- Supervisor = **process-compose**, run headless (`--tui=false`) as console
  user, file-backed logs, REST+UDS consumed by CLI and dashboard.
- `availability: on_request` / `idle_stop` / `min_uptime` are implemented by
  the already-spec'd lifecycle glue (~200 LOC): Caddy request metrics →
  process-compose REST start/stop, timers owned by dnser daemon (the fallback
  RFC 002 §3 pre-approved when the spike rejected tenement — it did).
- Packaging: brew formula `depends_on` cannot reference a tap-local formula
  from core automatically → dnser's formula declares
  `depends_on "f1bonacc1/tap/process-compose"` (tap+formula dependency is a
  supported brew mechanism) or vendors the binary; deb/rpm ship a bundled or
  repo-served process-compose package. Final call lands with M9 packaging work.

## 6. Raw artifacts

Spike workspace (transient, not committed): configs, logs, timing output from
the session that produced §2. Numbers above are copied verbatim from those runs.
