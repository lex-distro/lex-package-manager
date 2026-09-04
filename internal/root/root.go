package root

import (
	"os"
	"path/filepath"
)

// $LEX_ROOT or the binary dir, never CWD
func Resolve() (string, error) {
	if v := os.Getenv("LEX_ROOT"); v != "" {
		abs, err := filepath.Abs(v)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// symlinked installs (e.g. /usr/bin/lex)
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

func Join(root string, elems ...string) string {
	parts := append([]string{root}, elems...)
	return filepath.Join(parts...)
}
