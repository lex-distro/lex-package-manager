package remove

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/mikuripa/lex/internal/conf"
	"github.com/mikuripa/lex/internal/db"
	execx "github.com/mikuripa/lex/internal/exec"
	"github.com/mikuripa/lex/internal/install"
	"github.com/mikuripa/lex/internal/repo"
)

// never deleted even if listed; dirs only go when empty
var criticalPaths = map[string]bool{
	"/": true, "/usr": true, "/bin": true, "/sbin": true,
	"/lib": true, "/lib64": true, "/etc": true, "/var": true,
	"/opt": true, "/srv": true, "/home": true, "/root": true,
	"/boot": true, "/dev": true, "/proc": true, "/sys": true,
	"/run": true, "/tmp": true, "/mnt": true, "/media": true,
}

// missing files skipped, failures keep the db entry for retry
func Remove(lexRoot string, cfg conf.Config, name string) error {
	if err := repo.ValidateName(name); err != nil {
		return err
	}
	ins, err := db.Load(lexRoot, name)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("package %q is not installed", name)
		}
		return err
	}
	if err := install.RequireRoot(cfg); err != nil {
		return err
	}

	dest := install.DestRoot()
	ordered := orderDeepestFirst(ins.Files)
	var failed []string
	var removed, skipped int
	for _, f := range ordered {
		ok, gone, ferr := removeOne(dest, f)
		switch {
		case ferr != nil:
			failed = append(failed, fmt.Sprintf("%s: %v", f, ferr))
		case gone:
			skipped++
		case ok:
			removed++
		}
	}

	if hooks := loadHooks(lexRoot, cfg, ins); len(hooks) > 0 {
		logW, closeLog, err := openLog(lexRoot, cfg, name)
		if err != nil {
			return err
		}
		hookErr := execx.Run(hooks, "/", nil, logW)
		closeLog()
		if hookErr != nil {
			return fmt.Errorf("post_remove hooks failed (db entry kept): %w", hookErr)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("remove %s: %d files could not be deleted (db entry kept):\n  %s",
			name, len(failed), strings.Join(failed, "\n  "))
	}
	if err := os.Remove(db.Path(lexRoot, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("removed %s %s (%d files, %d already gone)\n", ins.Name, ins.Version, removed, skipped)
	return nil
}

func Verify(lexRoot, name string) error {
	if err := repo.ValidateName(name); err != nil {
		return err
	}
	ins, err := db.Load(lexRoot, name)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("package %q is not installed", name)
		}
		return err
	}
	dest := install.DestRoot()
	var missing []string
	for _, f := range ins.Files {
		if !filepath.IsAbs(f) {
			return fmt.Errorf("manifest entry not absolute: %q", f)
		}
		if _, err := os.Lstat(filepath.Join(dest, filepath.Clean(f))); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, f)
				continue
			}
			return fmt.Errorf("verify %s: %w", f, err)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d missing files:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	fmt.Printf("%s: OK (%d files)\n", name, len(ins.Files))
	return nil
}

// returns removed, alreadyGone
func removeOne(dest, f string) (bool, bool, error) {
	if !filepath.IsAbs(f) {
		return false, false, fmt.Errorf("manifest entry not absolute: %q", f)
	}
	clean := filepath.Clean(f)
	if criticalPaths[clean] {
		return false, true, nil
	}
	target := filepath.Join(dest, clean)
	st, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return false, true, nil
		}
		return false, false, err
	}
	if st.IsDir() {
		if err := os.Remove(target); err != nil {
			if os.IsNotExist(err) || isNotEmpty(err) {
				return false, true, nil
			}
			return false, false, err
		}
		return true, false, nil
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return false, true, nil
		}
		return false, false, err
	}
	return true, false, nil
}

func isNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST)
}

// children vanish before parents
func orderDeepestFirst(files []string) []string {
	out := append([]string(nil), files...)
	sort.Slice(out, func(i, j int) bool {
		di := strings.Count(out[i], "/")
		dj := strings.Count(out[j], "/")
		if di != dj {
			return di > dj
		}
		return out[i] < out[j]
	})
	return out
}

// nil when the recipe is gone; files still go
func loadHooks(lexRoot string, cfg conf.Config, ins *db.Installed) []string {
	r, _, err := repo.Find(lexRoot, cfg.Repos, ins.Name)
	if err != nil {
		return nil
	}
	var hooks []string
	switch ins.Method {
	case "binary":
		if r.Binary != nil {
			hooks = append(hooks, r.Binary.PostRemove...)
		}
	case "source":
		if r.Source != nil {
			hooks = append(hooks, r.Source.PostRemove...)
		}
	}
	hooks = append(hooks, r.PostRemove...)
	var out []string
	for _, h := range hooks {
		if strings.TrimSpace(h) != "" {
			out = append(out, h)
		}
	}
	return out
}

// same per-op log as install, for hook output
func openLog(lexRoot string, cfg conf.Config, name string) (io.Writer, func(), error) {
	if !cfg.DetailedLog {
		return nil, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Join(lexRoot, "logs"), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.Create(filepath.Join(lexRoot, "logs",
		fmt.Sprintf("%s-remove-%s.log", name, time.Now().UTC().Format("20060102-150405"))))
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}
