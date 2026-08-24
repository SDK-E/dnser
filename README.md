# dnser

Local development orchestrator: one command puts any project folder on its
own HTTPS domain, with DNS, TLS certificates, and dev-server supervision
handled by best-in-class tools under the hood.

**Status:** v2 rebuild in progress. The previous implementation was removed;
see `docs/rfc/` for the architecture and `docs/rfc/005-implementation-plan.md`
for the active execution plan (milestones M0–M10).

## Documentation

| Doc | Contents |
|---|---|
| [docs/rfc/001](docs/rfc/001-orchestrator-architecture.md) | Architecture, locked component decisions, resource budget |
| [docs/rfc/002](docs/rfc/002-project-manifest.md) | `.dnser.yaml` manifest spec (minimal by default, total by choice) |
| [docs/rfc/003](docs/rfc/003-command-flows.md) | Command surface, exit codes, confirmation protocol |
| [docs/rfc/004](docs/rfc/004-failure-containment.md) | Failure semantics, machine invariants |
| [docs/rfc/005](docs/rfc/005-implementation-plan.md) | Implementation milestones |

## Install

Not yet — v2 has no release. Planned: `brew install sdk-e/tap/dnser`
(macOS), install script / deb+rpm (Linux), scoop (Windows). See RFC 001 §7.

## License

See [LICENSE](LICENSE).
