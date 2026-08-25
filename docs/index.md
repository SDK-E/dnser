# DNS.er documentation

`dnser` gives every project on your machine a real domain (`api.test`),
trusted local HTTPS, and a supervisor that starts, stops and revives your
dev servers. One Go binary orchestrates three proven tools:

- **DNS** — an embedded dnsproxy listener answering your registered
  suffixes and forwarding everything else upstream untouched.
- **Caddy** — TLS with a local CA, reverse proxy to your project ports.
- **process-compose** — runs your processes as *you*, never as root,
  behind TCP readiness probes.

Everything dnser changes on your system is recorded in a mutation journal
(`dnser journal`) and can be undone (`journal revert`, `unelevate`,
`uninstall --purge`).

## Documentation map

| Page | Contents |
|---|---|
| [Getting started](getting-started.md) | Install, first run, the 60-second tour, elevation vs fallback mode |
| [CLI reference](cli-reference.md) | Every command, global flags, output formats, exit codes |
| [Manifest reference](manifest-reference.md) | The full `.dnser.yaml` spec: keys, inference rules, templates, overrides |
| [Architecture](architecture.md) | How the pieces fit together, privilege model, port fallbacks |
| [Troubleshooting](troubleshooting.md) | `doctor`, the mutation journal, common failures, clean uninstall |
| [Migrating from v1](migrating-from-v1.md) | Rewriting legacy manifests to schema v3 |

Ready-to-link example projects live in [`examples/`](../examples):
[`hello-static`](../examples/hello-static),
[`node-api`](../examples/node-api),
[`python-api`](../examples/python-api) and
[`mailbox-style`](../examples/mailbox-style).

Design decisions and engineering RFCs are not part of the user docs; they
live in [`rfc/`](../rfc).
