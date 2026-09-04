package execx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSuccess(t *testing.T) {
	dir := t.TempDir()
	var log bytes.Buffer
	err := Run([]string{"", "echo hello > out.txt", "mkdir -p sub"}, dir, nil, &log)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil || strings.TrimSpace(string(got)) != "hello" {
		t.Fatalf("out=%q err=%v", got, err)
	}
}

func TestRunFailureIncludesOutput(t *testing.T) {
	err := Run([]string{"echo before-fail-marker; exit 3"}, t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "before-fail-marker") {
		t.Fatalf("error lacks output: %v", err)
	}
}

func TestRunStopsAtFirstFailure(t *testing.T) {
	dir := t.TempDir()
	err := Run([]string{"exit 1", "touch should-not-exist"}, dir, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, serr := os.Stat(filepath.Join(dir, "should-not-exist")); !os.IsNotExist(serr) {
		t.Fatal("continued after failure")
	}
}

func TestRunEnvAndDir(t *testing.T) {
	dir := t.TempDir()
	err := Run([]string{`test "$LEX_TEST_VAR" = "ok" && pwd > pwd.txt`}, dir, []string{"LEX_TEST_VAR=ok"}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "pwd.txt"))
	if strings.TrimSpace(string(got)) != dir {
		t.Fatalf("workdir=%q want %q", strings.TrimSpace(string(got)), dir)
	}
}
