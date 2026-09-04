package fetch

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
}

func shaOf(b []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func TestFetchAndCache(t *testing.T) {
	body := []byte("fake tarball bytes")
	srv := serve(t, body)
	defer srv.Close()
	lexRoot := t.TempDir()

	p1, err := Fetch(lexRoot, srv.URL+"/x.tar.gz", shaOf(body))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// second call hits cache
	p2, err := Fetch(lexRoot, srv.URL+"/x.tar.gz", shaOf(body))
	if err != nil {
		t.Fatalf("cached Fetch: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("cache paths differ: %q %q", p1, p2)
	}
}

func TestFetchChecksumMismatch(t *testing.T) {
	srv := serve(t, []byte("real bytes"))
	defer srv.Close()
	if _, err := Fetch(t.TempDir(), srv.URL, shaOf([]byte("other"))); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestFetchBadSHA(t *testing.T) {
	for _, bad := range []string{"", "xyz", "ab12"} {
		if _, err := Fetch(t.TempDir(), "http://example.com/x", bad); err == nil {
			t.Fatalf("sha %q accepted", bad)
		}
	}
}

func TestFetchCorruptCacheRedownloads(t *testing.T) {
	body := []byte("good bytes")
	srv := serve(t, body)
	defer srv.Close()
	lexRoot := t.TempDir()
	p1, err := Fetch(lexRoot, srv.URL, shaOf(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p1, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	p2, err := Fetch(lexRoot, srv.URL, shaOf(body))
	if err != nil {
		t.Fatalf("redownload: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("paths differ: %q %q", p1, p2)
	}
	got, _ := os.ReadFile(p2)
	if string(got) != string(body) {
		t.Fatal("cache not repaired")
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := Fetch(t.TempDir(), srv.URL, shaOf([]byte("x"))); err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestExtract(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not in PATH")
	}
	work := t.TempDir()
	src := filepath.Join(work, "src", "sub")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	tarball := filepath.Join(work, "a.tar.gz")
	cmd := exec.Command("tar", "-czf", tarball, "-C", filepath.Join(work, "src"), ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar create: %v: %s", err, out)
	}
	dest := filepath.Join(work, "dest")
	if err := Extract(tarball, dest); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "sub", "f.txt"))
	if err != nil || string(got) != "hi" {
		t.Fatalf("extracted: %q %v", got, err)
	}
}
