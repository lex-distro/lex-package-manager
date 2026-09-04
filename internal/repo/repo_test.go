package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mikuripa/lex/internal/conf"
)

func writeRecipe(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".lexbuild"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateName(t *testing.T) {
	if err := ValidateName("fastfetch"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "../../etc/x", "a/b", "x y"} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestFindLocalShadowFirst(t *testing.T) {
	root := t.TempDir()
	repos := []conf.Repo{{Name: "stable", Server: "https://example.com/r.git", Include: "pkg/x86_64/"}}
	writeRecipe(t, filepath.Join(root, "pkg", "x86_64"), "mypkg",
		`{"kind":"recipe","name":"mypkg","version":"9.9","binary":{"url":"u","sha256":"s"}}`)
	r, src, err := Find(root, repos, "mypkg")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if r.Version != "9.9" || src == "" {
		t.Fatalf("got %+v %q", r, src)
	}
}
func TestFindNotFound(t *testing.T) {
	root := t.TempDir()
	repos := []conf.Repo{{Name: "stable", Include: "pkg/x86_64/"}}
	if _, _, err := Find(root, repos, "ghost"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestFindAbsoluteInclude(t *testing.T) {
	lexRoot := t.TempDir()
	extDir := t.TempDir()
	writeRecipe(t, extDir, "ext",
		`{"kind":"recipe","name":"ext","version":"1","binary":{"url":"u","sha256":"s"}}`)
	repos := []conf.Repo{{Name: "ext", Include: extDir}}
	r, src, err := Find(lexRoot, repos, "ext")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if r.Name != "ext" || src == "" {
		t.Fatalf("got %+v %q", r, src)
	}
}

func TestFindNameMismatch(t *testing.T) {
	root := t.TempDir()
	repos := []conf.Repo{{Name: "stable", Include: "pkg/x86_64/"}}
	writeRecipe(t, filepath.Join(root, "pkg", "x86_64"), "claimed",
		`{"kind":"recipe","name":"other","version":"1","binary":{"url":"u","sha256":"s"}}`)
	if _, _, err := Find(root, repos, "claimed"); err == nil {
		t.Fatal("expected name mismatch error")
	}
}
