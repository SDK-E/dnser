# Troubleshooting

## First stop: `dnser doctor`

Runs all health checks and prints, for each issue, its kind, evidence and
the exact fix command. Exit code is 10 when anything was found (an
outcome, not a crash). Checks include:

| Check | Symptom it catches |
|---|---|
| Resolver drift | `/etc/resolver/*` pointing at a port the listener no longer holds (60 s lookup hangs) |
| Dead resolver file | resolver entries left after the listener died — restores normal browsing when fixed |
| Interrupted plans | elevation or other mutations half-applied by a crash |
| Port conflicts | who owns a port dnser wanted, by PID |
| Shadowed public suffixes | a registered suffix like `io` swallowing real internet names |
| Stray listeners | orphaned processes from crashed supervisors still holding allocated ports |
| Config validity | generated Caddy/supervisor configs that fail validation |
| File permissions | key material not at 0600 |
| Tool versions | caddy/supervisor below the pinned minimum |
| Ownership | root-owned artifacts in project dirs |

`dnser doctor --fix` applies the safe subset automatically; anything
privileged asks first (or exits 3 with a plan when non-interactive).

## Reading the signals

```bash
dnser status            # actual ports, phases, PID/RSS/CPU per project
dnser explain <project> # where every value came from
dnser logs <project> -f # app output via the supervisor
```

Project phase meanings: `running`, `starting`, `sleeping`
(`on_request` stopped), `stopped`, `failing` (crash-looping; `logs` shows
the last exit reason).

## Common situations

**`demo.test` doesn't resolve.**
Fallback mode only resolves while the DNS listener runs — check `status`,
run `dnser up`. Elevated but stale? `doctor` flags resolver drift;
`doctor --fix` repairs it.

**Port already in use.**
Allocation walks the fallback chain (53→5353→35353 etc.). If exhausted,
the project isn't started and `status`/`doctor` name the owning PID. Free
the port or pin `port:` in the manifest.

**Caddy rejects the generated config.**
Last-known-good stays live; exit names the offending directive. Fix the
manifest (usually a bad raw `caddy:` merge) and re-run `link`.

**Project crash-loops.**
Supervisor backs off; routes return a branded 503 rather than hanging.
`dnser logs <project>` shows the exit reason.

**A command printed a plan and exited 3.**
That's the confirmation protocol: review `changes[]`, then re-run the
exact remediation command shown.

**Another dnser command is running.**
Single-instance lock; wait for the other invocation. Never force-remove
the lock while a mutation may be mid-flight.

## Recovering with the journal

Every mutation is recorded before it happens:

```bash
dnser journal ls               # list plans
dnser journal show PLAN_ID     # per-step states
dnser journal finish PLAN_ID   # complete an interrupted plan forward
dnser journal revert PLAN_ID   # roll back using captured pre-state
```

Rollback restores captured prior values exactly; things that did not exist
before are removed again, never invented.

## Clean uninstall

```bash
dnser uninstall            # stop runtime, keep state/configs
dnser uninstall --purge    # remove EVERYTHING dnser ever touched
```

`--purge` requires severe confirmation (typing the name). It stops
projects, removes the service, untrusts the CA, removes `/etc/resolver`
entries, deletes `~/.dnser/`, prints package-manager cleanup commands
(`brew uninstall dnser && brew autoremove`), then re-verifies against the
journal and itemizes any residue (exit 1 if anything remains).
