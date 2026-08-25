# Migration guide: legacy manifests → schema v3

`dnser migrate [path]` rewrites old manifest keys to schema v3.

## What changes

| Legacy key | Rewrite |
|---|---|
| `label: myapp` | `domain: myapp.test` (FQN form; suffix configurable via manifest `domain`) |
| `tld: dev` | removed — global TLDs are a zero-config hint only; declare explicit `domain:` instead |
| `name: myapp` | removed — project name comes from the directory |

## Safety

1. Dry-run diff by default: shows every rewrite before touching anything.
2. Moderate confirmation: `-y` applies without prompting (agents: expect
   exit 3 with a remediation command on non-TTY).
3. Original preserved as `.dnser.yaml.v2.bak` next to the manifest.
4. The rewrite is journaled like every mutation — revertable while the
   journal retains it.

## After migrating

```sh
dnser link .
dnser up
```

If validation fails after migration (e.g. a label contained characters
invalid in an FQN), fix the reported line numbers in the rewritten file —
the backup remains untouched.
