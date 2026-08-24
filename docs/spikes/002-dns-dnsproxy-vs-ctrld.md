# Spike B — DNS listener: dnsproxy vs ctrld

- Status: **Decided** (M0, RFC 005 §1)
- Verdict: **dnsproxy**, embedded as a Go **library** inside the dnser daemon
  (binary mode available as fallback for debugging)
- Loser: ctrld — documented below; reopen only with new evidence
- Date: 2026-08-24 · Platform measured: macOS 15 (darwin/arm64), console user

## 1. Method

Both binaries run headless on distinct loopback ports with cache enabled and a
default public upstream (1.1.1.1). Probed with `dig`: plain forwarding, unknown
names (release-blocker invariant), suffix-rule routing, cached latency. Idle
RSS/CPU sampled with `ps` over ~40 s after warmup.

## 2. Measured results

| Criterion | dnsproxy v0.84.1 | ctrld v1.5.5 | Budget (RFC 001 §11.2) |
|---|---|---|---|
| Idle RSS (2 samples, 15 s apart) | **2016 / 1776 KB** | **4096 / 4304 KB** | ≤ 15 MB — both pass ✅ |
| Idle CPU | **0.0 %** | **0.0 %** | ≈ 0 % — both pass ✅ |
| Forwarding correctness (github.com, amazon.de) | correct A answers | correct A answers | invariant ✅ |
| Unknown/nonexistent names forwarded upstream (not NXDOMAIN'd locally) | ✅ | ✅ | I-invariant ✅ |
| Suffix rule routing (`something.deep.spike.test` → rule upstream) | matched `/spike.test/`, routed to 9.9.9.9; default names still via 1.1.1.1 | same mechanism exists via `listener.policy.rules` (`{"*.x" = ["upstream.N"]}`) | RFC 001 §4 semantics ✅ |
| Cached query latency | 0 ms | 0–1 ms | n/a |

## 3. Decisive differences (retrieved evidence, 2026-08-24)

### 3.1 Library embeddability → deletes an external dependency
dnsproxy is a reusable Go library — AdGuard Home embeds it in production
(https://deepwiki.com/AdguardTeam/AdGuardHome/3.3-dns-cache-system;
https://pkg.go.dev/github.com/AdguardTeam/dnsproxy, v0.84.x Apache-2.0,
released Aug 2026, actively maintained). Embedding it in the dnser daemon
removes one external binary from all four packaging channels (brew formula
deps, deb/rpm Requires, scoop depends, install script), removes one supervised
process from the resource budget table, and puts the DNS listener under the
daemon's own health-gate control (invariant I1 becomes an internal function
call, not cross-process probing).
ctrld publishes Go package types (pkg.go.dev, MIT) but is designed around its
own service/watchdog machinery; embedding is not its intended use.

### 3.2 Passive vs system-owning design
dnser itself owns system DNS integration (`/etc/resolver/*` per RFC 001 §4/§6)
and must never fight the OS. ctrld defaults are built to own the machine:
`dns_watchdog_enabled = true` by default on macOS/Windows ("Watches all
physical interfaces for DNS changes and reverts them to ctrld's settings"),
`leak_on_upstream_failure = true` mutates interfaces on failure, plus mDNS/
ARP/DHCP/PTR discovery listeners. My spike config needed five explicit opt-outs
to make it behave as a passive listener. Its v1.5.5 changelog documents active
battles with mDNSResponder port ownership ("endless watchdog force-reload
loop and dig timeouts") — exactly the failure class RFC 004/I1 forbids.
(https://github.com/Control-D-Inc/ctrld/blob/main/docs/config.md;
https://github.com/Control-D-Inc/ctrld/releases — v1.5.5 notes)
dnsproxy has zero system-integration behavior: pure proxy.

### 3.3 Config-as-generated-file ergonomics (M2 generator)
- dnsproxy: YAML config file (`--config-path`) + flags override. Generator
  emits YAML using go.yaml.in/yaml/v4 — already a locked dependency.
- ctrld: TOML only → generator must add a TOML writer dependency or emit long
  ephemeral-mode flag lists.
Split-DNS syntax maps cleanly to RFC 001 §4 in both:
dnsproxy dnsmasq-style `[/suffix/]upstream` with `#` = default-upstream escape;
ctrld `rules = [{"*.suffix" = ["upstream.N"]}]`.

### 3.4 Local answering parity (risk retired, not differentiating)
Neither tool serves static answers from config (ctrld's `staticdns.go` saves
NIC settings, verified source; dnsproxy has no answer-zone feature). Both need
the tiny miekg/dns answer shim anticipated in RFC 005 §3 risk table, routed to
via `[/registered-suffix/]127.0.0.1:<shim-port>`. This spike confirms the shim
is required either way — it no longer differentiates the candidates.

### 3.5 Protocol/maintenance parity
Both: DoH/DoT/DoQ (+DoH3/dnsproxy; DNSCrypt dnsproxy-only), fallbacks, caches
with TTL overrides, serve-stale/optimistic modes, Aug 2026 releases, healthy
maintenance (AdguardTeam v0.84.1 2026-08-14; Control-D v1.5.5 signed,
2026-08-04). No maintenance argument separates them.

## 4. Consequence (plan of record)

- M3 implements: `internal/dnsl` wrapping `dnsproxy.Proxy` as library inside
  the daemon; answer-shim over `miekg/dns` serving registered suffixes
  (A=127.0.0.1 + manifest `records:`); upstream rules generated per project:
  `[/<root-suffix>/]127.0.0.1:<shim-port>`, everything else → configured
  upstream(s) (plain/DoH/DoT per settings); health probe gate before resolver
  writes (I1); high-port fallback chain 53→5353→35353 unchanged.
- ctrld remains the documented fallback if dnsproxy hits a blocking defect;
  swap surface is isolated behind the M3 listener interface.

## 5. Raw artifacts

Transient spike workspace held configs/logs/timings for this session; numbers
in §2 copied verbatim from those runs.
