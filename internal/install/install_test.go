package install

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mikuripa/lex/internal/conf"
	"github.com/mikuripa/lex/internal/db"
)

func TestRequireRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	if err := RequireRoot(conf.Config{}); err == nil {
		t.Fatal("expected root error")
	}
	if err := RequireRoot(conf.Config{AllowNonRoot: true}); err != nil {
		t.Fatalf("AllowNonRoot: %v", err)
	}
}

func TestManifestSorted(t *testing.T) {
	stage := t.TempDir()
	os.MkdirAll(filepath.Join(stage, "usr", "bin"), 0o755)
	os.WriteFile(filepath.Join(stage, "usr", "bin", "b"), []byte("b"), 0o755)
	os.WriteFile(filepath.Join(stage, "usr", "share.txt"), []byte("s"), 0o644)
	got, err := manifest(stage)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr", "/usr/bin", "/usr/bin/b", "/usr/share.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCopyStaged(t *testing.T) {
	stage := t.TempDir()
	os.MkdirAll(filepath.Join(stage, "usr", "bin"), 0o755)
	os.WriteFile(filepath.Join(stage, "usr", "bin", "tool"), []byte("#!/bin/sh\n"), 0o755)
	os.Symlink("tool", filepath.Join(stage, "usr", "bin", "alias"))
	dest := t.TempDir()

	if err := copyStaged(stage, dest); err != nil {
		t.Fatalf("copyStaged: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "usr", "bin", "tool"))
	if err != nil || string(got) != "#!/bin/sh\n" {
		t.Fatalf("file=%q err=%v", got, err)
	}
	if st, _ := os.Stat(filepath.Join(dest, "usr", "bin", "tool")); st.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o", st.Mode().Perm())
	}
	link, err := os.Readlink(filepath.Join(dest, "usr", "bin", "alias"))
	if err != nil || link != "tool" {
		t.Fatalf("symlink=%q err=%v", link, err)
	}
}

// hermetic: tarball over HTTP, install to a fake root
func TestInstallBinaryEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not in PATH")
	}
	if os.Geteuid() == 0 {
		t.Log("running as root; still hermetic via LEX_FAKE_ROOT")
	}
	work := t.TempDir()
	payload := filepath.Join(work, "payload")
	os.MkdirAll(filepath.Join(payload, "bin"), 0o755)
	os.WriteFile(filepath.Join(payload, "bin", "demo"), []byte("demo-bin"), 0o755)
	tarball := filepath.Join(work, "demo.tar.gz")
	cmd := exec.Command("tar", "-czf", tarball, "-C", filepath.Join(work, "payload"), ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v: %s", err, out)
	}
	tarBytes, _ := os.ReadFile(tarball)
	sum := fmt.Sprintf("%x", sha256.Sum256(tarBytes))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarBytes)
	}))
	defer srv.Close()

	lexRoot := t.TempDir()
	recipeDir := filepath.Join(lexRoot, "pkg", "x86_64")
	os.MkdirAll(recipeDir, 0o755)
	recipeJSON := fmt.Sprintf(`{"kind":"recipe","name":"demo","version":"1.0",
"binary":{"url":%q,"sha256":%q,
"install":["mkdir -p $DESTDIR/usr/bin","cp -a bin/demo $DESTDIR/usr/bin/demo"]}}`,
		srv.URL+"/demo.tar.gz", sum)
	os.WriteFile(filepath.Join(recipeDir, "demo.lexbuild"), []byte(recipeJSON), 0o644)

	fakeRoot := filepath.Join(lexRoot, "fakeroot")
	t.Setenv(FakeRootEnv, fakeRoot)
	cfg := conf.Config{Prefer: "binary", AllowNonRoot: true,
		Repos: []conf.Repo{{Name: "local", Include: "pkg/x86_64/"}}}

	if err := Install(lexRoot, cfg, "demo", ""); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(fakeRoot, "usr", "bin", "demo"))
	if err != nil || string(got) != "demo-bin" {
		t.Fatalf("installed file=%q err=%v", got, err)
	}
	ins, err := db.Load(lexRoot, "demo")
	if err != nil {
		t.Fatalf("db.Load: %v", err)
	}
	if ins.Version != "1.0" || ins.Method != "binary" {
		t.Fatalf("manifest=%+v", ins)
	}
	found := false
	for _, f := range ins.Files {
		if f == "/usr/bin/demo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("files=%v", ins.Files)
	}
	// Outside fake root: nothing leaked.
	if _, err := os.Stat("/usr/bin/demo"); !os.IsNotExist(err) {
		t.Fatal("leaked onto real system")
	}
}

