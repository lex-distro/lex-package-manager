package execx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// sh -c each snippet, fail fast with output in the error
func Run(cmds []string, dir string, extraEnv []string, logW io.Writer) error {
	for _, c := range cmds {
		if strings.TrimSpace(c) == "" {
			continue
		}
		cmd := exec.Command("sh", "-c", c)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), extraEnv...)
		var buf bytes.Buffer
		w := io.Writer(&buf)
		if logW != nil {
			w = io.MultiWriter(logW, &buf)
		}
		cmd.Stdout = w
		cmd.Stderr = w
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command failed: %s: %v\n%s", c, err, tailLines(buf.String(), 20))
		}
	}
	return nil
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
