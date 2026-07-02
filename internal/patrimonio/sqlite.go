package patrimonio

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SQLiteRepository implementa Repository (owner da tabela `caixinhas`).
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository cria o repositório.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

const caixinhaCols = `id, name, type, meta_valor, data_alvo, valor_mercado, data_valor_mercado,
	color, icon, display_order, archived, created_at, updated_at`

func scanCaixinha(s interface{ Scan(...any) error }) (Caixinha, error) {
	var c Caixinha
	var metaValor, valorMercado sql.NullInt64
	var dataAlvo, dataVM, color, icon sql.NullString
	var archived int
	var createdAt, updatedAt string
	err := s.Scan(
		&c.ID, &c.Name, (*string)(&c.Type), &metaValor, &dataAlvo, &valorMercado, &dataVM,
		&color, &icon, &c.DisplayOrder, &archived, &createdAt, &updatedAt,
	)
	if err != nil {
		return Caixinha{}, err
	}
	if metaValor.Valid {
		c.MetaValor = &metaValor.Int64
	}
	if valorMercado.Valid {
		c.ValorMercado = &valorMercado.Int64
	}
	if dataAlvo.Valid {
		c.DataAlvo = &dataAlvo.String
	}
	if dataVM.Valid {
		c.DataValorMercado = &dataVM.String
	}
	if color.Valid {
		c.Color = &color.String
	}
	if icon.Valid {
		c.Icon = &icon.String
	}
	c.Archived = archived != 0
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}

func (r *SQLiteRepository) Create(ctx context.Context, c Caixinha) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO caixinhas (`+caixinhaCols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, string(c.Type), int64PtrToNull(c.MetaValor), strPtrToNull(c.DataAlvo),
		int64PtrToNull(c.ValorMercado), strPtrToNull(c.DataValorMercado),
		strPtrToNull(c.Color), strPtrToNull(c.Icon), c.DisplayOrder, boolToInt(c.Archived),
		c.CreatedAt.UTC().Format(time.RFC3339), c.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("patrimonio sqlite: create: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id string) (Caixinha, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+caixinhaCols+" FROM caixinhas WHERE id = ?", id)
	c, err := scanCaixinha(row)
	if err == sql.ErrNoRows {
		return Caixinha{}, ErrCaixinhaNotFound
	}
	if err != nil {
		return Caixinha{}, fmt.Errorf("patrimonio sqlite: get: %w", err)
	}
	return c, nil
}

func (r *SQLiteRepository) List(ctx context.Context, includeArchived bool) ([]Caixinha, error) {
	q := "SELECT " + caixinhaCols + " FROM caixinhas"
	if !includeArchived {
		q += " WHERE archived = 0"
	}
	q += " ORDER BY display_order, created_at"
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("patrimonio sqlite: list: %w", err)
	}
	defer rows.Close()
	out := []Caixinha{}
	for rows.Next() {
		c, err := scanCaixinha(rows)
		if err != nil {
			return nil, fmt.Errorf("patrimonio sqlite: list scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) Update(ctx context.Context, c Caixinha) error {
	res, err := r.db.ExecContext(ctx, `UPDATE caixinhas SET
		name=?, type=?, meta_valor=?, data_alvo=?, valor_mercado=?, data_valor_mercado=?,
		color=?, icon=?, display_order=?, updated_at=? WHERE id=?`,
		c.Name, string(c.Type), int64PtrToNull(c.MetaValor), strPtrToNull(c.DataAlvo),
		int64PtrToNull(c.ValorMercado), strPtrToNull(c.DataValorMercado),
		strPtrToNull(c.Color), strPtrToNull(c.Icon), c.DisplayOrder,
		c.UpdatedAt.UTC().Format(time.RFC3339), c.ID,
	)
	if err != nil {
		return fmt.Errorf("patrimonio sqlite: update: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (r *SQLiteRepository) SetArchived(ctx context.Context, id string, archived bool) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE caixinhas SET archived=?, updated_at=? WHERE id=?",
		boolToInt(archived), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("patrimonio sqlite: set archived: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (r *SQLiteRepository) SetMarketValue(ctx context.Context, id string, valor int64, data string) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE caixinhas SET valor_mercado=?, data_valor_mercado=?, updated_at=? WHERE id=?",
		valor, data, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("patrimonio sqlite: set market value: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM caixinhas WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("patrimonio sqlite: delete: %w", err)
	}
	return notFoundIfNoRows(res)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func notFoundIfNoRows(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("patrimonio sqlite: rows affected: %w", err)
	}
	if n == 0 {
		return ErrCaixinhaNotFound
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func strPtrToNull(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func int64PtrToNull(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
