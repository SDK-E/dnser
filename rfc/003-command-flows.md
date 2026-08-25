# RFC 003 — Command Flows & Safety Contracts

- Status: **Proposed** (implements RFC 001/002)
- Failure semantics for every flow below: `004-failure-containment.md` —
  all mutating commands run inside the PLAN→CAPTURE→APPLY→VERIFY→JOURNAL
  wrapper with machine invariants I1–I7.
- Evidence base: clispec.dev (structured-output/exit-code/prompt spec),
  clig.dev (canonical CLI guidelines), Arcjet agent-facing CLI design
  (blog.arcjet.com, Jun 2026), sota-cli-ux rules, InfoQ agent-CLI patterns
  (Aug 2025), alexfurrier.dev agent-friendly CLI principles (Mar 2026).

## 1. Global contracts (every command inherits these)

### 1.1 Streams and output
- **stdout = data, stderr = everything else.** Progress, spinners, tables,
  warnings, confirmations, "Done in 2.1s" all go to stderr.
- Formats: `-o text|json|ndjson` (default: `text` on TTY, `json` when piped;
  piped default declared in the machine-readable schema). Lists may emit
  NDJSON so `head`/`grep` compose. No ANSI when not a TTY; `NO_COLOR` and
  `TERM=dumb` honored everywhere.
- `--fields a,b,c` prunes top-level JSON keys (agents pay context for bytes).
- Every read/list command has stable JSON field names — **versioned API**,
  additive-only changes; golden-file regression tests in CI guard them.

### 1.2 Exit codes (documented in `--help`)
| Code | Meaning |
|---|---|
| 0 | full success |
| 1 | operational failure (tool/network/process failed) |
| 2 | usage error (bad flag/arg; also unknown command — fuzzy "did you mean" is disabled) |
| 3 | confirmation required (mutation plan returned, see 1.3) |
| 4 | elevation required (helper must run; prints exact elevated command) |
| 10 | outcome, not error: `doctor` found issues |
| 130 / 141 | SIGINT after cleanup / EPIPE from downstream `head` |

JSON-mode errors carry `{error, kind, code, remediation}` where
`remediation` is the exact next command. Partial failure never exits 0.

