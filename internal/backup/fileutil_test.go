package backup

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestHashFileBoth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	sha, md5hex, size, err := hashFileBoth(path)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if sha != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("sha256 errado: %s", sha)
	}
	if md5hex != "900150983cd24fb0d6963f7d28e17f72" {
		t.Errorf("md5 errado: %s", md5hex)
	}
	if size != 3 {
		t.Errorf("size: got %d, want 3", size)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.WriteFile(src, []byte("hello"), 0o600)
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "hello" {
		t.Errorf("got %q", string(got))
	}
}

func TestWriteAndHashSHA(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	sha, size, err := writeAndHashSHA(dst, strings.NewReader("abc"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if sha != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" || size != 3 {
		t.Errorf("got sha=%s size=%d", sha, size)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "abc" {
		t.Errorf("conteúdo gravado errado: %q", string(got))
	}
}

func TestIntegrityCheck(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.sqlite")
	db, err := sql.Open("sqlite", valid)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE t(x)")
	db.Close()

	if err := integrityCheck(context.Background(), valid); err != nil {
		t.Errorf("banco válido deveria passar: %v", err)
	}

	garbage := filepath.Join(dir, "garbage.sqlite")
	os.WriteFile(garbage, []byte("not a sqlite file at all"), 0o600)
	if err := integrityCheck(context.Background(), garbage); err == nil {
		t.Error("arquivo inválido deveria falhar no integrity_check")
	}
}

func TestFileExists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	if fileExists(p) {
		t.Error("não deveria existir")
	}
	os.WriteFile(p, []byte("y"), 0o600)
	if !fileExists(p) {
		t.Error("deveria existir")
	}
}

func TestIsOffline(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", context.DeadlineExceeded, true},
		{"url error dial", &url.Error{Op: "Get", URL: "https://x", Err: errors.New("dial tcp: connection refused")}, true},
		{"connection refused string", errors.New("dial tcp 1.2.3.4:443: connect: connection refused"), true},
		{"x509", errors.New("x509: certificate signed by unknown authority"), true},
		{"no such host", errors.New("lookup googleapis.com: no such host"), true},
		{"erro de negócio comum", errors.New("file not found"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOffline(tc.err); got != tc.want {
				t.Errorf("isOffline(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
