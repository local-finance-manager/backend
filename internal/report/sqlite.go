package report

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

var _ Repository = (*SQLiteRepository)(nil)

// SQLiteRepository implementa Repository. Owner de report_monthly_closing e
// report_monthly_snapshot.
type SQLiteRepository struct{ db *sql.DB }

// NewSQLiteRepository cria o repositório.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

const closingCols = `reference, closed_at, month_last_day, hard_lock_at,
	total_receitas, total_despesas, total_transferencias, saldo_periodo,
	saldo_inicial, saldo_final, tx_count, recalculated_at, created_at, updated_at`

func scanClosing(s interface{ Scan(...any) error }) (Closing, error) {
	var c Closing
	var closedAt, createdAt, updatedAt string
	var recalc sql.NullString
	if err := s.Scan(
		&c.Reference, &closedAt, &c.MonthLastDay, &c.HardLockAt,
		&c.Totals.Receitas, &c.Totals.Despesas, &c.Totals.Transferencias, &c.Totals.SaldoPeriodo,
		&c.Totals.SaldoInicial, &c.Totals.SaldoFinal, &c.Totals.TxCount, &recalc, &createdAt, &updatedAt,
	); err != nil {
		return Closing{}, err
	}
	c.ClosedAt, _ = time.Parse(time.RFC3339, closedAt)
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if recalc.Valid {
		t, _ := time.Parse(time.RFC3339, recalc.String)
		c.RecalculatedAt = &t
	}
	return c, nil
}

func (r *SQLiteRepository) GetClosing(ctx context.Context, reference string) (Closing, bool, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+closingCols+" FROM report_monthly_closing WHERE reference = ?", reference)
	c, err := scanClosing(row)
	if err == sql.ErrNoRows {
		return Closing{}, false, nil
	}
	if err != nil {
		return Closing{}, false, fmt.Errorf("report repo: get closing: %w", err)
	}
	return c, true, nil
}

func (r *SQLiteRepository) ListClosings(ctx context.Context) ([]Closing, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+closingCols+" FROM report_monthly_closing ORDER BY reference DESC")
	if err != nil {
		return nil, fmt.Errorf("report repo: list closings: %w", err)
	}
	defer rows.Close()
	out := []Closing{}
	for rows.Next() {
		c, err := scanClosing(rows)
		if err != nil {
			return nil, fmt.Errorf("report repo: scan closing: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) ClosingsForRefs(ctx context.Context, refs []string) (map[string]Closing, error) {
	out := map[string]Closing{}
	if len(refs) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(refs)), ",")
	args := make([]any, len(refs))
	for i, r := range refs {
		args[i] = r
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+closingCols+" FROM report_monthly_closing WHERE reference IN ("+ph+")", args...)
	if err != nil {
		return nil, fmt.Errorf("report repo: closings for refs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scanClosing(rows)
		if err != nil {
			return nil, fmt.Errorf("report repo: scan closing: %w", err)
		}
		out[c.Reference] = c
	}
	return out, rows.Err()
}

const upsertClosingSQL = `
INSERT INTO report_monthly_closing (` + closingCols + `)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(reference) DO UPDATE SET
	closed_at=excluded.closed_at, month_last_day=excluded.month_last_day, hard_lock_at=excluded.hard_lock_at,
	total_receitas=excluded.total_receitas, total_despesas=excluded.total_despesas,
	total_transferencias=excluded.total_transferencias, saldo_periodo=excluded.saldo_periodo,
	saldo_inicial=excluded.saldo_inicial, saldo_final=excluded.saldo_final, tx_count=excluded.tx_count,
	recalculated_at=excluded.recalculated_at, updated_at=excluded.updated_at`

func (r *SQLiteRepository) SaveClosing(ctx context.Context, c Closing, rows []shared.SubcategoryAggregate) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("report repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var recalc any
	if c.RecalculatedAt != nil {
		recalc = c.RecalculatedAt.UTC().Format(time.RFC3339)
	}
	if _, err := tx.ExecContext(ctx, upsertClosingSQL,
		c.Reference, c.ClosedAt.UTC().Format(time.RFC3339), c.MonthLastDay, c.HardLockAt,
		c.Totals.Receitas, c.Totals.Despesas, c.Totals.Transferencias, c.Totals.SaldoPeriodo,
		c.Totals.SaldoInicial, c.Totals.SaldoFinal, c.Totals.TxCount, recalc,
		c.CreatedAt.UTC().Format(time.RFC3339), c.UpdatedAt.UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("report repo: upsert closing: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM report_monthly_snapshot WHERE reference = ?", c.Reference); err != nil {
		return fmt.Errorf("report repo: clear snapshot: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO report_monthly_snapshot (reference, subcategory_id, category_id, type, total, tx_count)
		 VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("report repo: prepare snapshot: %w", err)
	}
	defer stmt.Close()
	for _, a := range rows {
		if _, err := stmt.ExecContext(ctx, c.Reference, a.SubcategoryID, a.CategoryID, a.Type, a.Total, a.TxCount); err != nil {
			return fmt.Errorf("report repo: insert snapshot: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("report repo: commit: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Snapshot(ctx context.Context, reference string) ([]shared.SubcategoryAggregate, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT subcategory_id, category_id, type, total, tx_count
		 FROM report_monthly_snapshot WHERE reference = ?`, reference)
	if err != nil {
		return nil, fmt.Errorf("report repo: snapshot: %w", err)
	}
	defer rows.Close()
	return scanAggs(rows)
}

func (r *SQLiteRepository) SnapshotForRefs(ctx context.Context, refs []string) ([]shared.SubcategoryAggregate, error) {
	if len(refs) == 0 {
		return []shared.SubcategoryAggregate{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(refs)), ",")
	args := make([]any, len(refs))
	for i, r := range refs {
		args[i] = r
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT subcategory_id, category_id, type, SUM(total), SUM(tx_count)
		 FROM report_monthly_snapshot WHERE reference IN (`+ph+`)
		 GROUP BY subcategory_id, category_id, type`, args...)
	if err != nil {
		return nil, fmt.Errorf("report repo: snapshot for refs: %w", err)
	}
	defer rows.Close()
	return scanAggs(rows)
}

func scanAggs(rows *sql.Rows) ([]shared.SubcategoryAggregate, error) {
	out := []shared.SubcategoryAggregate{}
	for rows.Next() {
		var a shared.SubcategoryAggregate
		if err := rows.Scan(&a.SubcategoryID, &a.CategoryID, &a.Type, &a.Total, &a.TxCount); err != nil {
			return nil, fmt.Errorf("report repo: scan agg: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
