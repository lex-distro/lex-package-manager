package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/mikuripa/lex/internal/conf"
	"github.com/mikuripa/lex/internal/recipe"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func CacheDir(lexRoot string) string {
	return filepath.Join(lexRoot, "cache", "repo")
}

// no empty names, no path traversal
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid package name %q", name)
	}
	return nil
}

// local shadow, then synced cache, then legacy pkg/
// absolute Include is used as-is
func Candidates(lexRoot string, repos []conf.Repo, name string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = filepath.Clean(p)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	cache := CacheDir(lexRoot)
	for _, r := range repos {
		if r.Server != "" && !filepath.IsAbs(r.Include) {
			add(filepath.Join(lexRoot, filepath.FromSlash(r.Include), name+".lexbuild"))
		}
		add(filepath.Join(r.RecipeDir(lexRoot, cache), name+".lexbuild"))
	}
	add(filepath.Join(lexRoot, "pkg", name+".lexbuild"))
	return out
}

// first hit wins
func Find(lexRoot string, repos []conf.Repo, name string) (*recipe.Recipe, string, error) {
	if err := ValidateName(name); err != nil {
		return nil, "", err
	}
	cands := Candidates(lexRoot, repos, name)
	for _, p := range cands {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		r, err := recipe.Load(p)
		if err != nil {
			return nil, "", err
		}
		if r.Name != name {
			return nil, "", fmt.Errorf("recipe %s declares name %q", p, r.Name)
		}
		return r, p, nil
	}
	return nil, "", fmt.Errorf("package %q not found (searched %d locations)", name, len(cands))
}
