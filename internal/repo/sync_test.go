package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mikuripa/lex/internal/conf"
)

func gitOK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
}

func initRemote(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	gitOK(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run("init", "-b", "main")
	for p, content := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
	}
	run("commit", "-m", "init")
}

func TestSyncCloneAndPull(t *testing.T) {
	gitOK(t)
	remote := filepath.Join(t.TempDir(), "remote")
	initRemote(t, remote, map[string]string{
		"pkg/x86_64/a.lexbuild": `{"kind":"recipe","name":"a","version":"1","binary":{"url":"u","sha256":"s"}}`,
	})
	lexRoot := t.TempDir()
	repos := []conf.Repo{{Name: "stable", Server: remote, Branch: "main", Include: "pkg/x86_64/"}}
	if err := SyncAll(lexRoot, repos); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if _, _, err := Find(lexRoot, repos, "a"); err != nil {
		t.Fatalf("find after clone: %v", err)
	}
	if rev := Rev(lexRoot, "stable"); rev == "" {
		t.Fatal("empty rev after clone")
	}
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "bump")
	cmd.Dir = remote
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("upstream commit: %v: %s", err, out)
	}
	if err := SyncAll(lexRoot, repos); err != nil {
		t.Fatalf("pull: %v", err)
	}
}

func TestSyncSkipsLocalOnly(t *testing.T) {
	gitOK(t)
	lexRoot := t.TempDir()
	repos := []conf.Repo{{Name: "local", Include: "pkg/x86_64/"}}
	if err := SyncAll(lexRoot, repos); err != nil {
		t.Fatalf("local-only sync: %v", err)
	}
	if _, err := os.Stat(CacheDir(lexRoot)); !os.IsNotExist(err) {
		t.Fatalf("cache dir should not exist: %v", err)
	}
}

func TestSyncFailsCleanly(t *testing.T) {
	gitOK(t)
	lexRoot := t.TempDir()
	repos := []conf.Repo{{Name: "bad", Server: filepath.Join(t.TempDir(), "nope"), Include: "pkg/"}}
	if err := SyncAll(lexRoot, repos); err == nil {
		t.Fatal("expected error for bad remote")
	}
}
