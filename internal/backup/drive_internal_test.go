package backup

import (
	"errors"
	"strings"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
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
