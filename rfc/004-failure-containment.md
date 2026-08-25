# RFC 004 — Failure Containment: "A failed step never breaks the machine"

- Status: **Proposed** (extends RFC 003 flows)
- Evidence base: cfgd safety docs (transaction journal, backups, reverse-
  order rollback, validate→backup→write→restart→restore-on-fail), gel
  (eval/apply split, prior-value capture, "unset stays unset"), SONiC
  generic config update (checkpoint → apply → post-validate → rollback,
  single concurrent session), patchlog (ordered idempotent teardown),
  AKKU/endstate (plan/approve/verify-first, automatic rollback on failure,
  non-destructive defaults), clig.dev (crash-only design).

## 1. The universal mutation wrapper

Every mutating flow (elevate, link, up, migrate, uninstall…) wraps its steps
in one shape:

```
PLAN      pure, read-only; rendered for confirmation (RFC 003 §1.3)
CAPTURE   per-resource pre-state written to the journal BEFORE acting:
          file bytes+perms, unit enable-state, NIC DNS lists, port bindings;
          "was previously unset" is itself recorded — rollback restores
          absence, it never invents a value (gel rule)
APPLY     steps executed in dependency order; each step's inverse is derived
          from the captured pre-state, and the journal entry is fsync'd
          BEFORE the step runs (write-ahead), so a crash at any instant
          leaves a resumable record
VERIFY    success = observed end-state (port answers, file content matches,
          service healthy) — "it ran" is not success (endstate rule)
REPORT    per-step status; failures name the step, evidence, and the exact
          repair command
```

Rollback = replay captured inverses in reverse order, skipping artifacts
that no longer exist (patchlog idempotency). System-level actions that
cannot be safely auto-reverted are flagged in the journal for `doctor`
review instead of being silently attempted (cfgd rule).

## 2. Machine invariants (hold during every failure mode)

| # | Invariant | Enforced by |
|---|---|---|
| I1 | Normal browsing survives dnser dying at ANY instant | `/etc/resolver/*` is written only AFTER the DNS listener is probed answering (health gate before switch); a watchdog marks the entry dead after repeated probe failures and `doctor --fix` / `down --full` removes it. A missing resolver file is always safe; a stale one is the bug class we lived (60 s lookup hangs) |
| I2 | System DNS settings are always restorable | Captured per-NIC into the journal before any networksetup/resolvectl/netsh call; `unelevate` restores exactly what was captured |
| I3 | Nothing outside `~/.dnser` is ever deleted without severe confirmation | Deletion steps exist only in `uninstall --purge`; everything else creates/modifies with backups |
| I4 | CA trust is never left half-applied | Trust install/remove is a single helper action recorded before execution; failure ⇒ journal carries the inverse; purge removes the key only after untrust succeeded |
| I5 | User code never executes as root | UID asserted immediately before spawn (R1); refusal is loud, not a fallback |
| I6 | Bad generated configs never take traffic | tmp+rename writes; `caddy validate` / supervisor config-check before swap; validation failure ⇒ last-known-good stays in place and is reported (cfgd service pattern) |
| I7 | Every wait is bounded | Helper calls, port-wait, readiness probes, upstream fetches all carry deadlines; Ctrl-C once = graceful, twice = abandon cleanup (clig.dev) |

## 3. Failure playbook (what the user sees)

| Failing step | Immediate containment | User-visible output | Repair path |
|---|---|---|---|
| Elevation refused/timed out | nothing applied; plan discarded | exit 4 + exact elevated command | re-run `elevate` |
| Helper died mid-plan | journal shows applied vs pending steps | "applied X/Y; run `dnser doctor --fix`" | doctor completes or reverses (R3) |
| Listener won't start | resolver files NOT written (I1); projects stay stopped | exit 1 + log tail + `doctor` hint | fix cause, re-run `up` |
| Caddy config invalid | last-known-good retained (I6) | exit 1 naming the directive/file | fix manifest, regenerates |
| Project process crash-loops | supervisor backoff owns it; route returns branded 503, not a hang | status shows `failing` + last exit reason | `logs`, manifest fix |
| Port conflict at spawn | fallback chain walked; if exhausted, project not started (others unaffected) | names conflicting PID/process | free port or set `port:` |
| Update download/checksum fail | original binary untouched | exit 1, nothing replaced | retry / manual install |
| Uninstall step fails mid-purge | remaining steps still attempted where independent; journal tracks residue | exit 1 + itemized leftover list | `doctor --fix` or manual list |

## 4. Additions to the command surface

- `dnser journal [--last N] [--verify]` — read-only view of mutation
  history; `--verify` re-checks each entry's end-state against reality
  (drift report feeding `doctor`).
- `doctor --fix` gains journal-aware completion/reversal of interrupted
  plans (R3), keeping one repair entry point instead of per-flow recovery
  flags.

## 5. What we deliberately do NOT do

- No automatic filesystem snapshots (OS-specific, heavy); journal +
  captured-bytes backups cover our blast radius, which never leaves
  `~/.dnser`, `/etc/resolver`, NIC DNS settings, keychain trust, and
  service definitions.
- No auto-retry of privileged operations after refusal — a denied password
  is a decision, not an error to outlive.
- No best-effort deletion outside confirmed purge scope.
