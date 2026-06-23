package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

// loopbackPort é a porta local usada pelo fluxo de autorização único (D17).
const loopbackPort = 19743

// OAuthConfig monta o oauth2.Config com escopo mínimo drive.file (RNF-BKP-04).
// redirectURL vazio = uso runtime (sem redirect); preenchido = fluxo authorize.
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{drive.DriveFileScope},
	}
}

// LoadToken lê o token OAuth do token.json. Arquivo ausente → (nil, nil).
func LoadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("backup auth: read token: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("backup auth: unmarshal token: %w", err)
	}
	return &tok, nil
}

// SaveToken grava o token OAuth de forma atômica (0600 — contém o refresh token).
func SaveToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("backup auth: mkdir token dir: %w", err)
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("backup auth: marshal token: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("backup auth: write token: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("backup auth: rename token: %w", err)
	}
	return nil
}

// ResolveToken decide qual token usar no runtime: prefere o token.json; se ausente,
// faz fallback para o refresh token do .env (compat). nil se nenhum disponível.
func ResolveToken(tokenPath, refreshTokenEnv string) (*oauth2.Token, error) {
	tok, err := LoadToken(tokenPath)
	if err != nil {
		return nil, err
	}
	if tok != nil && (tok.RefreshToken != "" || tok.AccessToken != "") {
		return tok, nil
	}
	if refreshTokenEnv != "" {
		return &oauth2.Token{RefreshToken: refreshTokenEnv}, nil
	}
	return nil, nil
}

// Authorize roda o fluxo OAuth loopback uma única vez (no host, com browser): imprime
// a URL de consentimento, sobe um servidor local em loopbackPort para capturar o code,
// troca por um token (com refresh token) e o devolve. O chamador persiste com SaveToken.
func Authorize(ctx context.Context, conf *oauth2.Config, out interface{ Println(...any) }) (*oauth2.Token, error) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("backup auth: state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	out.Println("Abra esta URL no navegador para autorizar o backup no Google Drive:")
	out.Println("")
	out.Println(authURL)
	out.Println("")
	out.Println("Aguardando autorização em " + conf.RedirectURL + " ...")

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state inválido", http.StatusBadRequest)
			errCh <- errors.New("backup auth: state mismatch")
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			http.Error(w, "autorização negada", http.StatusBadRequest)
			errCh <- fmt.Errorf("backup auth: %s", e)
			return
		}
		code := r.URL.Query().Get("code")
		_, _ = w.Write([]byte("Autorização concluída. Você já pode fechar esta aba e voltar ao terminal."))
		codeCh <- code
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", loopbackPort))
	if err != nil {
		return nil, fmt.Errorf("backup auth: listen loopback: %w", err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()
	defer srv.Close()

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, errors.New("backup auth: tempo de autorização esgotado")
	}

	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("backup auth: exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, errors.New("backup auth: Google não retornou refresh token (revogue o acesso e tente de novo)")
	}
	return tok, nil
}

// LoopbackRedirectURL é a URL de callback usada no fluxo authorize.
func LoopbackRedirectURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/oauth/callback", loopbackPort)
}
