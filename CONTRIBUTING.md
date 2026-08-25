# Contributing to dnser

Thanks for helping build dnser. This document covers how to set up, work,
and submit changes.

## Project direction

dnser v2 is a clean-slate rebuild: one Go binary orchestrating proven tools
(Caddy, dnsproxy, process-compose, mkcert) instead of reimplementing them.
The architecture and locked decisions live in [`rfc/`](rfc/) — read them,
in order, before proposing changes:

1. [`rfc/001-orchestrator-architecture.md`](rfc/001-orchestrator-architecture.md)
2. [`rfc/002-project-manifest.md`](rfc/002-project-manifest.md)
3. [`rfc/003-command-flows.md`](rfc/003-command-flows.md)
4. [`rfc/004-failure-containment.md`](rfc/004-failure-containment.md)
5. [`rfc/005-implementation-plan.md`](rfc/005-implementation-plan.md)

Decisions marked locked (RFC 001 §5/§13) reopen only with new evidence.

## Product invariants (non-negotiable)

- Zero-config first run: bare `dnser` guides to a working setup.
- Unhandled DNS queries MUST forward upstream — breaking normal browsing
  is a release blocker.
- Privileged ports fall back gracefully (53→5353→35353, 80→8080, 443→8443).
- User project processes NEVER run as root.
- Uninstall purges everything dnser ever touched, verified against the
  mutation journal.

## Development setup

```bash
git clone https://github.com/SDK-E/dnser && cd dnser
go build ./...
go test ./...
```

External tools used by integration tests (Caddy, process-compose) are
looked up on PATH; unit tests run without them.

## Gates

Every change must pass before submission:

```bash
go build ./... && go test ./... && golangci-lint run
```

CI additionally diffs the generated JSON Schema:

```bash
go run ./cmd/dnser schema > internal/config/schema/dnser.manifest.schema.json
```

If you touched config structs, regenerate and commit the schema.

## Conventions

- **Conventional commits**: `feat:`, `fix:`, `chore:`, `docs:`, `test:`…
- **No comments in code.** Explain design decisions in the PR description
  or README instead.
- **Errors**: wrap with context (`fmt.Errorf("load config: %w", err)`),
  never panic outside main.
- **Prefer battle-tested open-source packages** over hand-rolled
  implementations; NIH needs written justification.
- **Research before decisions** (hard rule): dependency/tool choices come
  from problem-first searches compared against *this* project's
  requirements, with evidence links recorded in the relevant RFC — never
  from memory or name recognition.
- **Config files are user-facing**: strict parsing, atomic writes
  (tmp+rename).
- Never log secrets; CA private key path may be logged, contents never.
- Docs for users live in [`docs/`](docs/); engineering RFCs in [`rfc/`](rfc/).

## Submitting changes

1. Open an issue first for anything architectural or user-visible.
2. Branch, make the change, keep commits conventional.
3. Run the gates (above).
4. Open a PR using the template; describe the *why*, not just the *what*.
5. For CLI-facing changes: update the relevant page in
   [`docs/cli-reference.md`](docs/cli-reference.md) and golden output tests.

## Reporting bugs and security issues

Bugs: [open an issue](https://github.com/SDK-E/dnser/issues/new/choose).
Security vulnerabilities: **do not open a public issue** — see
[`SECURITY.md`](SECURITY.md).
