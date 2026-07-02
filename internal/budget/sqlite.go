package budget

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

var _ Repository = (*SQLiteRepository)(nil)

// SQLiteRepository implementa Repository (owner das tabelas allocation_*).
type SQLiteRepository struct{ db *sql.DB }

// NewSQLiteRepository cria o repositório.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

const destCols = `id, reference, name, kind, mode, percentage, fixed_amount,
	preset_subcategory_id, preset_payment_method, preset_description, display_order,
	materialized_transaction_id, materialized_amount, materialized_at, created_at, updated_at, caixinha_id`

func scanDestination(s interface{ Scan(...any) error }) (Destination, error) {
	var d Destination
	var pct, matAmt sql.NullInt64
	var fixed sql.NullInt64
	var presetSub, presetPm, presetDesc, matTx, matAt, caixinhaID sql.NullString
	var createdAt, updatedAt string
	if err := s.Scan(
		&d.ID, &d.Reference, &d.Name, &d.Kind, &d.Mode, &pct, &fixed,
		&presetSub, &presetPm, &presetDesc, &d.DisplayOrder,
		&matTx, &matAmt, &matAt, &createdAt, &updatedAt, &caixinhaID,
	); err != nil {
		return Destination{}, err
	}
	d.CaixinhaID = nullToPtr(caixinhaID)
	if pct.Valid {
		v := int(pct.Int64)
		d.Percentage = &v
	}
	if fixed.Valid {
		d.FixedAmount = &fixed.Int64
	}
	d.PresetSubcategoryID = nullToPtr(presetSub)
	d.PresetPaymentMethod = nullToPtr(presetPm)
	d.PresetDescription = nullToPtr(presetDesc)
	d.MaterializedTxID = nullToPtr(matTx)
	if matAmt.Valid {
		d.MaterializedAmount = &matAmt.Int64
	}
	if matAt.Valid {
		t, _ := time.Parse(time.RFC3339, matAt.String)
		d.MaterializedAt = &t
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	d.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return d, nil
}

func (r *SQLiteRepository) ListDestinations(ctx context.Context, reference string) ([]Destination, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+destCols+" FROM allocation_destination WHERE reference = ? ORDER BY display_order, created_at", reference)
	if err != nil {
		return nil, fmt.Errorf("budget repo: list destinations: %w", err)
	}
	defer rows.Close()
	out := []Destination{}
	for rows.Next() {
		d, err := scanDestination(rows)
		if err != nil {
			return nil, fmt.Errorf("budget repo: scan destination: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) GetDestination(ctx context.Context, id string) (Destination, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+destCols+" FROM allocation_destination WHERE id = ?", id)
	d, err := scanDestination(row)
	if err == sql.ErrNoRows {
		return Destination{}, ErrDestinationNotFound
	}
	if err != nil {
		return Destination{}, fmt.Errorf("budget repo: get destination: %w", err)
	}
	return d, nil
}

const insertDestSQL = `INSERT INTO allocation_destination (` + destCols + `)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

func destArgs(d Destination) []any {
	return []any{
		d.ID, d.Reference, d.Name, string(d.Kind), string(d.Mode),
		ptrToNullInt(d.Percentage), int64PtrToNull(d.FixedAmount),
		strPtrToNull(d.PresetSubcategoryID), strPtrToNull(d.PresetPaymentMethod), strPtrToNull(d.PresetDescription),
		d.DisplayOrder, strPtrToNull(d.MaterializedTxID), int64PtrToNull(d.MaterializedAmount),
		timePtrToNull(d.MaterializedAt), d.CreatedAt.UTC().Format(time.RFC3339), d.UpdatedAt.UTC().Format(time.RFC3339),
		strPtrToNull(d.CaixinhaID),
	}
}

func (r *SQLiteRepository) CreateDestination(ctx context.Context, d Destination) error {
	if _, err := r.db.ExecContext(ctx, insertDestSQL, destArgs(d)...); err != nil {
		return fmt.Errorf("budget repo: create destination: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) CreateDestinations(ctx context.Context, ds []Destination) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("budget repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, insertDestSQL)
	if err != nil {
		return fmt.Errorf("budget repo: prepare: %w", err)
	}
	defer stmt.Close()
	for _, d := range ds {
		if _, err := stmt.ExecContext(ctx, destArgs(d)...); err != nil {
			return fmt.Errorf("budget repo: bulk create destination: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("budget repo: commit: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) UpdateDestination(ctx context.Context, d Destination) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE allocation_destination SET name=?, kind=?, mode=?, percentage=?, fixed_amount=?,
			preset_subcategory_id=?, preset_payment_method=?, preset_description=?, caixinha_id=?, display_order=?, updated_at=?
		WHERE id=?`,
		d.Name, string(d.Kind), string(d.Mode), ptrToNullInt(d.Percentage), int64PtrToNull(d.FixedAmount),
		strPtrToNull(d.PresetSubcategoryID), strPtrToNull(d.PresetPaymentMethod), strPtrToNull(d.PresetDescription),
		strPtrToNull(d.CaixinhaID), d.DisplayOrder, time.Now().UTC().Format(time.RFC3339), d.ID)
	if err != nil {
		return fmt.Errorf("budget repo: update destination: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDestinationNotFound
	}
	return nil
}

func (r *SQLiteRepository) DeleteDestination(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM allocation_destination WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("budget repo: delete destination: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDestinationNotFound
	}
	return nil
}

func (r *SQLiteRepository) SetMaterialized(ctx context.Context, id, txID string, amount int64, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE allocation_destination
		SET materialized_transaction_id=?, materialized_amount=?, materialized_at=?, updated_at=?
		WHERE id=? AND materialized_transaction_id IS NULL`,
		txID, amount, at.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return false, fmt.Errorf("budget repo: set materialized: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *SQLiteRepository) ClearMaterialized(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE allocation_destination
		SET materialized_transaction_id=NULL, materialized_amount=NULL, materialized_at=NULL, updated_at=?
		WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("budget repo: clear materialized: %w", err)
	}
	return nil
}

// ─── Templates ───────────────────────────────────────────────────────────────

func (r *SQLiteRepository) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, created_at, updated_at FROM allocation_template ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("budget repo: list templates: %w", err)
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		var t Template
		var created, updated string
		if err := rows.Scan(&t.ID, &t.Name, &created, &updated); err != nil {
			return nil, fmt.Errorf("budget repo: scan template: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		items, err := r.templateItems(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Items = items
	}
	return out, nil
}

func (r *SQLiteRepository) GetTemplate(ctx context.Context, id string) (Template, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, name, created_at, updated_at FROM allocation_template WHERE id = ?", id)
	var t Template
	var created, updated string
	if err := row.Scan(&t.ID, &t.Name, &created, &updated); err == sql.ErrNoRows {
		return Template{}, ErrTemplateNotFound
	} else if err != nil {
		return Template{}, fmt.Errorf("budget repo: get template: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	items, err := r.templateItems(ctx, id)
	if err != nil {
		return Template{}, err
	}
	t.Items = items
	return t, nil
}

func (r *SQLiteRepository) templateItems(ctx context.Context, templateID string) ([]TemplateItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, kind, mode, percentage, fixed_amount,
			preset_subcategory_id, preset_payment_method, preset_description, display_order, caixinha_id
		FROM allocation_template_item WHERE template_id = ? ORDER BY display_order`, templateID)
	if err != nil {
		return nil, fmt.Errorf("budget repo: template items: %w", err)
	}
	defer rows.Close()
	out := []TemplateItem{}
	for rows.Next() {
		var it TemplateItem
		var pct, fixed sql.NullInt64
		var sub, pm, desc, caixinhaID sql.NullString
		if err := rows.Scan(&it.ID, &it.Name, &it.Kind, &it.Mode, &pct, &fixed, &sub, &pm, &desc, &it.DisplayOrder, &caixinhaID); err != nil {
			return nil, fmt.Errorf("budget repo: scan template item: %w", err)
		}
		if pct.Valid {
			v := int(pct.Int64)
			it.Percentage = &v
		}
		if fixed.Valid {
			it.FixedAmount = &fixed.Int64
		}
		it.PresetSubcategoryID = nullToPtr(sub)
		it.PresetPaymentMethod = nullToPtr(pm)
		it.PresetDescription = nullToPtr(desc)
		it.CaixinhaID = nullToPtr(caixinhaID)
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) CreateTemplate(ctx context.Context, t Template) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("budget repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO allocation_template (id, name, created_at, updated_at) VALUES (?,?,?,?)",
		t.ID, t.Name, now, now); err != nil {
		return fmt.Errorf("budget repo: insert template: %w", err)
	}
	for _, it := range t.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO allocation_template_item
			(id, template_id, name, kind, mode, percentage, fixed_amount, preset_subcategory_id, preset_payment_method, preset_description, display_order, caixinha_id)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			it.ID, t.ID, it.Name, string(it.Kind), string(it.Mode), ptrToNullInt(it.Percentage), int64PtrToNull(it.FixedAmount),
			strPtrToNull(it.PresetSubcategoryID), strPtrToNull(it.PresetPaymentMethod), strPtrToNull(it.PresetDescription), it.DisplayOrder, strPtrToNull(it.CaixinhaID),
		); err != nil {
			return fmt.Errorf("budget repo: insert template item: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("budget repo: commit: %w", err)
	}
	return nil
}

// ─── helpers de nulos ────────────────────────────────────────────────────────

func nullToPtr(n sql.NullString) *string {
	if n.Valid {
		return &n.String
	}
	return nil
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
func ptrToNullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
func timePtrToNull(p *time.Time) any {
	if p == nil {
		return nil
	}
	return p.UTC().Format(time.RFC3339)
}
