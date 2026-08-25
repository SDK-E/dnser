# AGENTS.md

Guidance for AI agents working in this repo.

## State
The v1 codebase was deliberately deleted (recoverable from git history,
pre-reset commits). dnser v2 is the clean-slate rebuild and it is complete:
work lands directly on `main`. Do not reintroduce v1 packages or patterns
(`proxyd`, `certs`, `dnscore`, `runner`, `logstream`, bespoke service
renderers — enforced by `TestDeletionDayLedgerFinalLOC`).

## Direction
The architecture of record is [`docs/architecture.md`](docs/architecture.md)
plus the code itself; the invariant regression pack
(`internal/e2e/invariants_test.go`, I1–I7) is the executable spec for
mutation safety. Locked component decisions — Caddy (TLS/proxy), embedded
dnsproxy listener, process-compose supervision, mkcert/caddy-trust stores —
reopen only with new evidence brought via a researched proposal.

## Stack
Go single binary (module `github.com/SDK-E/dnser`) orchestrating external
tools: Caddy (TLS/SNI/CA/proxy), an embedded dnsproxy DNS listener
(`internal/dnsl`), process-compose (supervision), mkcert/caddy-trust
(trust stores).
Go deps: cobra + charmbracelet/fang + huh + lipgloss, go.yaml.in/yaml/v4,
timberjack, cenkalti/backoff/v5, invopop/jsonschema, godotenv.
UI: Mantine v9 + mantine-datatable + TanStack Query (SPA in
`internal/dashboard/webapp`, built with Vite and embedded).
Layout: `internal/{orchestrator,generator,helper,journal,cli,...}`, one
entrypoint `cmd/dnser`. No desktop app.

## Commands
- Build/test/lint gates after every milestone:
  `go build ./... && go test ./... && golangci-lint run`
- Release automation via goreleaser; no repo scripts.

## Conventions
- RESEARCH BEFORE DECISIONS (hard rule): for any dependency, tool, framework, or UI choice, run problem-first online searches (current-year sources), compare at least 2–3 candidates against THIS project's requirements, and record the evidence links in the PR or issue proposing the change before deciding. Never pick from memory, fame, or because the user mentioned a name — a mention triggers research, not selection. "No package exists" claims require demonstrated searches. Decisions must be derived from retrieved facts and the concrete use case — never concluded from model memory; if you haven't searched for a claim, say so and search before answering.
- No comments in code. Explain design decisions here or in README instead.
- Commits: conventional commits (`feat:`, `fix:`, `chore:`…).
- Prefer battle-tested open-source packages over hand-rolled implementations; NIH needs a written justification.
- Errors: wrap with context (`fmt.Errorf("load config: %w", err)`); never panic outside main.
- Config/manifests are user-facing: strict parsing, atomic writes (tmp+rename).
- Domains: arbitrary user-owned FQDNs with suffix semantics
  (declaring `D` answers `D` and everything under `.D`); no fixed global
  TLD (`default_tld` is a zero-config hint only).
- Never log secrets; CA private key path may be logged, contents never.
- Brand: dark surfaces `#082003` family, accent `#2cdb16`; logo asset
  "DNS.er" is canonical — never redraw it (`docs/brand/`).
- Owner/admin identity: `hicham@sdk.enterprises`.

## Product invariants
- Zero-config first run: bare `dnser` must guide to a working setup.
- Unhandled queries MUST forward upstream — breaking normal browsing is a
  release blocker.
- Privileged ports fall back gracefully (53→5353→35353, 80→8080, 443→8443).
- Uninstall purges everything dnser ever touched, verified against the
  mutation journal.
- User project processes NEVER run as root (invariant I5).
