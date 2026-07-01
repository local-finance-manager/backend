-- Pagamento parcial/antecipado de fatura. Troca o pagamento unico por fatura
-- (credit_card_invoice_payments, PK card+reference) por um LEDGER de N pagamentos.
-- Modelo "por saldo devedor", em que cada pagamento e uma saida de caixa na sua
-- data e a fatura fica quitada quando a soma dos pagamentos cobre o total.

CREATE TABLE IF NOT EXISTS credit_card_invoice_payment (
    id             TEXT    PRIMARY KEY,
    credit_card_id TEXT    NOT NULL REFERENCES credit_cards(id) ON DELETE CASCADE,
    reference      TEXT    NOT NULL,
    amount         INTEGER NOT NULL,
    payment_date   TEXT    NOT NULL,
    transaction_id TEXT,
    created_at     TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ccinvpay_card_ref
    ON credit_card_invoice_payment(credit_card_id, reference);

-- Backfill cada pagamento antigo (1 por fatura) como uma entrada no ledger. O valor
-- vem do lancamento de pagamento vinculado, ou 0 quando nao houver.
INSERT INTO credit_card_invoice_payment (id, credit_card_id, reference, amount, payment_date, transaction_id, created_at)
SELECT
    p.credit_card_id || ':' || p.reference,
    p.credit_card_id,
    p.reference,
    COALESCE((SELECT t.amount FROM transactions t WHERE t.id = p.transaction_id), 0),
    p.payment_date,
    p.transaction_id,
    p.created_at
FROM credit_card_invoice_payments p;

DROP TABLE credit_card_invoice_payments;