func TestSourceRoot(t *testing.T) {
	base := t.TempDir()
	single := filepath.Join(base, "single")
	os.MkdirAll(filepath.Join(single, "pkg-1"), 0o755)
	got, err := sourceRoot(single)
	if err != nil || got != filepath.Join(single, "pkg-1") {
		t.Fatalf("got %q err=%v", got, err)
	}
	flat := filepath.Join(base, "flat")
	os.MkdirAll(flat, 0o755)
	os.WriteFile(filepath.Join(flat, "a"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(flat, "b"), []byte("b"), 0o644)
	got, err = sourceRoot(flat)
	if err != nil || got != flat {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestBuildEnv(t *testing.T) {
	if got := buildEnv(conf.Config{Jobs: 8}); len(got) != 1 || got[0] != "MAKEFLAGS=-j8" {
		t.Fatalf("got %v", got)
	}
	if got := buildEnv(conf.Config{}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

// payload/greet-1.0 tree as a tarball
func makeCSource(t *testing.T) []byte {
	t.Helper()
	work := t.TempDir()
	src := filepath.Join(work, "payload", "greet-1.0")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "greeting.c"),
		[]byte("#include <stdio.h>\nint main(){printf(\"hello unpatched\\n\");return 0;}\n"), 0o644)
	os.WriteFile(filepath.Join(src, "Makefile"), []byte(
		"CC=gcc\nall: greeting\ngreeting: greeting.c\n\t$(CC) -o greeting greeting.c\ninstall:\n\tmkdir -p $(DESTDIR)/usr/bin\n\tcp -a greeting $(DESTDIR)/usr/bin/greeting\n"), 0o644)
	os.WriteFile(filepath.Join(src, "README"), []byte("greet readme\n"), 0o644)
	tarball := filepath.Join(work, "greet.tar.gz")
	cmd := exec.Command("tar", "-czf", tarball, "-C", filepath.Join(work, "payload"), ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v: %s", err, out)
	}
	b, err := os.ReadFile(tarball)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func requireTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not in PATH", tool)
		}
	}
}

// compiles C, applies both patch kinds, installs to a fake root
func TestInstallSourceEndToEnd(t *testing.T) {
	requireTools(t, "tar", "gcc", "make", "patch")
	tarBytes := makeCSource(t)
	sum := fmt.Sprintf("%x", sha256.Sum256(tarBytes))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarBytes)
	}))
	defer srv.Close()

	lexRoot := t.TempDir()
	recipeDir := filepath.Join(lexRoot, "pkg", "x86_64")
	os.MkdirAll(recipeDir, 0o755)
	// Repo-relative patch: changes program output.
	os.WriteFile(filepath.Join(recipeDir, "greet.patch"), []byte(
		"--- a/greeting.c\n+++ b/greeting.c\n@@ -1,2 +1,2 @@\n #include <stdio.h>\n-int main(){printf(\"hello unpatched\\n\");return 0;}\n+int main(){printf(\"hello patched\\n\");return 0;}\n"), 0o644)
	// file:// patch: touches an unrelated file.
	extraDir := t.TempDir()
	os.WriteFile(filepath.Join(extraDir, "extra.patch"), []byte(
		"--- a/README\n+++ b/README\n@@ -1 +1,2 @@\n greet readme\n+patched readme\n"), 0o644)
	recipeJSON := fmt.Sprintf(`{"kind":"recipe","name":"greet","version":"1.0",
"source":{"url":%q,"sha256":%q,
"patches":["greet.patch","file://%s/extra.patch"],
"build":["make"],"install":["make install DESTDIR=$DESTDIR"]}}`,
		srv.URL+"/greet.tar.gz", sum, extraDir)
	os.WriteFile(filepath.Join(recipeDir, "greet.lexbuild"), []byte(recipeJSON), 0o644)

	fakeRoot := filepath.Join(lexRoot, "fakeroot")
	t.Setenv(FakeRootEnv, fakeRoot)
	cfg := conf.Config{Prefer: "source", Jobs: 2, AllowNonRoot: true,
		Repos: []conf.Repo{{Name: "local", Include: "pkg/x86_64/"}}}
	if err := Install(lexRoot, cfg, "greet", ""); err != nil {
		t.Fatalf("Install: %v", err)
	}
	bin := filepath.Join(fakeRoot, "usr", "bin", "greeting")
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run installed binary: %v", err)
	}
	if string(out) != "hello patched\n" {
		t.Fatalf("output=%q (patch not applied?)", out)
	}
	ins, err := db.Load(lexRoot, "greet")
	if err != nil {
		t.Fatalf("db.Load: %v", err)
	}
	if ins.Method != "source" || ins.Version != "1.0" {
		t.Fatalf("manifest=%+v", ins)
	}
}

// failing build leaves no db entry
func TestInstallSourceBuildFailure(t *testing.T) {
	requireTools(t, "tar", "patch")
	tarBytes := makeCSource(t)
	sum := fmt.Sprintf("%x", sha256.Sum256(tarBytes))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarBytes)
	}))
	defer srv.Close()

	lexRoot := t.TempDir()
	recipeDir := filepath.Join(lexRoot, "pkg", "x86_64")
	os.MkdirAll(recipeDir, 0o755)
	recipeJSON := fmt.Sprintf(`{"kind":"recipe","name":"greetfail","version":"1.0",
"source":{"url":%q,"sha256":%q,"build":["exit 1"],"install":["echo no"]}}`,
		srv.URL+"/greet.tar.gz", sum)
	os.WriteFile(filepath.Join(recipeDir, "greetfail.lexbuild"), []byte(recipeJSON), 0o644)
	cfg := conf.Config{Prefer: "source", AllowNonRoot: true,
		Repos: []conf.Repo{{Name: "local", Include: "pkg/x86_64/"}}}
	if err := Install(lexRoot, cfg, "greetfail", ""); err == nil {
		t.Fatal("expected build error")
	}
	if _, err := db.Load(lexRoot, "greetfail"); !os.IsNotExist(err) {
		t.Fatalf("db entry should not exist: %v", err)
	}
}

func TestInstallBadName(t *testing.T) {
	if err := Install(t.TempDir(), conf.Config{}, "../../x", ""); err == nil {
		t.Fatal("expected name error")
	}
}
