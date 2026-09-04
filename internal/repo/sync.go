package repo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mikuripa/lex/internal/conf"
)

// shallow clone first, pull --ff-only after
// local-only repos skipped, failures collected not fatal
func SyncAll(lexRoot string, repos []conf.Repo) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH (required for sync)")
	}
	var failed []string
	for _, r := range repos {
		if r.Server == "" {
			fmt.Printf("sync: [%s] local-only, skipping\n", r.Name)
			continue
		}
		if err := SyncOne(lexRoot, r); err != nil {
			fmt.Printf("sync: [%s] FAILED: %v\n", r.Name, err)
			failed = append(failed, r.Name)
			continue
		}
		fmt.Printf("sync: [%s] ok\n", r.Name)
	}
	if len(failed) > 0 {
		return fmt.Errorf("sync failed for: %s", strings.Join(failed, ", "))
	}
	return nil
}

func SyncOne(lexRoot string, r conf.Repo) error {
	dest := filepath.Join(CacheDir(lexRoot), r.Name)
	if st, err := os.Stat(filepath.Join(dest, ".git")); err != nil || !st.IsDir() {
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s exists but is not a git checkout (remove it or fix Server)", dest)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		args := []string{"clone", "--depth", "1"}
		if r.Branch != "" {
			args = append(args, "--branch", r.Branch)
		}
		args = append(args, r.Server, dest)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("git clone: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if out, err := exec.Command("git", "-C", dest, "pull", "--ff-only").CombinedOutput(); err != nil {
		return fmt.Errorf("git pull: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// "" when unknown; feeds db repo_rev
func Rev(lexRoot, repoName string) string {
	dest := filepath.Join(CacheDir(lexRoot), repoName)
	out, err := exec.Command("git", "-C", dest, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
