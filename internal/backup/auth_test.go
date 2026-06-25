package backup_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/local-finance-manager/backend/internal/backup"
)

type nopPrinter struct{}

func (nopPrinter) Println(...any) {}

// capturePrinter guarda as linhas impressas (thread-safe p/ o -race).
type capturePrinter struct {
	mu    sync.Mutex
	lines []string
}

func (p *capturePrinter) Println(a ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lines = append(p.lines, fmt.Sprint(a...))
}

func (p *capturePrinter) authURL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, l := range p.lines {
		if strings.Contains(l, "state=") {
			return l
		}
	}
	return ""
}

const authCallbackURL = "http://127.0.0.1:19743/oauth/callback"

// TestAuthorize_ContextCancelled cobre o setup do fluxo OAuth + o ramo de contexto cancelado.
func TestAuthorize_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conf := &oauth2.Config{RedirectURL: authCallbackURL, Endpoint: oauth2.Endpoint{AuthURL: "http://auth.example"}}
	if _, err := backup.Authorize(ctx, conf, nopPrinter{}); err == nil {
		t.Error("esperava erro com contexto cancelado")
	}
}

// TestAuthorize_HappyPath cobre o fluxo completo: captura o state do authURL impresso,
// dispara um callback válido com code e troca por token num endpoint mockado.
func TestAuthorize_HappyPath(t *testing.T) {
	tokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokSrv.Close()

	conf := &oauth2.Config{
		ClientID: "id", ClientSecret: "sec", RedirectURL: authCallbackURL,
		Endpoint: oauth2.Endpoint{AuthURL: "http://auth.example", TokenURL: tokSrv.URL},
	}
	pr := &capturePrinter{}
	type result struct {
		tok *oauth2.Token
		err error
	}
	done := make(chan result, 1)
	go func() {
		tok, err := backup.Authorize(context.Background(), conf, pr)
		done <- result{tok, err}
	}()

	// captura o state do authURL impresso
	var state string
	for i := 0; i < 100 && state == ""; i++ {
		if u, err := url.Parse(pr.authURL()); err == nil {
			state = u.Query().Get("state")
		}
		if state == "" {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if state == "" {
		t.Fatal("não capturou o state do authURL")
	}

	// dispara o callback válido (state correto + code)
	var ok bool
	for i := 0; i < 100; i++ {
		resp, err := http.Get(authCallbackURL + "?state=" + state + "&code=fake-code")
		if err == nil {
			resp.Body.Close()
			ok = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		t.Fatal("callback não respondeu")
	}

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("Authorize: %v", res.err)
		}
		if res.tok == nil || res.tok.RefreshToken != "rt" {
			t.Errorf("token inesperado: %+v", res.tok)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Authorize não retornou após o callback válido")
	}
}

// TestAuthorize_ListenError cobre o ramo de erro quando a porta de loopback está ocupada.
func TestAuthorize_ListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:19743")
	if err != nil {
		t.Skip("porta de loopback já ocupada")
	}
	defer ln.Close()
	conf := &oauth2.Config{RedirectURL: authCallbackURL, Endpoint: oauth2.Endpoint{AuthURL: "http://auth.example"}}
	if _, err := backup.Authorize(context.Background(), conf, nopPrinter{}); err == nil {
		t.Error("esperava erro de listen com a porta ocupada")
	}
}

// TestAuthorize_StateMismatch cobre o handler de callback (state inválido) + o ramo errCh.
func TestAuthorize_StateMismatch(t *testing.T) {
	conf := &oauth2.Config{RedirectURL: authCallbackURL, Endpoint: oauth2.Endpoint{AuthURL: "http://auth.example"}}
	done := make(chan error, 1)
	go func() {
		_, err := backup.Authorize(context.Background(), conf, nopPrinter{})
		done <- err
	}()
	// aguarda o servidor de loopback subir e dispara um callback com state inválido
	var ok bool
	for i := 0; i < 100; i++ {
		resp, err := http.Get(authCallbackURL + "?state=errado")
		if err == nil {
			resp.Body.Close()
			ok = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		t.Fatal("servidor de callback não respondeu")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("esperava erro de state inválido")
		}
	case <-time.After(3 * time.Second):
		t.Error("Authorize não retornou após o callback")
	}
}

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
