# v2 cut-over ledger (M10 final)

Recorded at M10 completion, per RFC 005 step 4 ("ledger updated with final LOC")
and RFC 001 §11.1/§11.2.

## Code ledger — final

| Metric | v1 | v2 actual |
|---|---|---|
| Go LOC (non-test, `internal/`) | ~13.3k | **8,102** |
| TS LOC (bespoke UI kit) | 1,962 | 0 (Mantine owns components; our app code is TSX but framework-free of bespoke kit) |
| CGO/webkit build dimension | yes | removed |

Package breakdown (non-test Go LOC): cli, config, dnsl, generator,
helper, journal, orchestrator, state, dashboard (embedded SPA), buildinfo, e2e.

Honest note: the ~3k aspirational target assumed thinner CLI/daemon glue.
v2 landed the full RFC 003 command surface with its own output/exit-code
contract layer (~2.6k in `internal/cli` alone), which v1 spread across
imperative commands excluded from that target. The deletion-day goals —
proxyd/certs/dnscore/runner/service/logstream/desktop/bespoke-UI all absent
(enforced by `TestDeletionDayLedgerFinalLOC`) — are met.

## Perf budget — measured vs §11.2

| Component | Budget | Measured |
|---|---|---|
| dnser orchestrator (dashboard process) | ≤ 40 MB / ≈0 % | **11 MB / 0.000s CPU over 2 s** (`TestPerfBudgetIdleDashboard`) |
| DNS listener (dnsproxy embedded) | ≤ 15 MB / ≈0 % | 1.8–2.0 MB (spike B, docs/spikes/002) |
| Caddy | ≤ 30 MB / ≈0 % | external binary; not measured here |
| process-compose daemon | ≤ 40 MB / ≈0 % | 2.8–7.8 MB (spike A, docs/spikes/001) |
| Infrastructure total | ≤ 125 MB / ≤ 1 % | comfortably inside budget on available rows |

## Invariant regression pack

Scripted containment proofs live in `internal/e2e/invariants_test.go`,
one test per invariant I1–I7, named after the invariant so a failure maps
directly to the RFC 004 table.

## E2E

Real-binary flows (init/link/status/explain, resolver↔bound-port doctor
gate) live in `internal/e2e/e2e_test.go`, sandboxed `$HOME`, high ports,
fake upstreams.

## Cut-over status

- Branch: work has landed directly on `main` (no v2 branch divergence).
- rc tag: pending owner prerequisites — create `SDK-E/homebrew-tap` and add
  `TAP_GITHUB_TOKEN`; then `git tag v0.1.0-rc.1 && git push origin v0.1.0-rc.1`.
