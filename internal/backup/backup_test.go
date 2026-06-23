package backup_test

import (
	"testing"
	"time"

	"github.com/local-finance-manager/backend/internal/backup"
)

func TestShouldUpload(t *testing.T) {
	if backup.ShouldUpload("abc", "abc") {
		t.Error("checksums iguais → não deveria subir")
	}
	if !backup.ShouldUpload("abc", "def") {
		t.Error("checksums diferentes → deveria subir")
	}
	if !backup.ShouldUpload("abc", "") {
		t.Error("sem backup anterior → deveria subir")
	}
}

func TestVersionsToPrune(t *testing.T) {
	mk := func(name string, day int) backup.Version {
		return backup.Version{
			FileID:    name,
			Name:      name,
			CreatedAt: time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC),
		}
	}
	// fora de ordem de propósito
	versions := []backup.Version{mk("v1", 1), mk("v3", 3), mk("v5", 5), mk("v2", 2), mk("v4", 4)}

	cases := []struct {
		name      string
		retention int
		wantIDs   []string // esperados na poda
	}{
		{"retention 3 poda as 2 mais antigas", 3, []string{"v2", "v1"}},
		{"retention >= total poda nada", 5, nil},
		{"retention maior que total poda nada", 10, nil},
		{"retention 0 poda todas", 0, []string{"v5", "v4", "v3", "v2", "v1"}},
		{"retention negativa poda todas", -1, []string{"v5", "v4", "v3", "v2", "v1"}},
		{"retention 1 mantém só a mais nova", 1, []string{"v4", "v3", "v2", "v1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := backup.VersionsToPrune(versions, tc.retention)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("len: got %d %v, want %d %v", len(got), ids(got), len(tc.wantIDs), tc.wantIDs)
			}
			for i, id := range tc.wantIDs {
				if got[i].FileID != id {
					t.Errorf("pos %d: got %s, want %s", i, got[i].FileID, id)
				}
			}
		})
	}
}

func TestVersionsToPrune_Empty(t *testing.T) {
	if got := backup.VersionsToPrune(nil, 30); got != nil {
		t.Errorf("lista vazia deveria devolver nil, got %v", got)
	}
}

func TestIsRemoteNewer(t *testing.T) {
	base := time.Date(2026, 6, 12, 14, 0, 0, 0, time.UTC)
	skew := 5 * time.Second
	cases := []struct {
		name          string
		remote, local time.Time
		want          bool
	}{
		{"remoto bem depois", base.Add(time.Hour), base, true},
		{"remoto dentro do skew", base.Add(3 * time.Second), base, false},
		{"iguais", base, base, false},
		{"remoto antes", base, base.Add(time.Hour), false},
		{"remoto exatamente no limite do skew", base.Add(5 * time.Second), base, false},
		{"remoto 1s além do skew", base.Add(6 * time.Second), base, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := backup.IsRemoteNewer(tc.remote, tc.local, skew); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDatedFilename(t *testing.T) {
	ts := time.Date(2026, 6, 12, 14, 30, 0, 0, time.UTC)
	got := backup.DatedFilename("financas", ts)
	want := "financas-2026-06-12T14-30-00Z.sqlite"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// não pode conter ':' (inválido em nome de arquivo)
	for _, r := range got {
		if r == ':' {
			t.Fatalf("nome contém ':' inválido: %q", got)
		}
	}
}

func ids(vs []backup.Version) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.FileID
	}
	return out
}
