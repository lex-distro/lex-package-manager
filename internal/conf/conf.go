package conf

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Repo struct {
	Name    string
	Server  string
	Branch  string
	Include string
}

type Config struct {
	Repos        []Repo
	Prefer       string // "binary" | "source"
	InfoMode     string // "combined" | "installed"
	Color        bool
	DetailedLog  bool
	Jobs         int
	AllowNonRoot bool
}

func Default() Config {
	return Config{Prefer: "binary", InfoMode: "combined", Jobs: 4}
}

// lex.conf shape:
//
//	[stable]
//	Server = https://github.com/mikuripa/lex-pkgs
//	Include = pkg/x86_64/
//
//	[options]
//	Prefer = binary
//	InfoMode = combined
//	Color = True
func ParseFile(path string) (Config, error) {
	cfg := Default()
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	sections := map[string]map[string]string{}
	current := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(stripInlineComment(sc.Text()))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if current == "" {
				return cfg, fmt.Errorf("empty section name in %s", path)
			}
			if _, ok := sections[current]; !ok {
				sections[current] = map[string]string{}
			}
			continue
		}
		if current == "" {
			return cfg, fmt.Errorf("key=value outside section in %s: %q", path, line)
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return cfg, fmt.Errorf("invalid line in %s: %q", path, line)
		}
		sections[current][strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	if err := sc.Err(); err != nil {
		return cfg, err
	}

	for name, kv := range sections {
		if strings.EqualFold(name, "options") {
			if v, ok := kv["prefer"]; ok {
				v = strings.ToLower(v)
				if v != "binary" && v != "source" {
					return cfg, fmt.Errorf("invalid Prefer %q: want binary|source", v)
				}
				cfg.Prefer = v
			}
			if v, ok := kv["infomode"]; ok {
				v = strings.ToLower(v)
				if v != "combined" && v != "installed" {
					return cfg, fmt.Errorf("invalid InfoMode %q: want combined|installed", v)
				}
				cfg.InfoMode = v
			}
			if v, ok := kv["color"]; ok {
				b, err := parseBool(v)
				if err != nil {
					return cfg, fmt.Errorf("invalid Color %q", v)
				}
				cfg.Color = b
			}
			if v, ok := kv["detailedlog"]; ok {
				b, err := parseBool(v)
				if err != nil {
					return cfg, fmt.Errorf("invalid DetailedLog %q", v)
				}
				cfg.DetailedLog = b
			}
			if v, ok := kv["jobs"]; ok {
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 {
					return cfg, fmt.Errorf("invalid Jobs %q", v)
				}
				cfg.Jobs = n
			}
			if v, ok := kv["allownonroot"]; ok {
				b, err := parseBool(v)
				if err != nil {
					return cfg, fmt.Errorf("invalid AllowNonRoot %q", v)
				}
				cfg.AllowNonRoot = b
			}
			continue
		}
		r := Repo{Name: name}
		r.Server = kv["server"]
		r.Branch = kv["branch"]
		r.Include = kv["include"]
		if r.Include == "" {
			return cfg, fmt.Errorf("repo [%s] missing Include", name)
		}
		cfg.Repos = append(cfg.Repos, r)
	}
	sort.Slice(cfg.Repos, func(i, j int) bool { return cfg.Repos[i].Name < cfg.Repos[j].Name })
	return cfg, nil
}

// stripInlineComment cuts " #..." and " ;..." suffixes
// whitespace prefix required, so URLs with '#' survive
func stripInlineComment(s string) string {
	for i := 1; i < len(s); i++ {
		if (s[i] == '#' || s[i] == ';') && (s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimSpace(s[:i-1])
		}
	}
	return s
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("not a bool: %q", s)
	}
}

// relative Include resolves against the repo cache when Server is set,
// else against lexRoot
func (r Repo) RecipeDir(lexRoot, repoCache string) string {
	inc := r.Include
	if filepath.IsAbs(inc) {
		return filepath.Clean(inc)
	}
	if r.Server != "" && repoCache != "" {
		return filepath.Join(repoCache, r.Name, filepath.FromSlash(inc))
	}
	return filepath.Join(lexRoot, filepath.FromSlash(inc))
}
