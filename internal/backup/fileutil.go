package backup

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/url"
	"os"
	"strings"

	_ "modernc.org/sqlite" // driver p/ integrityCheck do arquivo restaurado
)

// hashFileBoth calcula SHA-256 e MD5 (hex) e o tamanho de um arquivo numa única passada.
// SHA-256 alimenta o no-op (RF-BKP-03); MD5 confere com o md5Checksum do Drive (RF-BKP-05).
func hashFileBoth(path string) (sha256hex, md5hex string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", 0, fmt.Errorf("backup: open for hash: %w", err)
	}
	defer f.Close()

	sh := sha256.New()
	mh := md5.New()
	n, err := io.Copy(io.MultiWriter(sh, mh), f)
	if err != nil {
		return "", "", 0, fmt.Errorf("backup: hash: %w", err)
	}
	return hex.EncodeToString(sh.Sum(nil)), hex.EncodeToString(mh.Sum(nil)), n, nil
}

// copyFile copia src→dst (usado p/ backup local de segurança no boot, quando o banco
// não está aberto). 0600 para não vazar dados.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("backup: copy open src: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("backup: copy open dst: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("backup: copy: %w", err)
	}
	return out.Close()
}

// writeAndHashSHA grava r em dst e devolve o SHA-256 (hex) e o tamanho.
func writeAndHashSHA(dst string, r io.Reader) (sha256hex string, size int64, err error) {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("backup: open dst: %w", err)
	}
	var h hash.Hash = sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), r)
	if err != nil {
		out.Close()
		return "", 0, fmt.Errorf("backup: write: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", 0, fmt.Errorf("backup: close dst: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// integrityCheck abre o arquivo SQLite com uma conexão descartável e roda
// PRAGMA integrity_check, garantindo que um arquivo baixado é um banco íntegro
// antes de promovê-lo (RNF-BKP-06).
func integrityCheck(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("backup: integrity open: %w", err)
	}
	defer db.Close()

	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("backup: integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup: integrity check failed: %s", result)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isOffline classifica um erro como falta de conectividade (RNF-BKP-07): vira estado
// offline tratado, nunca panic/500 silencioso.
func isOffline(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	s := err.Error()
	for _, frag := range []string{"no such host", "connection refused", "x509", "tls:", "dial tcp", "network is unreachable", "i/o timeout"} {
		if strings.Contains(s, frag) {
			return true
		}
	}
	return false
}
