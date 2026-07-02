DROP INDEX IF EXISTS idx_transactions_caixinha;
DELETE FROM subcategories WHERE id IN ('sub-caixinha-aporte','sub-caixinha-resgate');
DELETE FROM categories WHERE id = 'cat-caixinha';
ALTER TABLE transactions DROP COLUMN caixinha_id;
ALTER TABLE subcategories DROP COLUMN caixinha_direction;
DROP TABLE IF EXISTS caixinhas;
