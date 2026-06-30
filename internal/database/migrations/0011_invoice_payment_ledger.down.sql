-- Reverte o ledger de pagamentos para o modelo de pagamento único por fatura.
-- Best-effort: mantém o pagamento mais antigo de cada fatura.

CREATE TABLE IF NOT EXISTS credit_card_invoice_payments (
    credit_card_id TEXT NOT NULL REFERENCES credit_cards(id) ON DELETE CASCADE,
    reference      TEXT NOT NULL,
    payment_date   TEXT NOT NULL,
    transaction_id TEXT,
    created_at     TEXT NOT NULL,
    PRIMARY KEY (credit_card_id, reference)
);

INSERT OR IGNORE INTO credit_card_invoice_payments (credit_card_id, reference, payment_date, transaction_id, created_at)
SELECT credit_card_id, reference, payment_date, transaction_id, created_at
FROM credit_card_invoice_payment
ORDER BY created_at ASC;

DROP TABLE credit_card_invoice_payment;
