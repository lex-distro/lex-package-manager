package install

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mikuripa/lex/internal/conf"
	"github.com/mikuripa/lex/internal/db"
	execx "github.com/mikuripa/lex/internal/exec"
	"github.com/mikuripa/lex/internal/fetch"
	"github.com/mikuripa/lex/internal/recipe"
	"github.com/mikuripa/lex/internal/repo"
)

// tests only; production always targets "/"
const FakeRootEnv = "LEX_FAKE_ROOT"

func BuildDir(lexRoot, name string) string {
	return filepath.Join(lexRoot, "cache", "build", name)
}

func RequireRoot(cfg conf.Config) error {
	if os.Geteuid() == 0 {
		return nil
	}
	if cfg.AllowNonRoot {
		return nil
	}
	return fmt.Errorf("must run as root to install system packages (or set AllowNonRoot = True)")
}

func destRoot() string {
	return DestRoot()
}

// shared with remove/verify so tests stay hermetic
func DestRoot() string {
	if v := os.Getenv(FakeRootEnv); v != "" {
		return v
	}
	return "/"
}

func Install(lexRoot string, cfg conf.Config, name, methodOverride string) error {
	if err := repo.ValidateName(name); err != nil {
		return err
	}
	r, src, err := repo.Find(lexRoot, cfg.Repos, name)
	if err != nil {
		return err
	}
	prefer := cfg.Prefer
	if methodOverride != "" {
		prefer = methodOverride
	}
	method, err := r.SelectMethod(prefer)
	if err != nil {
		return err
	}
	if !r.SupportsArch(runtime.GOARCH) {
		return fmt.Errorf("recipe arch %v does not support %s", r.Arch, runtime.GOARCH)
	}
	if err := RequireRoot(cfg); err != nil {
		return err
	}
	switch method {
	case "binary":
		return InstallBinary(lexRoot, cfg, r, src)
	case "source":
		return InstallSource(lexRoot, cfg, r, src)
	default:
		return fmt.Errorf("unknown method %q", method)
	}
}

func InstallBinary(lexRoot string, cfg conf.Config, r *recipe.Recipe, src string) error {
	b := r.Binary
	tarball, err := fetch.Fetch(lexRoot, b.URL, b.SHA256)
	if err != nil {
		return err
	}
	base := BuildDir(lexRoot, r.Name)
	srcDir := filepath.Join(base, "src")
	stage := filepath.Join(base, "stage")
	if err := fetch.Extract(tarball, srcDir); err != nil {
		return err
	}
	if err := freshDir(stage); err != nil {
		return err
	}

	logW, closeLog, err := openLog(lexRoot, cfg, r.Name)
	if err != nil {
		return err
	}
	defer closeLog()
	if err := execx.Run(b.Install, srcDir, []string{"DESTDIR=" + stage}, logW); err != nil {
		return err
	}
	return finalize(lexRoot, cfg, r, src, stage, "binary")
}

func InstallSource(lexRoot string, cfg conf.Config, r *recipe.Recipe, src string) error {
	s := r.Source
	tarball, err := fetch.Fetch(lexRoot, s.URL, s.SHA256)
	if err != nil {
		return err
	}
	base := BuildDir(lexRoot, r.Name)
	srcDir := filepath.Join(base, "src")
	stage := filepath.Join(base, "stage")
	if err := fetch.Extract(tarball, srcDir); err != nil {
		return err
	}
	root, err := sourceRoot(srcDir)
	if err != nil {
		return err
	}
	if err := applyPatches(lexRoot, r, src, s.Patches, root); err != nil {
		return err
	}
	if err := freshDir(stage); err != nil {
		return err
	}

	logW, closeLog, err := openLog(lexRoot, cfg, r.Name)
	if err != nil {
		return err
	}
	defer closeLog()
	if err := execx.Run(s.Build, root, buildEnv(cfg), logW); err != nil {
		return err
	}
	if err := execx.Run(s.Install, root, []string{"DESTDIR=" + stage}, logW); err != nil {
		return err
	}
	return finalize(lexRoot, cfg, r, src, stage, "source")
}

func finalize(lexRoot string, cfg conf.Config, r *recipe.Recipe, src, stage, method string) error {
	files, err := manifest(stage)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("install produced no files (empty DESTDIR stage)")
	}
	if err := copyStaged(stage, destRoot()); err != nil {
		return err
	}
	ins := &db.Installed{
		Name:        r.Name,
		Version:     r.Version,
		Method:      method,
		RepoRev:     revFor(lexRoot, cfg, src),
		Files:       files,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := db.Save(lexRoot, ins); err != nil {
		return err
	}
	fmt.Printf("installed %s %s (%s, %d files)\n", r.Name, r.Version, method, len(files))
	return nil
}

func openLog(lexRoot string, cfg conf.Config, name string) (io.Writer, func(), error) {
	if !cfg.DetailedLog {
		return nil, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Join(lexRoot, "logs"), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.Create(filepath.Join(lexRoot, "logs",
		fmt.Sprintf("%s-%s.log", name, time.Now().UTC().Format("20060102-150405"))))
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

func freshDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

// lone top-level dir wins, else the extract dir itself
func sourceRoot(srcDir string) (string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(srcDir, entries[0].Name()), nil
	}
	return srcDir, nil
}

// make picks this up as -j
func buildEnv(cfg conf.Config) []string {
	if cfg.Jobs > 0 {
		return []string{fmt.Sprintf("MAKEFLAGS=-j%d", cfg.Jobs)}
	}
	return nil
}

// URL, absolute path, or recipe-relative; applied with patch -p1
func applyPatches(lexRoot string, r *recipe.Recipe, src string, patches []string, root string) error {
	recipeDir := filepath.Dir(src)
	for i, p := range patches {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var patchFile string
		switch {
		case fetch.IsURL(p):
			u := p
			if strings.HasPrefix(u, "file://") {
				patchFile = strings.TrimPrefix(u, "file://")
			} else {
				patchFile = filepath.Join(BuildDir(lexRoot, r.Name), fmt.Sprintf("patch-%d.patch", i))
				fmt.Printf("fetching patch %s\n", u)
				if err := fetch.Download(u, patchFile); err != nil {
					return err
				}
			}
		case filepath.IsAbs(p):
			patchFile = p
		default:
			patchFile = filepath.Join(recipeDir, filepath.FromSlash(p))
		}
		if _, err := os.Stat(patchFile); err != nil {
			return fmt.Errorf("patch %q not found: %w", p, err)
		}
		if out, err := runPatch(root, patchFile); err != nil {
			return fmt.Errorf("patch %q failed: %v: %s", p, err, out)
		}
	}
	return nil
}

func runPatch(dir, patchFile string) (string, error) {
	cmd := exec.Command("patch", "-p1", "--forward", "-i", patchFile)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func manifest(stage string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == stage {
			return nil
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.Join(string(filepath.Separator), rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// keeps modes and symlinks
func copyStaged(stage, dest string) error {
	return filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode().IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return copyFile(p, target, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported file type: %s", p)
		}
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// "" unless src came from a synced checkout
func revFor(lexRoot string, cfg conf.Config, src string) string {
	for _, r := range cfg.Repos {
		prefix := filepath.Join(repo.CacheDir(lexRoot), r.Name) + string(filepath.Separator)
		if strings.HasPrefix(src, prefix) {
			return repo.Rev(lexRoot, r.Name)
		}
	}
	return ""
}
