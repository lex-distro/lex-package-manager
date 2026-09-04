package remove

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikuripa/lex/internal/conf"
	"github.com/mikuripa/lex/internal/db"
	"github.com/mikuripa/lex/internal/install"
)

func testRoot(t *testing.T) (string, conf.Config) {
	t.Helper()
	lexRoot := t.TempDir()
	t.Setenv(install.FakeRootEnv, filepath.Join(lexRoot, "fakeroot"))
	cfg := conf.Config{AllowNonRoot: true,
		Repos: []conf.Repo{{Name: "local", Include: "pkg/x86_64/"}}}
	return lexRoot, cfg
}

func writeRecipe(t *testing.T, lexRoot, name, stanza string) {
	t.Helper()
	dir := filepath.Join(lexRoot, "pkg", "x86_64")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf(`{"kind":"recipe","name":%q,"version":"1.0",%s}`, name, stanza)
	if err := os.WriteFile(filepath.Join(dir, name+".lexbuild"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedInstalled(t *testing.T, lexRoot, name string, files []string) {
	t.Helper()
	fake := os.Getenv(install.FakeRootEnv)
	for _, f := range files {
		p := filepath.Join(fake, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Save(lexRoot, &db.Installed{Name: name, Version: "1.0", Method: "binary", Files: files}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveEndToEnd(t *testing.T) {
	lexRoot, cfg := testRoot(t)
	fake := os.Getenv(install.FakeRootEnv)
	// Shared dir with an untracked file must survive.
	os.MkdirAll(filepath.Join(fake, "usr", "bin"), 0o755)
	os.WriteFile(filepath.Join(fake, "usr", "bin", "keep"), []byte("k"), 0o644)
	hookMarker := filepath.Join(lexRoot, "hook-ran")
	writeRecipe(t, lexRoot, "demo",
		fmt.Sprintf(`"binary":{"url":"u","sha256":"s","install":[],"post_remove":["touch %s"]}`, hookMarker))
	files := []string{"/usr", "/usr/bin", "/usr/bin/demo", "/usr/bin/demo-link", "/usr/share/demo", "/usr/share/demo/f.txt"}
	seedInstalled(t, lexRoot, "demo",
		[]string{"/usr/bin/demo", "/usr/share/demo/f.txt"})
	if err := os.Symlink("demo", filepath.Join(fake, "usr", "bin", "demo-link")); err != nil {
		t.Fatal(err)
	}
	// dir entries like a real manifest
	ins, _ := db.Load(lexRoot, "demo")
	ins.Files = files
	db.Save(lexRoot, ins)

	if err := Remove(lexRoot, cfg, "demo"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, gone := range []string{"usr/bin/demo", "usr/bin/demo-link", "usr/share/demo/f.txt", "usr/share/demo"} {
		if _, err := os.Lstat(filepath.Join(fake, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone: %v", gone, err)
		}
	}
	// Critical / shared paths survive.
	if _, err := os.Stat(filepath.Join(fake, "usr", "bin", "keep")); err != nil {
		t.Fatalf("shared file removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fake, "usr")); err != nil {
		t.Fatalf("/usr removed: %v", err)
	}
	if _, err := os.Stat(hookMarker); err != nil {
		t.Fatalf("hook did not run: %v", err)
	}
	if _, err := db.Load(lexRoot, "demo"); !os.IsNotExist(err) {
		t.Fatalf("db entry should be gone: %v", err)
	}
}

func TestRemoveSkipsMissing(t *testing.T) {
	lexRoot, cfg := testRoot(t)
	writeRecipe(t, lexRoot, "ghost", `"binary":{"url":"u","sha256":"s","install":[]}`)
	seedInstalled(t, lexRoot, "ghost", []string{"/usr/bin/ghost"})
	// already-gone files are fine
	os.RemoveAll(filepath.Join(os.Getenv(install.FakeRootEnv), "usr"))
	if err := Remove(lexRoot, cfg, "ghost"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestRemoveNotInstalled(t *testing.T) {
	lexRoot, cfg := testRoot(t)
	if err := Remove(lexRoot, cfg, "nope"); err == nil {
		t.Fatal("expected not-installed error")
	}
}

func TestRemoveBadName(t *testing.T) {
	lexRoot, cfg := testRoot(t)
	if err := Remove(lexRoot, cfg, "../x"); err == nil {
		t.Fatal("expected name error")
	}
}

func TestRemoveKeepsDBOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("needs non-root (chmod does not stop root)")
	}
	lexRoot, cfg := testRoot(t)
	fake := os.Getenv(install.FakeRootEnv)
	locked := filepath.Join(fake, "usr", "locked")
	// locked dir: inner delete fails, surrounding tree ops still work
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	writeRecipe(t, lexRoot, "locked", `"binary":{"url":"u","sha256":"s","install":[]}`)
	seedInstalled(t, lexRoot, "locked", []string{"/usr/locked/f"})
	// dir entry like a real manifest
	ins, _ := db.Load(lexRoot, "locked")
	ins.Files = []string{"/usr/locked", "/usr/locked/f"}
	db.Save(lexRoot, ins)
	if err := Remove(lexRoot, cfg, "locked"); err == nil {
		t.Fatal("expected deletion error")
	}
	if _, err := db.Load(lexRoot, "locked"); err != nil {
		t.Fatalf("db entry should be kept for retry: %v", err)
	}
	os.Chmod(locked, 0o755)
}

func TestRemoveHookFailureKeepsDB(t *testing.T) {
	lexRoot, cfg := testRoot(t)
	writeRecipe(t, lexRoot, "hookfail",
		`"binary":{"url":"u","sha256":"s","install":[],"post_remove":["exit 9"]}`)
	seedInstalled(t, lexRoot, "hookfail", []string{"/usr/bin/hookfail"})
	if err := Remove(lexRoot, cfg, "hookfail"); err == nil {
		t.Fatal("expected hook error")
	}
	if _, err := db.Load(lexRoot, "hookfail"); err != nil {
		t.Fatalf("db entry should be kept: %v", err)
	}
}

func TestRemoveWithoutRecipe(t *testing.T) {
	lexRoot, cfg := testRoot(t)
	seedInstalled(t, lexRoot, "orphan", []string{"/usr/bin/orphan"})
	if err := Remove(lexRoot, cfg, "orphan"); err != nil {
		t.Fatalf("Remove without recipe: %v", err)
	}
}

func TestVerifyOK(t *testing.T) {
	lexRoot, _ := testRoot(t)
	seedInstalled(t, lexRoot, "demo", []string{"/usr/bin/demo"})
	if err := Verify(lexRoot, "demo"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyMissing(t *testing.T) {
	lexRoot, _ := testRoot(t)
	seedInstalled(t, lexRoot, "demo", []string{"/usr/bin/demo", "/usr/bin/gone"})
	fake := os.Getenv(install.FakeRootEnv)
	os.Remove(filepath.Join(fake, "usr", "bin", "gone"))
	err := Verify(lexRoot, "demo")
	if err == nil {
		t.Fatal("expected missing-files error")
	}
	if got := err.Error(); !strings.Contains(got, "/usr/bin/gone") {
		t.Fatalf("error should name the file: %v", err)
	}
}

func TestVerifyNotInstalled(t *testing.T) {
	if err := Verify(t.TempDir(), "nope"); err == nil {
		t.Fatal("expected not-installed error")
	}
}
