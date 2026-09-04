package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	root := t.TempDir()
	ins := &Installed{Name: "fastfetch", Version: "2.68.1", Method: "binary",
		Files: []string{"/usr/bin/fastfetch"}}
	if err := Save(root, ins); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(root, "fastfetch")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Kind != "installed" || got.Files[0] != "/usr/bin/fastfetch" {
		t.Fatalf("roundtrip=%+v", got)
	}
	if _, err := os.Stat(Path(root, "fastfetch")); err != nil {
		t.Fatalf("stat: %v", err)
	}
	// No temp files left behind.
	entries, _ := os.ReadDir(filepath.Join(root, "db"))
	for _, e := range entries {
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			t.Fatalf("leftover temp file %q", e.Name())
		}
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(t.TempDir(), "nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	if names, err := List(root); err != nil || len(names) != 0 {
		t.Fatalf("empty db: %v %v", names, err)
	}
	Save(root, &Installed{Name: "b-pkg", Version: "1", Method: "source"})
	Save(root, &Installed{Name: "a-pkg", Version: "2", Method: "binary"})
	// Non-lexdata files ignored.
	os.WriteFile(filepath.Join(root, "db", "notes.txt"), []byte("x"), 0o644)
	names, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("names=%v", names)
	}
}
