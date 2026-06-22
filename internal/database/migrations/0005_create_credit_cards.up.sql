CREATE TABLE IF NOT EXISTS credit_cards (
    id               TEXT    PRIMARY KEY,
    name             TEXT    NOT NULL,
    brand            TEXT    NOT NULL CHECK(brand IN ('visa','mastercard','elo','amex','hipercard','outros')),
    last_four_digits TEXT,
    issuer           TEXT,
    credit_limit     INTEGER NOT NULL CHECK(credit_limit > 0),
    closing_day      INTEGER NOT NULL CHECK(closing_day BETWEEN 1 AND 31),
    due_day          INTEGER NOT NULL CHECK(due_day BETWEEN 1 AND 31),
    color            TEXT,
    icon             TEXT,
    archived         INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_credit_cards_archived ON credit_cards(archived);

CREATE TABLE IF NOT EXISTS credit_card_invoice_payments (
    credit_card_id TEXT NOT NULL REFERENCES credit_cards(id) ON DELETE CASCADE,
    reference      TEXT NOT NULL,
    payment_date   TEXT NOT NULL,
    transaction_id TEXT,
    created_at     TEXT NOT NULL,
    PRIMARY KEY (credit_card_id, reference)
)
