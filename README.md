# dnser

Local domains, TLS and dev-server orchestration for macOS and Linux.
One binary orchestrates Caddy (TLS/SNI/proxy), an embedded DNS listener,
and process-compose — driven by a single `.dnser.yaml` per project.

## Quickstart

```sh
brew install sdk-e/tap/dnser
cd ~/code/my-app
dnser init            # scaffold .dnser.yaml (detects stack)
dnser link .          # register + generate infra (validates, pins port)
dnser elevate         # one-shot admin: /etc/resolver entries
dnser up              # start; https://myapp.test answers
```

Without elevation dnser runs in **fallback mode**: projects stay reachable
at `http://localhost:<port>` and normal browsing is never touched.

## Commands that matter

| Command | Purpose |
|---|---|
| `init` | Scaffold a validated `.dnser.yaml` |
| `link <dir>` | Register project, pin ports, generate Caddy/supervisor/DNS artifacts |
| `up` / `down` | Start/stop everything (`down --infra` also stops DNS/Caddy) |
| `status` | Actual ports, phases, resolver health |
| `doctor` | Health checks; exit 10 = issues found; `--fix` safe subset |
| `elevate` / `unelevate` | Apply/reverse system changes transactionally |
| `journal ls\|show\|finish\|revert` | Inspect/resume/cancel mutation plans |
| `uninstall --purge --confirm <name>` | Erase every trace (severe) |
| `dashboard` | Loopback-only web UI with token URL |

Every command is agent-friendly: `-o json|ndjson`, `--fields`,
exit-code contract (0/1/2/3/4/10), and confirmation envelopes that print
the exact re-run command.

## Install

- **macOS/Linux:** `brew install sdk-e/tap/dnser`
  (pulls in caddy, process-compose, mkcert)
- **Debian/Ubuntu:** download the `.deb` from Releases
  (`apt install ./dnser_*.deb`; depends on caddy/process-compose/mkcert)
- **Fedora/RHEL:** download the `.rpm` from Releases
- **Windows:** scoop bucket entry under `packaging/scoop/`
  (copy into your bucket); requires `scoop install caddy process-compose mkcert`
- **Any:** grab `checksums.txt` + archive from GitHub Releases;
  `dnser update --check` handles upgrades later.

## Migrating from v2 manifests

```sh
dnser migrate path/to/project        # dry-run diff of legacy keys
dnser migrate path/to/project -y     # apply (backup kept alongside)
```

Rewrites performed: `label:` → FQN domain, removal of `tld:`/`name:`.
Original saved as `.dnser.yaml.v2.bak`; the change is journaled.

## Development

```sh
go build ./... && go test ./... && golangci-lint run
```

Releases are cut by tagging: `git tag v0.1.0 && git push origin v0.1.0`
(GitHub Actions runs goreleaser: binaries, checksums, brew tap, deb/rpm).

See `docs/rfc/` for architecture and milestone plans.
