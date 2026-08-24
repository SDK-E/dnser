# Routes

Routes are the forwarding table of the proxy layer. They live in
`projects[].routes` in `~/.dnser/dnser.json` and hot-reload.

## Model

```jsonc
{
  "host": "api",                  // @ | * | label | absolute.host
  "backends": ["localhost:3000"], // 1..n endpoints, round-robin + health failover
  "https": true,                  // serve (and accept) HTTPS for this hostname
  "force_https": false,           // redirect plain HTTP → HTTPS
  "paths": ["/api"],              // HTTP path prefixes (optional)
  "tcp": false,                   // raw TCP forwarding instead of HTTP
  "udp": false,                   // UDP relay instead of HTTP
  "listen": 55432                 // required for tcp/udp: local frontend port
}
```

### Host resolution

| `host` value | Resolves to |
|---|---|
| `@` | the project zone (`myproject.test`) |
| `*` | wildcard: every subdomain (`*.myproject.test`) |
| `api` | single label → `api.myproject.test` |
| `deep.sub` | multi-level relative label → `deep.sub.myproject.test` |
| `other.tld` | absolute when it ends in the settings TLD → `other.tld` |

Lookup order per request: exact host match, then longest matching wildcard
suffix.

## Path-based routing (HTTP/HTTPS)

A route may declare `paths` — URL path prefixes. Among all routes whose host
matches, DNSer picks the entry with the **longest matching prefix**;
a route *without* paths is the fallback for everything else. Prefixes match
on segment boundaries (`/api` matches `/api` and `/api/users`, not `/apix`).

```jsonc
{ "host": "@", "backends": ["localhost:3000"], "https": true },
{ "host": "@", "backends": ["localhost:4000", "localhost:4001"], "paths": ["/api", "/v2"] }
```

- `/api/users` → port 4000/4001 (round-robin)
- `/apix` → port 3000
- `/anything` → port 3000

If a host has only path-scoped routes and no fallback, unmatched paths return
404 rather than guessing.

## Backends

- Any host: loopback, LAN IP, Docker name resolvable by the daemon, external
  hostname — anything valid as `host:port`.
- Multiple backends round-robin; health-gated selection skips backends that
  recently failed probes and fails over automatically.
- `{port}` / `{port:<service>}` placeholders are substituted with the
  project's allocated ports at reconcile time (see
  [Projects](projects.md#command)).

## TLS

- `https: true` — the route is served on the HTTPS listener with an
  auto-issued leaf certificate from the local CA.
- `force_https: true` — plain-HTTP requests to this route answer
  `308 Permanent Redirect` to the same URL over HTTPS.
- Global default: `settings.force_https: true` applies the redirect to **all**
  routes that have `https: true`. Effective state:
  `route.force_https || (settings.force_https && route.https)`.

## TCP & UDP forwarding

For non-HTTP services — databases, message queues, game servers, SMTP:

```jsonc
{ "host": "dbx",   "tcp": true, "listen": 55432, "backends": ["127.0.0.1:5432"] },
{ "host": "sysdns","udp": true, "listen": 15053, "backends": ["127.0.0.1:5353"] }
```

- The frontend binds `127.0.0.1:<listen>`; connect locally on that port.
- TCP pipes raw bytes after a successful dial; failed dials rotate to the next
  backend.
- UDP maintains per-client session affinity to one backend with a 30 s idle
  timeout.
- `listen` ports must be unique across the whole configuration (validated);
  they are also excluded from automatic allocations.
- Combine with service DNS records to get stable names:
  `<service>.<domain>` → connect target.

## Managing routes

```sh
dnser route add myproject.test --host=api --backend=127.0.0.1:{port} \
    --path=/api --https --force-https
dnser route add myproject.test --host=dbx --tcp --listen=55432 \
    --backend=db.internal:5432
dnser route add myproject.test --host=dnsr --udp --listen=15053 \
    --backend=10.0.0.9:53
dnser route list myproject.test
dnser route remove myproject.test --host=dbx
dnser route remove myproject.test --host=@ --path=/v2   # strip one prefix
```

Dashboard: project → **Routes** tab (add, edit fields inline, save).
API: `PUT /api/v1/projects/<domain>` with a full `routes` array.

## Landing & unknown hosts

Requests to hostnames no route claims receive DNSer's landing page on HTTP
and a 404 on HTTPS — never a silent forward upstream.
