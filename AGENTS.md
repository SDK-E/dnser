# AGENTS.md

Guidance for AI agents working in this repo.

## State
The v1 codebase was deliberately deleted (recoverable from git history,
pre-reset commits). This is a **clean-slate rebuild of dnser v2**. Do not
reintroduce v1 packages or patterns; implement exactly per the RFCs below.

## Direction
Read, in order, before any code:
1. `docs/rfc/001-orchestrator-architecture.md` — architecture, locked
   component decisions (§5, §13), resource budget (§11), simplification ledger
2. `docs/rfc/002-project-manifest.md` — `.dnser.yaml` v3 spec
3. `docs/rfc/003-command-flows.md` — commands, exit codes, confirmation protocol
4. `docs/rfc/004-failure-containment.md` — mutation wrapper, invariants I1–I7
5. `docs/rfc/005-implementation-plan.md` — milestones M0–M10 you execute

Decisions in RFC 001 §5/§13 are locked; reopen only with new evidence.
Open spikes (tenement vs process-compose, ctrld vs dnsproxy) are part of M0.

## Stack (v2 target)
Go single binary (module `github.com/SDK-E/dnser`) orchestrating external
tools: Caddy (TLS/SNI/CA/proxy), ctrld-or-dnsproxy (DNS), process-compose
or tenement (supervision), mkcert/caddy-trust (trust stores).
Go deps: cobra + charmbracelet/fang + huh + lipgloss, go.yaml.in/yaml/v4,
timberjack, cenkalti/backoff/v5, invopop/jsonschema, godotenv.
UI: Mantine v9 + mantine-datatable + TanStack Query, embedded later.
Layout grows per RFC 005 M0: `internal/{orchestrator,generator,helper,
journal,cli,...}`, one entrypoint `cmd/dnser`. No desktop app.

## Commands
- Build/test/lint gates after every milestone:
  `go build ./... && go test ./... && golangci-lint run`
- Release automation via goreleaser (RFC 001 §13.4); no repo scripts.

## Conventions
- RESEARCH BEFORE DECISIONS (hard rule): for any dependency, tool, framework, or UI choice, run problem-first online searches (current-year sources), compare at least 2–3 candidates against THIS project's requirements, and record the evidence links in the relevant RFC before deciding. Never pick from memory, fame, or because the user mentioned a name — a mention triggers research, not selection. "No package exists" claims require demonstrated searches. Decisions must be derived from retrieved facts and the concrete use case — never concluded from model memory; if you haven't searched for a claim, say so and search before answering.
- No comments in code. Explain design decisions here or in README instead.
- Commits: conventional commits (`feat:`, `fix:`, `chore:`…).
- Prefer battle-tested open-source packages over hand-rolled implementations; NIH needs a written justification.
- Errors: wrap with context (`fmt.Errorf("load config: %w", err)`); never panic outside main.
- Config/manifests are user-facing: strict parsing, atomic writes (tmp+rename).
- Domains: arbitrary user-owned FQDNs with suffix semantics (RFC 001 §4);
  no fixed global TLD (`default_tld` is a zero-config hint only).
- Never log secrets; CA private key path may be logged, contents never.
- Brand: dark surfaces `#082003` family, accent `#2cdb16`; logo asset
  "DNS.er" is canonical — never redraw it (assets must be re-added from
  the owner's brand source before UI work; not present in this repo).
- Owner/admin identity: `hicham@sdk.enterprises`.

## Product invariants
- Zero-config first run: bare `dnser` must guide to a working setup.
- Unhandled queries MUST forward upstream — breaking normal browsing is a
  release blocker.
- Privileged ports fall back gracefully (53→5353→35353, 80→8080, 443→8443).
- Uninstall purges everything dnser ever touched, verified against the
  mutation journal (RFC 003 §3.12, RFC 004).
- User project processes NEVER run as root (invariant I5).
