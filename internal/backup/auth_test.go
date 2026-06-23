package backup_test

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/local-finance-manager/backend/internal/backup"
)

func TestLoadToken_Missing(t *testing.T) {
	tok, err := backup.LoadToken(filepath.Join(t.TempDir(), "token.json"))
	if err != nil {
		t.Fatalf("arquivo ausente não deveria ser erro: %v", err)
	}
	if tok != nil {
		t.Errorf("esperava nil, got %+v", tok)
	}
}

func TestSaveLoadToken_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "token.json") // MkdirAll deve criar "sub"
	want := &oauth2.Token{AccessToken: "at", RefreshToken: "rt-123", TokenType: "Bearer"}
	if err := backup.SaveToken(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := backup.LoadToken(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil || got.RefreshToken != "rt-123" || got.AccessToken != "at" {
		t.Errorf("round-trip divergente: %+v", got)
	}
}

func TestResolveToken(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")

	// 1) sem token.json e sem env → nil
	tok, err := backup.ResolveToken(tokenPath, "")
	if err != nil || tok != nil {
		t.Errorf("sem nada: got tok=%v err=%v, want nil/nil", tok, err)
	}

	// 2) sem token.json mas com refresh do env → fallback
	tok, err = backup.ResolveToken(tokenPath, "env-refresh")
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	if tok == nil || tok.RefreshToken != "env-refresh" {
		t.Errorf("fallback env: got %+v", tok)
	}

	// 3) com token.json → prefere o arquivo
	backup.SaveToken(tokenPath, &oauth2.Token{RefreshToken: "file-refresh"})
	tok, err = backup.ResolveToken(tokenPath, "env-refresh")
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if tok == nil || tok.RefreshToken != "file-refresh" {
		t.Errorf("deveria preferir o token.json: got %+v", tok)
	}
}

func TestOAuthConfig(t *testing.T) {
	conf := backup.OAuthConfig("cid", "secret", "http://localhost/cb")
	if conf.ClientID != "cid" || conf.ClientSecret != "secret" || conf.RedirectURL != "http://localhost/cb" {
		t.Errorf("config inesperada: %+v", conf)
	}
	if len(conf.Scopes) != 1 || !strings.Contains(conf.Scopes[0], "drive.file") {
		t.Errorf("escopo deveria ser drive.file (mínimo privilégio), got %v", conf.Scopes)
	}
}

func TestLoopbackRedirectURL(t *testing.T) {
	if !strings.HasPrefix(backup.LoopbackRedirectURL(), "http://127.0.0.1:") {
		t.Errorf("redirect loopback inesperado: %q", backup.LoopbackRedirectURL())
	}
}
