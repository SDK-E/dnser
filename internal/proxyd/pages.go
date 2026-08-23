package proxyd

import (
	"fmt"
	"net/http"
	"strings"
)

const pageStyle = `
<style>
  :root { color-scheme: dark; }
  * { margin: 0; box-sizing: border-box; }
  body {
    font-family: ui-sans-serif, -apple-system, "Segoe UI", sans-serif;
    background: #082003; color: #e6f4e2;
    min-height: 100vh; display: flex; align-items: center; justify-content: center;
  }
  .card {
    max-width: 560px; padding: 48px; text-align: center;
  }
  h1 { font-size: 22px; font-weight: 600; letter-spacing: -0.01em; margin-bottom: 12px; }
  .brand { color: #2cdb16; font-weight: 700; }
  p { color: #9db89a; font-size: 15px; line-height: 1.6; margin-bottom: 8px; }
  code {
    background: #0d2f08; border: 1px solid #1c4a14; border-radius: 6px;
    padding: 2px 8px; font-size: 13px; color: #7fe36d;
  }
  .hint { margin-top: 24px; font-size: 13px; color: #6b8a66; }
</style>`

func page(title, body string) string {
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title>%s</head><body><div class="card">%s</div></body></html>`, title, pageStyle, body)
}

func writeLanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	body := fmt.Sprintf(`
<h1><span class="brand">DNS.</span>er is running</h1>
<p>This request reached <code>%s</code>, but no project is linked to this hostname.</p>
<p>Link one with:</p>
<p><code>dnser link --domain=%s --port=3000</code></p>
<p class="hint">Dashboard: <code>https://dnser.test</code></p>`,
		hostOnly(r.Host), hostOnly(r.Host))
	fmt.Fprint(w, page("DNSer", body))
}

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	body := fmt.Sprintf(`
<h1>No project linked</h1>
<p><code>%s</code> is not linked to any local project over HTTPS.</p>
<p>Link it with:</p>
<p><code>dnser link --domain=%s --port=3000</code></p>`,
		hostOnly(r.Host), hostOnly(r.Host))
	fmt.Fprint(w, page("DNSer", body))
}

func writeUpstreamDown(w http.ResponseWriter, r *http.Request, rt Route) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	body := fmt.Sprintf(`
<h1>Dev server not responding</h1>
<p><code>%s</code> is linked to <code>%s</code>, but nothing answered there.</p>
<p>Start your dev server, then reload this page.</p>
<p class="hint">Health status and logs are available in the dashboard.</p>`,
		rt.Host, strings.Join(rt.Backends, ", "))
	fmt.Fprint(w, page("Upstream unavailable", body))
}
