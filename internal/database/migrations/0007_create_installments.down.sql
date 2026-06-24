DROP INDEX IF EXISTS idx_transactions_installment_group_id;
ALTER TABLE transactions DROP COLUMN installment_total;
ALTER TABLE transactions DROP COLUMN installment_number;
ALTER TABLE transactions DROP COLUMN installment_group_id;
DROP INDEX IF EXISTS idx_installment_groups_credit_card_id;
DROP TABLE IF EXISTS installment_groups
