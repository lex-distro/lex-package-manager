package fetch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func Dir(lexRoot string) string {
	return filepath.Join(lexRoot, "cache", "dl")
}

// cached by sha256, corrupt entries re-downloaded, always verified
func Fetch(lexRoot, url, sha string) (string, error) {
	sum, err := parseSHA(sha)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(Dir(lexRoot), 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(Dir(lexRoot), strings.ToLower(hex.EncodeToString(sum)))
	if ok, _ := verify(path, sum); ok {
		return path, nil
	}
	os.Remove(path)

	tmp, err := os.CreateTemp(Dir(lexRoot), ".dl-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url) //nolint:gosec,noctx
	if err != nil {
		tmp.Close()
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return "", fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if !bytes.Equal(h.Sum(nil), sum) {
		return "", fmt.Errorf("checksum mismatch for %s: want %x got %x",
			url, sum, h.Sum(nil))
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	return path, nil
}
// system tar, unzip, unrar; dest replaced first
func Extract(archive, dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(archive))
	name := strings.ToLower(filepath.Base(archive))

	var cmd *exec.Cmd

	switch {
	case ext == ".zip":
		cmd = exec.Command("unzip", "-q", archive, "-d", dest)

	case ext == ".rar":
		cmd = exec.Command("unrar", "x", "-o+", archive, dest)

	case ext == ".tar",
		strings.HasSuffix(name, ".tar.gz"),
		strings.HasSuffix(name, ".tgz"),
		strings.HasSuffix(name, ".tar.bz2"),
		strings.HasSuffix(name, ".tbz2"),
		strings.HasSuffix(name, ".tar.xz"),
		strings.HasSuffix(name, ".txz"):
		cmd = exec.Command("tar", "-xf", archive, "-C", dest)

	default:
		return fmt.Errorf("unsupported archive format: %s", archive)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf(
			"extract %s: %v: %s",
			archive,
			err,
			strings.TrimSpace(string(out)),
		)
	}

	return nil
}


func parseSHA(s string) ([]byte, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != sha256.Size*2 {
		return nil, fmt.Errorf("invalid sha256 %q: want 64 hex chars", s)
	}
	sum, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid sha256 %q: %v", s, err)
	}
	return sum, nil
}

func verify(path string, sum []byte) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return bytes.Equal(h.Sum(nil), sum), nil
}

// no checksum; patches carry none in the schema
func Download(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".patch-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url) //nolint:gosec,noctx
	if err != nil {
		tmp.Close()
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

func IsURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "file://")
}
