# Architecture

## The one-hop data path

```
        dnser (one binary, CLI-on-demand — no idle daemon)
   ┌─────────────┬──────────────────┬───────────────────┐
   │ DNS listener│ Caddy            │ process-compose   │
   │ *.test →    │ TLS (local CA)   │ start/stop/wake,  │
   │ 127.0.0.1   │ reverse proxy    │ readiness probes  │
   └─────────────┴───────────────────┴───────────────────┘
        every change journalled → dnser journal / revert / uninstall
```

dnser is a thin orchestrator: browser → Caddy → app, one hop, always. It
writes configs for three proven tools and never sits in the data path:

- **DNS listener** (embedded dnsproxy): registered suffixes answer
  locally, every unhandled query forwards upstream untouched — breaking
  normal browsing is a release blocker. Names outside your suffixes are
  forwarded as-is.
- **Caddy**: binds 80/443 (or fallbacks), issues leaf certificates
  on-demand from a local CA restricted to registered suffixes, reverse
  proxies to project ports.
- **process-compose**: supervises project processes **as the console
  user**, with restart policies and TCP readiness probes. `dnser start`
  returning "ready" means the probe accepted.

## Privilege model

A privileged helper performs exactly three mutation classes, once,
atomically and only via explicit `dnser elevate`:

1. write/remove `/etc/resolver/<suffix>` files (macOS),
2. install/remove the local CA trust,
3. install/remove the background service definition.

Everything else — Caddy, DNS listener, process-compose, your processes —
runs as you. User project processes **never** run as root; `up` refuses to
run if its effective uid is root.

## Port fallback chains

Privileged ports degrade gracefully instead of failing:

| Port | Fallback chain |
|---|---|
| DNS | 53 → 5353 → 35353 |
| HTTP | 80 → 8080 |
| HTTPS | 443 → 8443 |

`status` always shows the *actual* bound ports, and resolver files are
written only after the listener is probed answering — so a stale
`/etc/resolver` entry pointing at a dead port cannot hang your lookups.

## Safety contracts

Every mutating command runs inside one wrapper:

```
PLAN → CAPTURE pre-state → APPLY (write-ahead journal) → VERIFY → REPORT
```

- Pre-state (file bytes+perms, NIC DNS lists, port bindings) is captured
  *before* acting; "was previously unset" is itself recorded.
- Journal entries are fsync'd before each step runs, so a crash at any
  instant leaves a resumable record (`journal finish` / `journal revert`).
- Success means verified end-state — "it ran" is not success.
- Generated configs are written tmp+rename and validated (`caddy
  validate`) before swap; on failure the last-known-good stays in place.
- A single-instance lock makes concurrent mutating invocations fail fast
  instead of racing.

The full invariants (I1–I7) each have a scripted regression test in
`internal/e2e/invariants_test.go`, named after the invariant so a failure
maps directly to the contract.

## Resource profile

Measured steady-state infrastructure budget: dnser ~11 MB, DNS listener
~2 MB, process-compose ~8 MB, Caddy external — comfortably inside ≤125 MB
total, ≈0% idle CPU. Your apps' memory dwarfs this, which is why
per-project start/stop and `availability: on_request` + `idle_stop` exist:
pay only for what you actively develop.

## State layout

All state lives under `~/.dnser/`: config, generated Caddy/supervisor
configs, logs (rotated), certs, the mutations journal. `uninstall --purge`
deletes it entirely and verifies against the journal that nothing was
missed.
