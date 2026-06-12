CREATE TABLE IF NOT EXISTS transactions (
    id                      TEXT    PRIMARY KEY,
    title                   TEXT    NOT NULL,
    description             TEXT,
    amount                  INTEGER NOT NULL CHECK(amount > 0),
    type                    TEXT    NOT NULL CHECK(type IN ('despesa','receita','transferencia')),
    subcategory_id          TEXT    NOT NULL REFERENCES subcategories(id) ON DELETE RESTRICT,
    payment_method          TEXT    NOT NULL CHECK(payment_method IN (
                                'pix','cartao_credito','cartao_debito',
                                'dinheiro','ted','boleto','outros'
                            )),
    status                  TEXT    NOT NULL DEFAULT 'pendente' CHECK(status IN ('pendente','realizado','cancelado')),
    competence_date         TEXT    NOT NULL,
    payment_date            TEXT,
    account_id              TEXT,
    destination_account_id  TEXT,
    created_at              TEXT    NOT NULL,
    updated_at              TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transactions_competence_date  ON transactions(competence_date);
CREATE INDEX IF NOT EXISTS idx_transactions_subcategory_id   ON transactions(subcategory_id);
CREATE INDEX IF NOT EXISTS idx_transactions_status           ON transactions(status);
CREATE INDEX IF NOT EXISTS idx_transactions_type             ON transactions(type);
CREATE INDEX IF NOT EXISTS idx_transactions_type_status_date ON transactions(type, status, competence_date)
