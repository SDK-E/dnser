# CLI reference

## Global contracts

- **stdout = data, stderr = everything else.** Progress, warnings and
  confirmations never pollute piped output.
- **Formats:** `-o text|json|ndjson` (default: `text` on a TTY, `json`
  when piped). List commands may emit NDJSON so `head`/`grep` compose.
- **Field pruning:** `--fields a,b,c` keeps only those top-level JSON keys.
- **No fuzzy matching:** unknown commands and flags exit 2 with usage, no
  "did you mean" guessing.
- **Idempotent:** every command can be re-run safely; concurrent runs fail
  fast (`another dnser command is running`) instead of queueing.
- **Agent-safe:** `--no-input` fails fast instead of prompting anywhere;
  every interactive value also has a flag or env equivalent.

### Global flags

| Flag | Effect |
|---|---|
| `-o, --output text\|json\|ndjson` | Output format (default: text on TTY, json when piped) |
| `--fields a,b,c` | Keep only these top-level JSON fields |
| `-y, --yes` | Skip moderate-severity confirmations |
| `--no-input` | Never prompt; fail fast instead |

### Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | operational failure (tool/network/process failed) |
| 2 | usage error (bad flag/argument/unknown command) |
| 3 | confirmation required — the mutation plan was printed; re-run with the shown `--confirm` invocation |
| 4 | elevation required — the exact elevated command is printed |
| 10 | outcome, not error: `doctor` found issues |
| 130 / 141 | SIGINT after cleanup / EPIPE from downstream `head` |

JSON errors carry `{error, kind, code, remediation}` where `remediation` is
the exact next command to run.

### Confirmation protocol

Mutating commands compute a plan first. On a TTY you get the plan plus
`[y/N]`. Without a TTY the plan is printed as JSON and the process exits 3
with the exact `--confirm` re-invocation. Severe operations (`uninstall
--purge`) additionally require typing the project name.

## Commands

### Project lifecycle

| Command | Notes |
|---|---|
| `dnser init [--type T] [dir]` | Scaffold `.dnser.yaml`; types: `laravel`, `symfony`, `nodejs`, `rails`, `go`, `python`, `bash`, `sh`, `zsh`, `static` |
| `dnser link <path>` | Register a project: validates strictly, pins ports, registers domains, generates infra. Safe to re-run |
| `dnser unlink <name>` | Remove registration and regenerate infrastructure |
| `dnser up [--project P]` | Ensure service/configs are current, start linked projects honoring availability tiers |
| `dnser down [--infra]` | Stop supervised projects gracefully; add `--infra` to also stop Caddy/DNS/service |
| `dnser start <project>` | Manual override of the availability tier until the manifest changes |
| `dnser stop <project>` | Stop one project |
| `dnser restart <project>` | Stop then start, with wake semantics identical to an `on_request` first hit |

Unknown project names exit 2 listing the known ones.

### Introspection

| Command | Notes |
|---|---|
| `dnser status [--project P]` | Daemon, DNS listener and Caddy with **actual** ports (never configured ones), per-project phase/PID/RSS/CPU, resolver health |
| `dnser logs <project> [-f]` | Print or follow supervisor-managed log files; stdout carries log lines only |
| `dnser explain [project]` | Fully-resolved effective configuration annotated by source (`manifest\|template\|detected\|default`). Env values redacted by default |
| `dnser schema` | Emit the generated JSON Schema (draft 2020-12) for `.dnser.yaml` for editor completion |
| `dnser dashboard [--port P]` | Serve the embedded web UI on 127.0.0.1 only, gated by a random token |

### System integration

| Command | Notes |
|---|---|
| `dnser elevate --suffix NAME --port PORT` | Apply privileged changes once: `/etc/resolver/*`, CA trust, background service. Atomic and journalled; re-running is a no-op |
| `dnser unelevate` | Replay captured inverses in reverse order, verify zero residue |
| `dnser update [--check] [--yes]` | Detect install source: brew/deb/rpm installs print the exact manager upgrade command; script installs get checksum-verified atomic replacement. `--check` is read-only |
| `dnser doctor [--fix]` | All health checks; each issue prints kind, evidence and the exact fix command. Exit 10 when issues are found. `--fix` applies the safe subset |
| `dnser uninstall [--purge] [--confirm NAME]` | Without `--purge`: stop runtime, keep state. With `--purge`: severe confirmation, then purge everything dnser ever touched, verified against the journal; residue prints itemized and exits 1 |

### Journal

| Command | Notes |
|---|---|
| `dnser journal ls` | List mutation plans |
| `dnser journal show PLAN_ID` | One plan with per-step states |
| `dnser journal finish PLAN_ID` | Complete an interrupted plan forward |
| `dnser journal revert PLAN_ID` | Roll a plan back using captured pre-state |

### Manifest migration

```bash
dnser migrate [path] [-y]
```

Dry-run diff by default; see [Migrating from v1](migrating-from-v1.md).
