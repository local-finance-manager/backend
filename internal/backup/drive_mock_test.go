package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// newMockDriveClient constrói um GoogleDriveClient apontando para um httptest.Server,
// permitindo cobrir os wrappers da SDK do Drive sem chamar a API real.
func newMockDriveClient(t *testing.T, h http.HandlerFunc) *GoogleDriveClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	svc, err := drive.NewService(context.Background(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new drive service: %v", err)
	}
	return &GoogleDriveClient{svc: svc}
}

// driveHandler responde às operações do Drive: list (GET), download (GET alt=media),
// create (POST), update (PATCH), delete (DELETE).
func driveHandler(listFiles []map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("alt") == "media":
			io.WriteString(w, "conteudo-do-arquivo")
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"files": listFiles})
		case r.Method == http.MethodPost, r.Method == http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id": "new-id", "name": "financas-latest.sqlite", "size": "19",
				"md5Checksum": "abc123", "modifiedTime": "2026-06-20T10:00:00Z",
				"createdTime": "2026-06-19T09:00:00Z",
			})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func TestGoogleDriveClient_EnsureFolder_Create(t *testing.T) {
	c := newMockDriveClient(t, driveHandler(nil)) // list vazia → cria
	id, err := c.EnsureFolder(context.Background(), "Financas")
	if err != nil || id != "new-id" {
		t.Fatalf("EnsureFolder create: id=%q err=%v", id, err)
	}
}

func TestGoogleDriveClient_EnsureFolder_Existing(t *testing.T) {
	c := newMockDriveClient(t, driveHandler([]map[string]any{{"id": "folder-1", "name": "Financas"}}))
	id, err := c.EnsureFolder(context.Background(), "Financas")
	if err != nil || id != "folder-1" {
		t.Fatalf("EnsureFolder existing: id=%q err=%v", id, err)
	}
}

func TestGoogleDriveClient_UploadAndUpdate(t *testing.T) {
	c := newMockDriveClient(t, driveHandler(nil))
	ctx := context.Background()

	up, err := c.UploadNew(ctx, "folder-1", "financas.sqlite", bytes.NewReader([]byte("dados")))
	if err != nil || up.ID != "new-id" {
		t.Fatalf("UploadNew: %+v err=%v", up, err)
	}
	upd, err := c.UpdateContents(ctx, "new-id", bytes.NewReader([]byte("novos-dados")))
	if err != nil || upd.MD5Checksum != "abc123" {
		t.Fatalf("UpdateContents: %+v err=%v", upd, err)
	}
}

func TestGoogleDriveClient_FindByName(t *testing.T) {
	ctx := context.Background()
	// encontrado
	c := newMockDriveClient(t, driveHandler([]map[string]any{
		{"id": "f1", "name": "financas-latest.sqlite", "size": "10", "md5Checksum": "z"},
	}))
	got, err := c.FindByName(ctx, "folder-1", "financas-latest.sqlite")
	if err != nil || got == nil || got.ID != "f1" {
		t.Fatalf("FindByName encontrado: %+v err=%v", got, err)
	}
	// não encontrado → nil
	empty := newMockDriveClient(t, driveHandler(nil))
	got, err = empty.FindByName(ctx, "folder-1", "x")
	if err != nil || got != nil {
		t.Fatalf("FindByName vazio deveria ser nil: %+v err=%v", got, err)
	}
}

func TestGoogleDriveClient_List(t *testing.T) {
	c := newMockDriveClient(t, driveHandler([]map[string]any{
		{"id": "a", "name": "v1", "size": "1", "createdTime": "2026-06-01T00:00:00Z"},
		{"id": "b", "name": "v2", "size": "2", "createdTime": "2026-06-02T00:00:00Z"},
	}))
	files, err := c.List(context.Background(), "folder-1")
	if err != nil || len(files) != 2 {
		t.Fatalf("List: got %d err=%v", len(files), err)
	}
}

func TestGoogleDriveClient_Download(t *testing.T) {
	c := newMockDriveClient(t, driveHandler(nil))
	rc, err := c.Download(context.Background(), "f1")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "conteudo-do-arquivo" {
		t.Errorf("Download conteúdo: %q", string(data))
	}
}

func TestGoogleDriveClient_Delete(t *testing.T) {
	c := newMockDriveClient(t, driveHandler(nil))
	if err := c.Delete(context.Background(), "f1"); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

// TestGoogleDriveClient_Errors cobre os ramos de erro (driveErr) de todos os wrappers:
// o servidor responde 500 a tudo.
func TestGoogleDriveClient_Errors(t *testing.T) {
	c := newMockDriveClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":500,"message":"boom"}}`, http.StatusInternalServerError)
	})
	ctx := context.Background()
	if _, err := c.EnsureFolder(ctx, "F"); err == nil {
		t.Error("EnsureFolder deveria falhar")
	}
	if _, err := c.UploadNew(ctx, "f", "n", bytes.NewReader([]byte("x"))); err == nil {
		t.Error("UploadNew deveria falhar")
	}
	if _, err := c.UpdateContents(ctx, "id", bytes.NewReader([]byte("x"))); err == nil {
		t.Error("UpdateContents deveria falhar")
	}
	if _, err := c.FindByName(ctx, "f", "n"); err == nil {
		t.Error("FindByName deveria falhar")
	}
	if _, err := c.List(ctx, "f"); err == nil {
		t.Error("List deveria falhar")
	}
	if _, err := c.Download(ctx, "id"); err == nil {
		t.Error("Download deveria falhar")
	}
	if err := c.Delete(ctx, "id"); err == nil {
		t.Error("Delete deveria falhar")
	}
}

func TestGoogleDriveClient_EnsureFolder_CreateError(t *testing.T) {
	c := newMockDriveClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"files": []any{}})
			return
		}
		http.Error(w, `{"error":{"code":500}}`, http.StatusInternalServerError)
	})
	if _, err := c.EnsureFolder(context.Background(), "F"); err == nil {
		t.Error("EnsureFolder deveria falhar quando o create falha")
	}
}

// TestNewGoogleDriveClient cobre o construtor (o TokenSource é lazy → sem rede aqui).
func TestNewGoogleDriveClient(t *testing.T) {
	c, err := NewGoogleDriveClient(context.Background(), &oauth2.Config{}, &oauth2.Token{AccessToken: "x", RefreshToken: "y"})
	if err != nil {
		t.Fatalf("NewGoogleDriveClient: %v", err)
	}
	if c == nil {
		t.Fatal("client não deveria ser nil")
	}
}
