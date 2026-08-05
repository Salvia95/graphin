package main

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/provision"
)

// The plugin installer and /graphin:doctor parse this output to decide
// whether a binary is usable, so its shape is a contract
// (docs/plugin-distribution.md §3.2, §6.2).
func TestVersionJSONContract(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := printVersion([]string{"--json"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errOut.String())
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, out.String())
	}
	for _, k := range []string{"version", "commit", "build_date", "os", "arch", "ort", "semantic_supported"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	if got["os"] != runtime.GOOS || got["arch"] != runtime.GOARCH {
		t.Errorf("os/arch = %v/%v, want %s/%s", got["os"], got["arch"], runtime.GOOS, runtime.GOARCH)
	}
	if got["ort"] != provision.ORTVersion {
		t.Errorf("ort = %v, want %s", got["ort"], provision.ORTVersion)
	}
	want := provision.SemanticSupported(runtime.GOOS, runtime.GOARCH)
	if got["semantic_supported"] != want {
		t.Errorf("semantic_supported = %v, want %v", got["semantic_supported"], want)
	}
	if got["version"] == "" {
		t.Error("version is empty")
	}
}

func TestVersionPlainNamesPlatform(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := printVersion(nil, &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errOut.String())
	}
	line := out.String()
	if !strings.HasPrefix(line, "graphin ") {
		t.Errorf("line %q does not start with the binary name", line)
	}
	if !strings.Contains(line, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("line %q does not name the platform", line)
	}
}

// An unknown argument must fail loudly and keep stdout clean — a parser
// reading a stray word as a version string is worse than an error.
func TestVersionRejectsUnknownArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := printVersion([]string{"--josn"}, &out, &errOut); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got %q", out.String())
	}
	if errOut.Len() == 0 {
		t.Error("no diagnostic on stderr")
	}
}

// buildIdentity must never report an empty version: install.sh treats the
// output as the binary's identity.
func TestBuildIdentityNeverEmpty(t *testing.T) {
	orig := version
	defer func() { version = orig }()

	version = ""
	if ver, _ := buildIdentity(); ver == "" {
		t.Fatal("empty version reported")
	}
}
