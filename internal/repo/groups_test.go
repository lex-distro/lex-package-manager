package repo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mikuripa/lex/internal/conf"
)

func writeGroup(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExpandGroupLocal(t *testing.T) {
	lexRoot := t.TempDir()
	writeGroup(t, filepath.Join(lexRoot, "groups"), "base",
		"# comment\n\nglibc\nbinutils # trailing\nglibc\n")
	got, err := ExpandGroup(lexRoot, nil, "@base")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"glibc", "binutils"}) {
		t.Fatalf("got %v", got)
	}
}

func TestExpandGroupNested(t *testing.T) {
	lexRoot := t.TempDir()
	writeGroup(t, filepath.Join(lexRoot, "groups"), "inner", "b\n")
	writeGroup(t, filepath.Join(lexRoot, "groups"), "outer", "a\n@inner\nc\n")
	got, err := ExpandGroup(lexRoot, nil, "@outer")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %v", got)
	}
}

func TestExpandGroupCycle(t *testing.T) {
	lexRoot := t.TempDir()
	writeGroup(t, filepath.Join(lexRoot, "groups"), "a", "@b\n")
	writeGroup(t, filepath.Join(lexRoot, "groups"), "b", "@a\n")
	if _, err := ExpandGroup(lexRoot, nil, "@a"); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestExpandGroupMissing(t *testing.T) {
	if _, err := ExpandGroup(t.TempDir(), nil, "@ghost"); err == nil {
		t.Fatal("expected not found")
	}
	if _, err := ExpandGroup(t.TempDir(), nil, "base"); err == nil {
		t.Fatal("expected error without @")
	}
}

func TestExpandGroupFromRepoCache(t *testing.T) {
	lexRoot := t.TempDir()
	repos := []conf.Repo{{Name: "stable", Server: "https://example.com/r.git", Include: "pkg/"}}
	writeGroup(t, filepath.Join(CacheDir(lexRoot), "stable", "groups"), "net", "curl\n")
	got, err := ExpandGroup(lexRoot, repos, "@net")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"curl"}) {
		t.Fatalf("got %v", got)
	}
}

func TestExpandGroupBadEntry(t *testing.T) {
	lexRoot := t.TempDir()
	writeGroup(t, filepath.Join(lexRoot, "groups"), "bad", "../evil\n")
	if _, err := ExpandGroup(lexRoot, nil, "@bad"); err == nil {
		t.Fatal("expected validation error")
	}
}
