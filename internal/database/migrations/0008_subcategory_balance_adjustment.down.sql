DELETE FROM subcategories WHERE id IN ('sub-trf-saldo-inicial', 'sub-trf-saldo-anterior', 'sub-trf-pgto-fatura');
ALTER TABLE subcategories DROP COLUMN is_balance_adjustment
