package backup

import (
	"errors"
	"strings"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

func TestDriveErr_GoogleForbiddenIsDisplayableDomain(t *testing.T) {
	err := driveErr("list folder", &googleapi.Error{Code: 403, Message: "API disabled"})
	if _, ok := domainerr.IsDomain(err); !ok {
		t.Fatalf("403 do Google deveria virar erro de domínio displayable, got %T", err)
	}
	if !strings.Contains(err.Error(), "Google Drive recusou o acesso") {
		t.Errorf("mensagem inesperada: %q", err.Error())
	}
}

func TestDriveErr_GoogleUnauthorized(t *testing.T) {
	err := driveErr("download", &googleapi.Error{Code: 401})
	if _, ok := domainerr.IsDomain(err); !ok {
		t.Error("401 do Google deveria virar erro de domínio")
	}
}

func TestDriveErr_GenericStaysInfra(t *testing.T) {
	err := driveErr("list", errors.New("boom"))
	if _, ok := domainerr.IsDomain(err); ok {
		t.Error("erro genérico não deveria ser tratado como erro de domínio (vira 500)")
	}
}

func TestDriveErr_Google500StaysInfra(t *testing.T) {
	// 5xx do Google não é erro de configuração do usuário → infra (500)
	err := driveErr("upload", &googleapi.Error{Code: 500})
	if _, ok := domainerr.IsDomain(err); ok {
		t.Error("500 do Google não deveria virar erro de domínio displayable")
	}
}

func TestEscapeQuery(t *testing.T) {
	if got := escapeQuery("a'b'c"); got != `a\'b\'c` {
		t.Errorf("escapeQuery: got %q", got)
	}
	if got := escapeQuery("sem aspas"); got != "sem aspas" {
		t.Errorf("escapeQuery sem aspas: got %q", got)
	}
}

func TestToDriveFile(t *testing.T) {
	df := toDriveFile(&drive.File{
		Id: "f1", Name: "financas.sqlite", Size: 1024, Md5Checksum: "abc",
		ModifiedTime: "2026-06-20T10:00:00Z", CreatedTime: "2026-06-19T09:00:00Z",
	})
	if df.ID != "f1" || df.Name != "financas.sqlite" || df.Size != 1024 || df.MD5Checksum != "abc" {
		t.Errorf("toDriveFile campos: %+v", df)
	}
	if df.ModifiedTime.IsZero() || df.CreatedTime.IsZero() {
		t.Errorf("toDriveFile datas não parseadas: %+v", df)
	}
	// datas inválidas → zero (sem panic)
	bad := toDriveFile(&drive.File{Id: "x", ModifiedTime: "nao-data"})
	if !bad.ModifiedTime.IsZero() {
		t.Errorf("data inválida deveria ficar zero")
	}
}

func TestDriveCallErr(t *testing.T) {
	if err := driveCallErr("stage", ErrDriveOffline); err != ErrDriveOffline {
		t.Errorf("offline deveria mapear para ErrDriveOffline, got %v", err)
	}
	dom := domainerr.NewConflict("conflito")
	if err := driveCallErr("stage", dom); err != dom {
		t.Errorf("erro de domínio deveria passar direto, got %v", err)
	}
	generic := errors.New("falha genérica")
	wrapped := driveCallErr("upload", generic)
	if wrapped == nil || !errors.Is(wrapped, generic) {
		t.Errorf("erro genérico deveria ser embrulhado, got %v", wrapped)
	}
}
