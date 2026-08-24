# Troubleshooting

## Managed dev-server commands crash-loop with `exit status 127`

**Symptom** — `dnser ps` shows a project crash-looping; the log stream repeats:

```
started pid 4211: pnpm dev --port 52123 (port 52123)
/bin/sh: pnpm: command not found
process exited (exit status 127); restarting in 2s (restart #2)
```

`dnser ps` annotates the failure directly:
`exit status 127 — "pnpm" not found in daemon PATH; run 'dnser doctor' for details`.

**Why it happens** — a daemon started by launchd, systemd or the desktop app
inherits that environment's minimal `PATH`
(`/usr/bin:/bin:/usr/sbin:/sbin` on stock macOS). Managed commands run through
`/bin/sh -c`, so the first word of your command resolves against *the daemon's*
PATH — not the PATH of your interactive shell. Tools installed by Homebrew
(`/opt/homebrew/bin`), pnpm (`~/Library/pnpm/bin`), Volta, nvm/fnm/asdf shims,
Cargo or Go are invisible there.

**The fix is automatic.** At startup the daemon builds an augmented PATH and
uses it for every managed command and service:

1. the invoking user's login-shell PATH (`$SHELL -lc 'echo $PATH'`), captured
   once and cached in `~/.dnser/path-cache.json`;
2. well-known tool directories that exist on disk — Homebrew on macOS,
   `/usr/local/bin`, `/snap/bin`, `$HOME/.local/bin`, `$HOME/bin`;
3. version-manager directories — nvm, fnm, asdf/mise shims, Volta, pnpm
   (`$PNPM_HOME` or the platform default), Cargo, Go, Bun, Deno;
4. the daemon's inherited PATH last.

Duplicates collapse to the first occurrence. The cache refreshes after
`settings.path_refresh_minutes` (default 24 h), when missing, or when `$SHELL`
changes — so installing a new tool later just needs the TTL to elapse or a
daemon restart.

Power-user override: set `DNSER_EXTRA_PATH` in the daemon's environment;
those entries are prepended verbatim.

**Verify**

```sh
dnser doctor          # → ✓ commands  all managed commands resolve in daemon PATH
dnser start --foreground   # watch the augmented-PATH note at startup
```

**Doctor proactively flags this class of problem**: the `commands` check warns
when any linked project's stored command (primary or service) has a binary
that does not resolve in the effective PATH, before you ever hit the loop.

### Related pitfalls

- **Editing the launchd plist manually does not stick.**
  `~/Library/LaunchAgents/enterprises.sdk.dnser.plist` is regenerated on every
  plain `dnser start`; hand-added `EnvironmentVariables` keys are clobbered.
- **Root daemons and user homes.** When the desktop app spawns the privileged
  daemon with `--home /Users/<you>/.dnser`, DNSer derives your home directory
  from that path — version-manager dirs are discovered under *your* home, not
  root's.
- **Machine-specific workarounds are obsolete.** Prefixing commands like
  `command: export PATH="/opt/homebrew/bin:$PATH"; pnpm dev …` still works but
  is no longer necessary.

## Port conflicts / privileged ports

DNSer never fails hard on occupied privileged ports: DNS falls back
`53 → 5353 → 35353`, HTTP `80 → 8080`, HTTPS `443 → 8443`, each with a warning
in `dnser status` and the dashboard header. A linked app's port that another
process grabbed is re-allocated automatically on reload and rewritten in the
config.

If `doctor` reports `port N is free but dnser is not serving on it`, the
daemon needs a restart to claim the preferred port again.

## Resolver not intercepting queries

macOS owns `5353` via mDNSResponder; Linux resolvers vary by desktop.
Run `dnser setup` (or the desktop **Setup System Integration**) once, then
check `dnser doctor → resolver`. Verify with:

```sh
dig @127.0.0.1 myproject.test +short      # should answer through dnser
scutil --dns | head                        # macOS resolver order
```

## Dependencies missing

Detection never installs anything. When `node_modules`, `vendor` etc. are
absent, `deps_missing` surfaces in `dnser ps`/doctor with the exact install
command for the detected stack (`pnpm install`, `go mod download`, …).

## Certificate warnings

Trust happens once during setup: `dnser setup` installs the local CA into the
system/login trust store. If browsers still warn, re-run `dnser setup` or use
the desktop Setup panel; per-domain leaves are re-issued automatically.

## Still stuck

```sh
dnser doctor && dnser ps && dnser logs | tail -50
```

Include that output plus `~/.dnser/dnser.json` (redact secrets) and
`~/.dnser/path-cache.json` in a bug report at
<https://github.com/SDK-E/dnser/issues>.
