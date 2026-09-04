package repo

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikuripa/lex/internal/conf"
)

// "@name" to ordered unique packages
// local groups/ first, then synced repos; nested @ allowed, cycles error
func ExpandGroup(lexRoot string, repos []conf.Repo, ref string) ([]string, error) {
	name := strings.TrimPrefix(ref, "@")
	if name == "" || name == ref || !nameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid group reference %q (want @name)", ref)
	}
	visited := map[string]bool{}
	return expandGroup(lexRoot, repos, name, visited)
}

func expandGroup(lexRoot string, repos []conf.Repo, name string, visited map[string]bool) ([]string, error) {
	if visited[name] {
		return nil, fmt.Errorf("group cycle detected at @%s", name)
	}
	visited[name] = true
	path, err := groupFile(lexRoot, repos, name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// trailing " #" comments too
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@") {
			sub, err := expandGroup(lexRoot, repos, strings.TrimPrefix(line, "@"), visited)
			if err != nil {
				return nil, err
			}
			for _, p := range sub {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
			continue
		}
		if err := ValidateName(line); err != nil {
			return nil, fmt.Errorf("group @%s: %v", name, err)
		}
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	delete(visited, name)
	return out, nil
}

func groupFile(lexRoot string, repos []conf.Repo, name string) (string, error) {
	local := filepath.Join(lexRoot, "groups", name+".txt")
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	for _, r := range repos {
		p := filepath.Join(CacheDir(lexRoot), r.Name, "groups", name+".txt")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("group @%s not found", name)
}
