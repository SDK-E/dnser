# Security Policy

## Supported versions

| Version | Supported |
|---|---|
| latest release | yes |
| older releases | no — upgrade first |

dnser is a local development tool. It binds services to loopback, never
runs user processes as root, and performs privileged mutations only when
you explicitly run `dnser elevate`.

## Reporting a vulnerability

**Do not open a public GitHub issue for security reports.**

Use GitHub's private vulnerability reporting on this repository
(**Security → Report a vulnerability**), or email **hicham@sdk.enterprises**
if you prefer. Include:

- affected version (`dnser --version`) and install method (brew/deb/rpm/source)
- a minimal reproduction and observed vs expected behavior
- your assessment of impact

You will get an acknowledgment within 72 hours. We will coordinate a fix
and disclosure timeline with you and credit reporters in the release notes
unless you prefer to stay anonymous.

## Scope notes

Especially valuable report topics for a tool like dnser:

- DNS suffix handling: names that escape the local answer set or shadow
  real internet domains (public-suffix registration is warned but any
  bypass of the warning path matters)
- the privilege boundary: anything that lets project manifests or
  generated configs execute as root or escalate
- CA trust handling: private key exposure paths beyond `~/.dnser`
- the dashboard: anything weakening loopback-only binding or token gating
- secret redaction: env values leaking into `explain`, logs, journal or
  JSON output despite redaction
- uninstall completeness: residue contradicting the mutation-journal
  verification

## Known non-issues

- Local plaintext HTTP on fallback ports (8080/8443) in fallback mode.
- Self-signed local CA warnings by design; trust is installed only via
  explicit elevation.
