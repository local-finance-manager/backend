-- Recria a tabela do ledger vazia (best-effort). Os lançamentos sinteticos apagados na up
-- nao sao restauraveis.
CREATE TABLE IF NOT EXISTS credit_card_invoice_payment (
    id             TEXT    PRIMARY KEY,
    credit_card_id TEXT    NOT NULL REFERENCES credit_cards(id) ON DELETE CASCADE,
    reference      TEXT    NOT NULL,
    amount         INTEGER NOT NULL,
    payment_date   TEXT    NOT NULL,
    transaction_id TEXT,
    created_at     TEXT    NOT NULL
);
