package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lex.conf")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseBasic(t *testing.T) {
	p := writeTemp(t, `[stable]
Server = https://github.com/mikuripa/lex-pkgs
Include = pkg/x86_64/

[options]
Prefer = binary
Color = True
DetailedLog = True
`)
	cfg, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Name != "stable" {
		t.Fatalf("repos=%+v", cfg.Repos)
	}
	if cfg.Repos[0].Server != "https://github.com/mikuripa/lex-pkgs" {
		t.Fatalf("server=%q", cfg.Repos[0].Server)
	}
	if cfg.Prefer != "binary" || !cfg.Color || !cfg.DetailedLog {
		t.Fatalf("options=%+v", cfg)
	}
	if cfg.Jobs != 4 {
		t.Fatalf("default jobs=%d", cfg.Jobs)
	}
	if cfg.InfoMode != "combined" {
		t.Fatalf("default infomode=%q", cfg.InfoMode)
	}
}

func TestMultipleReposSorted(t *testing.T) {
	p := writeTemp(t, "[testing]\nInclude = pkg/any/\n[stable]\nInclude = pkg/x86_64/\n")
	cfg, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repos) != 2 || cfg.Repos[0].Name != "stable" || cfg.Repos[1].Name != "testing" {
		t.Fatalf("repos not sorted: %+v", cfg.Repos)
	}
}

func TestMissingInclude(t *testing.T) {
	p := writeTemp(t, "[stable]\nServer = https://example.com/r.git\n")
	if _, err := ParseFile(p); err == nil {
		t.Fatal("expected error for missing Include")
	}
}

func TestInvalidPrefer(t *testing.T) {
	p := writeTemp(t, "[options]\nPrefer = tarball\n")
	if _, err := ParseFile(p); err == nil {
		t.Fatal("expected error for bad Prefer")
	}
}

func TestInlineComments(t *testing.T) {
	p := writeTemp(t, "[stable]\nInclude = pkg/x86_64/ # arch recipes\n")
	cfg, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repos[0].Include != "pkg/x86_64/" {
		t.Fatalf("include=%q", cfg.Repos[0].Include)
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := ParseFile(filepath.Join(t.TempDir(), "nope.conf")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestInfoMode(t *testing.T) {
	p := writeTemp(t, "[options]\nInfoMode = installed\n")
	cfg, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InfoMode != "installed" {
		t.Fatalf("infomode=%q", cfg.InfoMode)
	}
	p = writeTemp(t, "[options]\nInfoMode = everything\n")
	if _, err := ParseFile(p); err == nil {
		t.Fatal("expected error for bad InfoMode")
	}
}

func TestRepoBranch(t *testing.T) {
	p := writeTemp(t, "[stable]\nServer = https://example.com/r.git\nBranch = main\nInclude = pkg/x86_64/\n")
	cfg, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repos[0].Branch != "main" {
		t.Fatalf("branch=%q", cfg.Repos[0].Branch)
	}
}
