package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const folderMimeType = "application/vnd.google-apps.folder"

// driveFields são os campos que pedimos ao Drive em cada arquivo.
const driveFields = "id,name,size,md5Checksum,modifiedTime,createdTime"

var _ DriveClient = (*GoogleDriveClient)(nil)

// GoogleDriveClient implementa DriveClient com a SDK oficial do Google.
type GoogleDriveClient struct{ svc *drive.Service }

// NewGoogleDriveClient cria o cliente a partir do OAuth config + token. O TokenSource
// renova o access token automaticamente usando o refresh token (sem intervenção — D17).
func NewGoogleDriveClient(ctx context.Context, conf *oauth2.Config, tok *oauth2.Token) (*GoogleDriveClient, error) {
	ts := conf.TokenSource(ctx, tok)
	svc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("backup drive: new service: %w", err)
	}
	return &GoogleDriveClient{svc: svc}, nil
}

// EnsureFolder retorna o id da pasta `name` (cria se não existir). Em drive.file, só
// enxerga pastas que o próprio app criou.
func (c *GoogleDriveClient) EnsureFolder(ctx context.Context, name string) (string, error) {
	q := fmt.Sprintf("mimeType='%s' and name='%s' and trashed=false", folderMimeType, escapeQuery(name))
	res, err := c.svc.Files.List().Q(q).Spaces("drive").Fields("files(id,name)").Context(ctx).Do()
	if err != nil {
		return "", driveErr("list folder", err)
	}
	if len(res.Files) > 0 {
		return res.Files[0].Id, nil
	}
	created, err := c.svc.Files.Create(&drive.File{Name: name, MimeType: folderMimeType}).
		Fields("id").Context(ctx).Do()
	if err != nil {
		return "", driveErr("create folder", err)
	}
	return created.Id, nil
}

// UploadNew cria um novo arquivo com o conteúdo de r dentro de folderID.
func (c *GoogleDriveClient) UploadNew(ctx context.Context, folderID, name string, r io.Reader) (DriveFile, error) {
	f, err := c.svc.Files.Create(&drive.File{Name: name, Parents: []string{folderID}}).
		Media(r).Fields(driveFields).Context(ctx).Do()
	if err != nil {
		return DriveFile{}, driveErr("upload new", err)
	}
	return toDriveFile(f), nil
}

// UpdateContents sobrescreve o conteúdo de um arquivo existente (a "latest").
func (c *GoogleDriveClient) UpdateContents(ctx context.Context, fileID string, r io.Reader) (DriveFile, error) {
	f, err := c.svc.Files.Update(fileID, &drive.File{}).
		Media(r).Fields(driveFields).Context(ctx).Do()
	if err != nil {
		return DriveFile{}, driveErr("update contents", err)
	}
	return toDriveFile(f), nil
}

// FindByName procura um arquivo por nome dentro da pasta; nil se não existe.
func (c *GoogleDriveClient) FindByName(ctx context.Context, folderID, name string) (*DriveFile, error) {
	q := fmt.Sprintf("'%s' in parents and name='%s' and trashed=false", escapeQuery(folderID), escapeQuery(name))
	res, err := c.svc.Files.List().Q(q).Spaces("drive").Fields("files(" + driveFields + ")").Context(ctx).Do()
	if err != nil {
		return nil, driveErr("find by name", err)
	}
	if len(res.Files) == 0 {
		return nil, nil
	}
	df := toDriveFile(res.Files[0])
	return &df, nil
}

// List retorna todos os arquivos da pasta (segue paginação do Drive).
func (c *GoogleDriveClient) List(ctx context.Context, folderID string) ([]DriveFile, error) {
	q := fmt.Sprintf("'%s' in parents and trashed=false", escapeQuery(folderID))
	var out []DriveFile
	pageToken := ""
	for {
		call := c.svc.Files.List().Q(q).Spaces("drive").
			OrderBy("createdTime desc").Fields("nextPageToken, files(" + driveFields + ")").Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		res, err := call.Do()
		if err != nil {
			return nil, driveErr("list", err)
		}
		for _, f := range res.Files {
			out = append(out, toDriveFile(f))
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return out, nil
}

// Download abre o conteúdo de um arquivo. O chamador fecha o ReadCloser.
func (c *GoogleDriveClient) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	resp, err := c.svc.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return nil, driveErr("download", err)
	}
	return resp.Body, nil
}

// Delete remove um arquivo (poda de retenção).
func (c *GoogleDriveClient) Delete(ctx context.Context, fileID string) error {
	if err := c.svc.Files.Delete(fileID).Context(ctx).Do(); err != nil {
		return driveErr("delete", err)
	}
	return nil
}

func toDriveFile(f *drive.File) DriveFile {
	df := DriveFile{ID: f.Id, Name: f.Name, Size: f.Size, MD5Checksum: f.Md5Checksum}
	if t, err := time.Parse(time.RFC3339, f.ModifiedTime); err == nil {
		df.ModifiedTime = t
	}
	if t, err := time.Parse(time.RFC3339, f.CreatedTime); err == nil {
		df.CreatedTime = t
	}
	return df
}

// escapeQuery protege aspas simples em valores interpolados na query do Drive.
func escapeQuery(s string) string {
	return strings.ReplaceAll(s, "'", `\'`)
}

// driveErr traduz erros da API do Google em erros úteis. Erros de auth/permissão/config
// (401/403 — ex.: API do Drive desabilitada no projeto) viram erro de domínio displayable,
// em vez de um 500 genérico. A tradução vive no adapter (mantém o isolamento: o serviço
// não importa a SDK do Google).
func driveErr(stage string, err error) error {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && (gerr.Code == 401 || gerr.Code == 403) {
		return domainerr.NewForbidden(
			fmt.Sprintf("Google Drive recusou o acesso (HTTP %d): verifique se a API do Google Drive está habilitada no projeto e se as permissões do OAuth estão corretas", gerr.Code),
			domainerr.WithDisplayable(),
		)
	}
	return fmt.Errorf("backup drive: %s: %w", stage, err)
}
