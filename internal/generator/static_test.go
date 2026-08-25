package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticProjectServesFilesWithoutProxy(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public", "index.html"), []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := mustDecode(t, "type: static\ndomain: hello.test\nhttps:\n  hello.test: true\n")
	out, err := Generate(Input{Project: "hello-static", Root: "/tmp/x", Dir: dir, LogsDir: t.TempDir(), Manifest: m})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	s := string(out.Caddyfile)
	for _, want := range []string{"root * " + filepath.Join(dir, "public"), "file_server", "tls internal"} {
		if !strings.Contains(s, want) {
			t.Fatalf("static site missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "reverse_proxy") {
		t.Fatalf("static site must not reverse_proxy to a phantom port:\n%s", s)
	}
	if out.Supervisor != nil {
		t.Fatalf("static project must not render a supervisor config")
	}

	m2 := mustDecode(t, "type: static\ndomain: hello.test\n")
	out2, err := Generate(Input{Project: "flat", Root: "/tmp/x", Dir: dir, LogsDir: t.TempDir(), Manifest: m2})
	if err != nil {
		t.Fatalf("generate flat: %v", err)
	}
	if !strings.Contains(string(out2.Caddyfile), "root * "+dir) {
		t.Fatalf("without public/, root must be the project dir:\n%s", out2.Caddyfile)
	}
}

func TestHostRouteMergeDoesNotDuplicateTlsOrErrorHandling(t *testing.T) {
	m := mustDecode(t, "domain: inbox.test\nport: 11090\nroutes:\n  - host: \"@\"\n    https: true\n    backends:\n      - \"127.0.0.1:11090\"\n      - \"127.0.0.1:11091\"\n")
	out, err := Generate(Input{Project: "mbx", Root: "/tmp/x", Dir: t.TempDir(), LogsDir: t.TempDir(), Manifest: m, Port: 11090})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	s := string(out.Caddyfile)
	if n := strings.Count(s, "tls internal"); n != 1 {
		t.Fatalf("want exactly one tls internal, got %d:\n%s", n, s)
	}
	if n := strings.Count(s, "handle_errors {"); n != 1 {
		t.Fatalf("want exactly one handle_errors, got %d:\n%s", n, s)
	}
	if !strings.Contains(s, "reverse_proxy 127.0.0.1:11090 127.0.0.1:11091") {
		t.Fatalf("both backends must render:\n%s", s)
	}
}

func TestAnswerTableExpandsBareRecordLabels(t *testing.T) {
	m := mustDecode(t, "domain: inbox.test\nrecords:\n  - {name: www, type: CNAME, value: inbox.test}\n  - {name: \"@\", type: A, value: 127.0.0.1}\n")
	ans, err := answerTable(m, m.EffectiveNames(), m.PrimaryDomain("mbx"))
	if err != nil {
		t.Fatalf("answerTable: %v", err)
	}
	got := map[string]string{}
	for _, a := range ans {
		got[a.Name+"|"+a.Type] = a.Value
	}
	if got["www.inbox.test|CNAME"] != "inbox.test" {
		t.Fatalf("www must expand to www.inbox.test, got %v", got)
	}
	if _, ok := got["inbox.test|A"]; !ok {
		t.Fatalf("@ must expand to primary, got %v", got)
	}
}
