# AGENTS.md

Guidance for AI agents working in this repo.

## Stack
Go 1.27 single static binary · Cobra CLI · miekg/dns (authoritative + forwarding) · net/http stdlib reverse proxy · fsnotify hot-reload · embedded React 19 + Vite + Tailwind v4 UI via go:embed · pnpm 11 · golangci-lint v2

## Commands
- Build: `go build ./...`; binary: `go build -o dnser ./cmd/dnser`
- Test: `go test ./...`
- Lint: `golangci-lint run` (config in .golangci.yaml)
- Full verify before finishing any change: `go build ./... && go test ./... && golangci-lint run`
- Desktop verify when touching `internal/desktop` or `cmd/dnser-desktop`: `go build -tags desktop ./... && go vet -tags desktop ./... && golangci-lint run --build-tags desktop`
- UI build: `pnpm --dir web install && pnpm --dir web build` (dist/ is embedded; Go build fails if missing only when building cmd/dnser after touching web/)
- Version injection: `-ldflags "-X github.com/SDK-E/dnser/internal/cli.version=X -X main.version=X"` (release pipeline sets both)
- Packaging recipes: `scripts/package/{icons.go,macos.sh,windows.sh,linux.sh}` + `packaging/`; Linux GUI builds need CGO with `-tags "desktop,gtk3"` (GTK3 + webkit2gtk-4.1, dev packages: libgtk-3-dev libwebkit2gtk-4.1-dev); AppImage assembly requires a native Linux host (AppImage runtime won't exec under qemu)

## Layout rules
- `cmd/dnser` is the only main package, except `cmd/dnser-desktop` (the Wails GUI entrypoint); every desktop/Wails file carries the `desktop` build tag so default `go build ./...` never pulls in GUI deps (Linux needs webkit2gtk). All logic lives under `internal/`.
- `internal/cli` — cobra commands, one file per command group; commands must stay thin, delegating to domain packages.
- `internal/config` — the only place that reads/writes `~/.dnser/dnser.json`. Schema types, defaults, validation, atomic saves, fsnotify watch. Everything else consumes `*config.Store`.
- `internal/api` — REST `/api/v1` + SSE log stream + embedded static UI. Depends on a `Runtime` interface (satisfied by `daemon.Runtime`), never on daemon directly (import cycle). Version string is injected via `daemon.Options.Version`.
- `internal/web` (`web/web.go`) — go:embed of `web/dist`; keep `web/dist/.gitkeep` committed so fresh checkouts compile without a UI build.
- `internal/setup` state file `~/.dnser/setup-state.json` records exactly what setup changed; `unsetup` restores from it only.
- Port fallbacks when privileged ports are taken: DNS 53→5353→35353 (macOS owns 5353 via mDNSResponder), HTTP 80→8080, HTTPS 443→8443 — never fail hard on occupied privileged ports.
- `internal/dnscore` — DNS engine: zone matching (exact, wildcard), record rendering, upstream forwarding with cache. Imports miekg/dns; no HTTP code here.
- `internal/proxyd` — reverse proxy + SNI routing; `internal/certs` — CA + leaf issuance; both consume config.Store snapshots.
- `internal/setup`, `internal/service` — OS-specific files carry build tags: `darwin.go`, `linux.go`, `windows.go`. Never put syscall/OS-specific calls in shared files.
- `internal/logstream` — ring buffer + broadcast hub for query events; `internal/daemon` wires DNS, proxy, health checks and the API server into one process with fsnotify-driven hot reload.
- Tests live next to code as `*_test.go`; integration tests bind high ports (>30000) never privileged ports.
- `internal/e2e` — black-box end-to-end suite: builds the real binary, spawns it on free high ports with a sandboxed DNSER_HOME and a fake upstream resolver, exercises DNS wire queries, TLS proxy, API/SSE, CLI and hot reload. Runs on every OS in CI (`go test ./internal/e2e/`).

## Conventions
- No comments in code. Explain design decisions here or in README instead.
- Commits: conventional commits (`feat:`, `fix:`, `chore:`…).
- Errors: wrap with context (`fmt.Errorf("load config: %w", err)`); never panic outside main.
- Config file is user-facing: keep JSON field order stable, always write atomically (tmp + rename), preserve unknown-version errors loudly.
- Domains are normalized lowercase, no trailing dot, stored FQDN-style (`myproject.test`). Record names inside a project are relative labels (`@`, `api`, `*`).
- Never log secrets; the CA private key path may be logged, its contents never.
- Brand: dark surfaces from parent brand (`#082003` family) with green `#2cdb16` accents; canonical wordmark assets copied into `web/public/brand/` — never re-draw the mark.
- Owner/admin identity: `hicham@sdk.enterprises`.

## Product invariants
- Zero-config first run: bare `dnser` must guide to a working setup; every default overridable in config/settings.
- Unhandled queries MUST forward upstream — breaking normal browsing is a release blocker.
- Privileged ports (53/80/443) fall back gracefully (5353/8080/8443) with clear messaging when unavailable.
- `dnser unsetup` reverts exactly what `dnser setup` changed, nothing more.
