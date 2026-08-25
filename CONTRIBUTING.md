# Contributing to dnser

Thanks for helping build dnser — local domains, trusted local HTTPS, and
supervised dev servers, orchestrated from one Go binary.

## Project direction

dnser is a thin orchestrator of proven tools (Caddy, dnsproxy,
process-compose, mkcert); we write glue, not infrastructure. The v2
cut-over is complete and lives on `main`; the architecture remains
specified by the RFCs — read them, in order, before proposing changes:

1. [`rfc/001-orchestrator-architecture.md`](rfc/001-orchestrator-architecture.md)
2. [`rfc/002-project-manifest.md`](rfc/002-project-manifest.md)
3. [`rfc/003-command-flows.md`](rfc/003-command-flows.md)
4. [`rfc/004-failure-containment.md`](rfc/004-failure-containment.md)
5. [`rfc/005-implementation-plan.md`](rfc/005-implementation-plan.md)

Decisions marked locked (RFC 001 §5/§13) reopen only with new evidence.
Current state and measured budgets are recorded in
[`rfc/ledger.md`](rfc/ledger.md).

## Product invariants (non-negotiable)

- Zero-config first run: bare `dnser` guides to a working setup.
- Unhandled DNS queries MUST forward upstream — breaking normal browsing
  is a release blocker.
- Privileged ports fall back gracefully (53→5353→35353, 80→8080, 443→8443).
- User project processes NEVER run as root.
- Uninstall purges everything dnser ever touched, verified against the
  mutation journal.

## Repository layout

```
cmd/dnser                 entrypoint (the only main package)
internal/cli              command surface, output contract, locks
internal/config           manifest decode/resolve, templates, JSON Schema
internal/dnsl             embedded DNS listener (dnsproxy)
internal/generator        manifest → Caddy/supervisor configs
internal/orchestrator     tool supervision and lifecycle
internal/helper           privileged plan executor + service definitions
internal/journal          mutation journal (plan/capture/apply/verify)
internal/state            runtime state
internal/dashboard        loopback web UI server + embedded SPA
internal/dashboard/webapp Vite + React 19 + Mantine 9 + TanStack Query SPA
internal/e2e              real-binary flows and invariant regression pack
packaging/, .goreleaser.yaml, .github/workflows   release automation
docs/                     user documentation      rfc/   engineering specs
```

## Development setup

Go per [`go.mod`](go.mod) (1.27+) and golangci-lint v2. There are no repo
scripts — everything is plain commands.

```bash
git clone https://github.com/SDK-E/dnser && cd dnser
go build ./...
go test ./...
```

Tests are self-contained: e2e flows run the real binary in a sandboxed
`$HOME` on high ports with fake upstreams. No Caddy or process-compose on
PATH is required to develop or test. The perf-budget test skips under
`go test -short ./...`.

Dashboard work additionally needs Node/npm:

```bash
cd internal/dashboard/webapp
npm ci
npm run dev        # vite dev server
npm run build      # tsc -b && vite build → webapp/dist (embedded via go:embed)
```

Rebuild the SPA before `go build` when you changed it, or the binary ships
a stale dashboard.

## Gates

Every change must pass before submission:

```bash
go build ./... && go test ./... && golangci-lint run
```

CI also diffs the generated JSON Schema against the committed file. If you
touched config structs, regenerate and commit it:

```bash
go run ./cmd/dnser schema > internal/config/schema/dnser.manifest.schema.json
```

Invariant regressions map 1:1 to the RFC 004 table via
`internal/e2e/invariants_test.go` — if you touch mutations (link/elevate/
uninstall/generated configs), extend the matching invariant test.

## Conventions

- **Conventional commits**: `feat:`, `fix:`, `chore:`, `docs:`, `test:`…
- **No comments in code.** Explain design decisions in the PR description
  instead.
- **Errors**: wrap with context (`fmt.Errorf("load config: %w", err)`),
  never panic outside main.
- **Prefer battle-tested open-source packages** over hand-rolled code;
  NIH needs written justification.
- **Research before decisions** (hard rule): dependency/tool choices come
  from problem-first searches compared against *this* project's
  requirements, with evidence links recorded in the relevant RFC — never
  from memory or name recognition.
- **Config files are user-facing**: strict parsing (unknown keys error),
  atomic writes (tmp+rename).
- **Never log secrets**; the CA private key path may be logged, its
  contents never. Env values stay redacted in output unless explicitly
  overridden.
- **Brand**: dark surfaces `#082003` family, accent `#2cdb16`. The DNS.er
  wordmark in `docs/brand/` is canonical — never redraw it.
- Docs for users live in [`docs/`](docs/); engineering specs in
  [`rfc/`](rfc/).

## Submitting changes

1. Open an issue first for anything architectural or user-visible
   (usage questions go to Discussions).
2. Branch, make the change, keep commits conventional.
3. Run the gates (above).
4. Open a PR using the template; describe the *why*, not just the *what*.
5. For CLI-facing changes update the relevant pages in
   [`docs/cli-reference.md`](docs/cli-reference.md) and golden output tests;
   for manifest-facing changes update
   [`docs/manifest-reference.md`](docs/manifest-reference.md).

## Releases

Maintainers tag `vX.Y.Z`; goreleaser builds, publishes checksums, GitHub
Release, the Homebrew formula/tap and deb/rpm packages automatically (see
`.goreleaser.yaml`).

## Reporting bugs and security issues

Bugs: [open an issue](https://github.com/SDK-E/dnser/issues/new/choose).
Security vulnerabilities: **do not open a public issue** — see
[`SECURITY.md`](SECURITY.md).
