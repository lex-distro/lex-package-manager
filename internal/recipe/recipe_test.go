package recipe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateGood(t *testing.T) {
	r := &Recipe{Kind: "recipe", Name: "fastfetch", Version: "2.68.1",
		Binary: &BinaryStanza{URL: "https://x/y.tar.gz", SHA256: "abc"}}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateTraversal(t *testing.T) {
	for _, name := range []string{"", "../../etc/x", "a/b", "x y", "foo!"} {
		r := &Recipe{Name: name, Version: "1.0",
			Binary: &BinaryStanza{URL: "u", SHA256: "s"}}
		if err := r.Validate(); err == nil {
			t.Fatalf("name %q accepted", name)
		}
	}
}

func TestValidateNeedsStanza(t *testing.T) {
	r := &Recipe{Name: "x", Version: "1.0"}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error with no stanzas")
	}
	r.Binary = &BinaryStanza{URL: "", SHA256: ""}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for binary without url+sha")
	}
}

func TestSelectFallback(t *testing.T) {
	onlyBin := &Recipe{Name: "a", Version: "1",
		Binary: &BinaryStanza{URL: "u", SHA256: "s"}}
	if m, _ := onlyBin.SelectMethod("binary"); m != "binary" {
		t.Fatalf("got %q", m)
	}
	if m, _ := onlyBin.SelectMethod("source"); m != "binary" {
		t.Fatalf("fallback got %q", m)
	}
	onlySrc := &Recipe{Name: "b", Version: "1",
		Source: &SourceStanza{URL: "u", SHA256: "s"}}
	if m, _ := onlySrc.SelectMethod("source"); m != "source" {
		t.Fatalf("got %q", m)
	}
	if m, _ := onlySrc.SelectMethod("binary"); m != "source" {
		t.Fatalf("fallback got %q", m)
	}
}

func TestSupportsArch(t *testing.T) {
	any := &Recipe{Name: "a", Version: "1"}
	if !any.SupportsArch("x86_64") {
		t.Fatal("empty arch should match")
	}
	x := &Recipe{Name: "b", Version: "1", Arch: []string{"x86_64"}}
	if !x.SupportsArch("x86_64") || x.SupportsArch("aarch64") {
		t.Fatal("arch match wrong")
	}
	// Go naming vs LFS naming.
	if !x.SupportsArch("amd64") {
		t.Fatal("amd64 should match x86_64 recipe")
	}
	a := &Recipe{Name: "c", Version: "1", Arch: []string{"aarch64"}}
	if !a.SupportsArch("arm64") {
		t.Fatal("arm64 should match aarch64 recipe")
	}
}

func TestLoadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lexbuild")
	data := `{"kind":"recipe","name":"x","version":"1.0","binary":{"url":"u","sha256":"s"}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r.Name != "x" {
		t.Fatalf("name=%q", r.Name)
	}
	if _, err := Load(p + ".missing"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