### 1.3 Confirmation protocol (replaces interactive "are you sure?")
Mutating commands compute a **plan first**: on a TTY, render the plan +
`[y/N]` prompt (Enter aborts); without a TTY, print the plan as JSON and
exit 3 with the exact `--confirm` re-invocation. Agents show `changes[]` to
the human and re-run. Severity classes:
- **moderate** (`link`, `up` first-run writes, `migrate`): plan shown, `-y` skips.
- **severe** (`uninstall --purge`, removing a project's data): requires
  typing the name or `--confirm "<name>"`; `--yes` alone is refused.
Every interactively-gathered value has a flag/env equivalent; `--no-input`
forces fail-fast instead of prompting anywhere.

### 1.4 Invariants
- **Idempotent by design** — agents retry; every command re-runs safely.
- **Single-instance lock** (`flock` on the control socket): concurrent
  mutating invocations fail fast with `hint: another dnser command is
  running`, never queue silently.
- **Crash-only**: all state files written tmp+rename; any command may die at
  any point; next command reconciles from disk truth.
- First output within ~100 ms; operations >2 s show progress on stderr
  (TTY-gated).
- `--help` includes realistic examples *and* "when not to use" hints.

## 2. Risk register → where mitigations live

| # | Risk | Mitigation (flow step) |
|---|---|---|
| R1 | Root-owned project artifacts (v1 bug) | Processes spawn only via supervisor running as console user; `doctor` verifies ownership of `<project>/.next` style dirs; `up` refuses if effective uid is root |
| R2 | Stale `/etc/resolver/*` port drift (v1 bug) | Every daemon start + `doctor` re-reads resolver files and compares against bound port (`ResolverDomainStale`); e2e gate fails loudly (exit 77 pattern) on mismatch |
| R3 | Partial elevation leaves half-applied system state | Helper applies an atomic plan recorded in the mutations journal; `unelevate` replays inverse; `doctor --fix` completes interrupted plans |
| R4 | Port conflicts / privileged-port fallback | Allocation walks fallback chain; `status` always displays *actual* ports; conflicts reported with owning PID |
| R5 | Public-suffix hijack shadows real internet names | `link` warns loudly; `explain` lists shadowed suffixes; `unelevate` removes registrations |
| R6 | Orphaned processes after daemon/supervisor crash | Supervisor owns process groups; startup sweep compares registry PIDs vs liveness and reaps strays found listening on our allocated ports |
| R7 | Corrupted/half-written generated configs | Generator writes tmp+rename; `caddy validate` / supervisor config check run before swap; last-known-good retained and restored on validation failure |
| R8 | Two writers racing (CLI + hot reload) | Single-instance lock (1.4); reload coalesces via watcher debounce |
| R9 | Secrets exposure | `env_file`/`env` values redacted in `explain`/logs by default (`--redact=false` to override); key material perms asserted 0600 |
| R10 | Tool version drift breaks generated configs | Packaging pins caddy/supervisor versions; start gates on minimum versions with remediation message |
| R11 | Uninstall leaves residue | Journal-driven purge + final verification pass that **prints any residue instead of exiting silently** |
| R12 | Daemon down ⇒ `.test` lookups hang browsing | Resolver entries point at listener that answers-or-forwards instantly; if listener absent, `doctor` detects dead resolver file and offers removal (restores normal browsing immediately) |
| R13 | Agent hallucinates flags/commands | No fuzzy suggestions; unknown = exit 2 with usage; `--help` carries examples and when-not-to-use |

## 3. Command flows

Preconditions shared by all: lock acquired (1.4); config loaded (strict);
supervisor/Caddy reachability probed lazily — commands that don't need them
never touch them.

### 3.1 `dnser init [--type T] [--dir D] [--name N] [--port P] [--domain D]`
**Purpose:** scaffold `.dnser.yaml`. **Class:** moderate (writes one file).
Flow: resolve dir → load template registry for `--type` (interactive huh
wizard iff TTY and flags missing) → detect stack → fill conventions →
**validate against schema** → if file exists: diff old/new, require
confirmation (moderate) → write atomically → print next-step breadcrumb
(`dnser link <dir>`).
Failures: unknown type → exit 2 listing types; invalid template → exit 1
with line numbers. Never touches daemon.

### 3.2 `dnser link <path>`
**Purpose:** register project with daemon (watch manifests; generate infra).
Flow: validate manifest strictly → allocate/pin port (R4 chain) → register
domains (warn R5 on public suffixes) → trigger generator → report resolved
summary (domain, port+source, availability tier, services) → suggest `up`.
Idempotent: re-link = refresh + diff report. Rollback: unlink removes
registration and regenerates.

### 3.3 `dnser up [--project P]` / `dnser down`
`up`: ensure service installed+running (if not: exit 4 with exact elevated/
manager command — never self-elevate silently) → ensure Caddy/DNS configs
current (R7 validate-before-swap) → start requested projects honoring
availability tiers (`on_request` registers route only) → stream readiness
lines → summary with URLs and actual ports (R4).
`down`: stop supervised projects (graceful, supervisor-managed), keep
infrastructure running unless `--infra` also stops Caddy/DNS/service.
Ctrl-C once = graceful stop; twice = skip cleanup (clig.dev).

### 3.4 `dnser start|stop|restart <project>`
Manual override of availability tier until next manifest change (state kept
in runtime only, never persisted into manifests). Unknown project → exit 2
listing known ones (exit-code-as-data). `restart` = stop→start with wake
semantics identical to `on_request` first hit.

### 3.5 `dnser status [-o json] [--project P]`
Read-only, never touches supervisor controls beyond queries. Shows: daemon,
Caddy, DNS listener (with **actual ports**, R2/R4), per-project state
(running/starting/sleeping/stopped, pid, RSS/CPU — RFC 001 §11 dashboard
parity), domains, upstreams, resolver-file health (R2). Piped default JSON.

### 3.6 `dnser logs <project> [-f] [--since D]`
Tails supervisor log files (timberjack-rotated). stdout = log lines only;
NDJSON mode adds `{ts, stream, line}` envelopes. `-f` follows with clean
SIGINT exit 130.

### 3.7 `dnser explain [project]`
Prints fully-resolved effective configuration annotated by source
`(manifest|template|detected|default)` — RFC 002 rule 3. Env values
redacted unless `--redact=false` (R9). Read-only; JSON mode is the
machine contract used by tests.

### 3.8 `dnser doctor [--fix]`
Runs all checks: R1 ownership, R2 resolver drift, R3 journal completeness,
R4 ports, R5 shadowed suffixes, R6 stray listeners, R7 config validity,
R9 perms, R10 versions, R12 dead-resolver. Exit **0 clean / 10 issues-found**
(outcome, not error). Each issue prints `kind`, evidence, and the exact fix
command; `--fix` executes the safe subset, asking confirmation (moderate)
for anything privileged.

### 3.9 `dnser elevate` / `unelevate`
`elevate`: builds plan (resolver files, CA trust, service install) →
single helper invocation applying it transactionally (R3) → journal entries
appended per item → verification pass → summary. Re-running = no-op
reporting "already applied". `unelevate`: inverse replay + verification of
zero residue. Both print what will change before requesting privileges;
refusal at password prompt aborts cleanly with nothing applied.

### 3.10 `dnser update`
Detect-and-defer (RFC 001 §13.3): classify install source → managed:
print `brew upgrade dnser` etc. and exit 0; script/manual: fetch release,
verify checksums.txt, atomic replace; `--check` read-only; ambiguous →
guidance, never overwrite (R10 adjacent).

### 3.11 `dnser migrate`
v2→v3 manifest rewrite: dry-run diff by default (moderate: `-y` applies);
shows every rewrite performed (label→FQN etc.); journals the change;
original backed up alongside.

### 3.12 `dnser uninstall [--purge] [--yes|--confirm NAME]`
**Severe.** Flow: stop projects → stop/remove service → CA untrust →
remove resolver entries + restore captured NIC DNS → delete state dirs →
package-manager guidance printed (`brew uninstall && brew autoremove`) →
**verification pass re-checks journal; prints residue list**; non-empty
residue = exit 1 with itemized leftovers. Without `--purge`: stops runtime
only, keeps config/state, says exactly that.

### 3.13 `dnser schema [config|project]`
Emits generated JSON Schema (invopop/jsonschema from the Go structs — same
source the binary validates against). CI diffs emitted schemas to catch
breaking output/flag changes.

### 3.14 Removed vs v1
No imperative `route`/`records` editing (manifest-only), no `transfer`
(manifests are portable), no interactive setup wizard beyond `elevate`.

## 4. Non-goals
Telemetry (none), background auto-update daemons (none), server/multi-host
flows (out of scope per RFC 001 §3).
