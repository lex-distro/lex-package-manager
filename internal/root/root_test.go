package root

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEX_ROOT", dir)
	got, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(dir)
	// /tmp may be a symlink (macOS)
	if got != want {
		if resolved, err := filepath.EvalSymlinks(want); err == nil && got == resolved {
			return
		}
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestJoin(t *testing.T) {
	if got := Join("/r", "db", "x.lexdata"); got != filepath.Join("/r", "db", "x.lexdata") {
		t.Fatalf("got %q", got)
	}
	_ = os.Getenv("LEX_ROOT")
}
