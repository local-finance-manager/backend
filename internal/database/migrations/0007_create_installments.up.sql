CREATE TABLE IF NOT EXISTS installment_groups (
    id                  TEXT    PRIMARY KEY,
    credit_card_id      TEXT    NOT NULL REFERENCES credit_cards(id) ON DELETE RESTRICT,
    subcategory_id      TEXT    NOT NULL REFERENCES subcategories(id) ON DELETE RESTRICT,
    title               TEXT    NOT NULL,
    description         TEXT,
    total_amount        INTEGER NOT NULL CHECK(total_amount > 0),
    principal_amount    INTEGER,
    installments_count  INTEGER NOT NULL CHECK(installments_count BETWEEN 2 AND 72),
    purchase_date       TEXT    NOT NULL,
    first_reference     TEXT    NOT NULL,
    created_at          TEXT    NOT NULL,
    updated_at          TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_installment_groups_credit_card_id ON installment_groups(credit_card_id);

ALTER TABLE transactions ADD COLUMN installment_group_id TEXT REFERENCES installment_groups(id) ON DELETE CASCADE;
ALTER TABLE transactions ADD COLUMN installment_number INTEGER;
ALTER TABLE transactions ADD COLUMN installment_total INTEGER;

CREATE INDEX IF NOT EXISTS idx_transactions_installment_group_id ON transactions(installment_group_id)
