<!--
  Keep the "Why" about the problem, not the diff — the diff speaks for itself.
-->

## Why

<!-- What problem does this solve? Link the issue: Fixes #123 -->

## What

<!-- The change in a few sentences. For CLI/user-visible changes: which docs pages were updated? -->

## Gates

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes
- [ ] Schema regenerated if config structs changed (`go run ./cmd/dnser schema > internal/config/schema/dnser.manifest.schema.json`)
- [ ] Docs updated for user-visible changes (`docs/`)
- [ ] Conventional commits used (`feat:`, `fix:`, `chore:`…)

## Risk containment

<!-- If this touches mutations (elevate/link/uninstall/generated configs):
     which invariants (I1–I7, see docs/architecture.md) could it affect,
     and what test covers them? -->
