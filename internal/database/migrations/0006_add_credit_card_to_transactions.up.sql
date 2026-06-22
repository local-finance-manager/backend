ALTER TABLE transactions ADD COLUMN credit_card_id TEXT REFERENCES credit_cards(id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS idx_transactions_credit_card_id ON transactions(credit_card_id)
