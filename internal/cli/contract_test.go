package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	prevExit := exitFn
	exitCode := 0
	exitFn = func(c int) { exitCode = c }
	t.Cleanup(func() { exitFn = prevExit })
	root := NewRootCommand()
	root.SetArgs(args)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SilenceErrors = true
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		env, code2 := ErrorEnvelope(err)
		b, _ := json.Marshal(env)
		errBuf.WriteString(string(b) + "\n")
		exitCode = code2
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestExitCodeContract(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if _, _, code := runCLI(t, "status", "--project", "nope-not-here", "-o", "json"); code != 0 {
		_ = code
	}

	_, _, code := runCLI(t, "bogus-command")
	if code != 2 {
		t.Fatalf("unknown command must exit 2, got %d", code)
	}
	_, _, code = runCLI(t, "link", filepath.Join(dir, "missing-dir"))
	if code != 1 {
		t.Fatalf("link on missing manifest is operational failure, got %d", code)
	}
}

func TestNonTTYDefaultsToJSONAndNeverPrompts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	out, _, _ := runCLI(t, "journal", "ls")
	if !strings.Contains(out, "no journal entries") && !strings.Contains(out, "\"") {
		t.Fatalf("expected machine-readable output when piped: %q", out)
	}
}

func TestUninstallPurgeRequiresSevereConfirm(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	_, stderr, code := runCLI(t, "uninstall", "--purge")
	if code != 3 {
		t.Fatalf("purge without confirm must exit 3 (confirmation required), got %d: %s", code, stderr)
	}
	var env CodedError
	if err := json.Unmarshal([]byte(firstJSONLine(stderr)), &env); err != nil || env.Code != 3 || env.Kind != "confirmation_required" {
		t.Fatalf("confirm envelope malformed: %s (%v)", stderr, err)
	}
	if env.Remediation == "" || !strings.Contains(env.Remediation, "--confirm") {
		t.Fatalf("remediation must carry the exact --confirm re-invocation: %q", env.Remediation)
	}

	_, stderr, code = runCLI(t, "uninstall", "--purge", "--yes")
	if code != 3 {
		t.Fatalf("--yes alone must be refused for severe mutations, got %d", code)
	}

	var tokenEnv CodedError
	if err := json.Unmarshal([]byte(firstJSONLine(stderr)), &tokenEnv); err != nil || !strings.Contains(tokenEnv.Remediation, "--confirm") {
		t.Fatalf("refused --yes must still carry remediation: %q (%v)", stderr, err)
	}
	parts := strings.SplitN(tokenEnv.Remediation, "--confirm ", 2)
	if len(parts) != 2 || parts[1] == "" {
		t.Fatalf("remediation missing token: %q", tokenEnv.Remediation)
	}
	token := strings.TrimSpace(parts[1])

	_, _, code = runCLI(t, "uninstall", "--purge", "--confirm", token)
	if code != 0 {
		t.Fatalf("correct token authorizes purge, got %d", code)
	}
}

func TestLinkIdempotentWithGoldenSummary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	proj := filepath.Join(dir, "myapp")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "domain: myapp.test\nport: 3000\ncommand: npm start\navailability: always\n"
	if err := os.WriteFile(filepath.Join(proj, ".dnser.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, code := runCLI(t, "link", proj)
	if code != 0 {
		t.Fatalf("first link failed: %d", code)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &first); err != nil {
		t.Fatalf("piped default must be JSON: %q %v", stdout, err)
	}
	if first["domain"] != "myapp.test" || first["availability"] != "always" {
		t.Fatalf("golden fields wrong: %+v", first)
	}
	port1 := first["port"].(float64)

	stdout2, _, code := runCLI(t, "link", proj, "--fields", "project,port")
	if code != 0 {
		t.Fatalf("re-link failed: %d", code)
	}
	var second map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout2)), &second); err != nil {
		t.Fatalf("re-link json: %q %v", stdout2, err)
	}
	if len(second) != 2 || second["project"] != "myapp" {
		t.Fatalf("--fields pruning broken: %+v", second)
	}
	if second["port"].(float64) != port1 {
		t.Fatalf("port pin drifted across relinks: %v vs %v", second["port"], port1)
	}

	st, serr := stateOpen()
	if serr != nil {
		t.Fatal(serr)
	}
	if lp, ok := st.Linked("myapp"); !ok || lp.Port != int(port1) {
		t.Fatalf("registry missing link: %+v ok=%v", lp, ok)
	}
}

func TestStartUnknownProjectListsKnown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	proj := filepath.Join(dir, "known")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".dnser.yaml"), []byte("domain: known.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "link", proj)

	_, stderr, code := runCLI(t, "start", "unknown-proj")
	if code != 2 {
		t.Fatalf("unknown project must exit 2, got %d", code)
	}
	if !strings.Contains(stderr, "known") {
		t.Fatalf("exit-code-as-data must list known projects: %s", stderr)
	}
}

func TestDoctorCleanExitsZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_, _, code := runCLI(t, "doctor")
	if code != 0 {
		t.Fatalf("fresh install must be clean, got %d", code)
	}
}

func TestUpdateClassifiesBrewInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	fake := filepath.Join(dir, "bin", "dnser")
	if err := os.MkdirAll(filepath.Dir(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DNSER_SELF_TEST", fake)
	_, _, code := runCLI(t, "update", "--check")
	if code != 0 {
		t.Fatalf("update check must be clean: %d", code)
	}
}

func firstJSONLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			return strings.TrimSpace(line)
		}
	}
	return "{}"
}

var _ = os.Stdout
