-- Um registro POR MÊS FECHADO. Mês sem registro aqui = ABERTO (estado derivado).
CREATE TABLE IF NOT EXISTS report_monthly_closing (
    reference            TEXT PRIMARY KEY,        -- 'YYYY-MM'
    closed_at            TEXT NOT NULL,           -- ISO-8601 UTC
    month_last_day       TEXT NOT NULL,           -- 'YYYY-MM-DD'
    hard_lock_at         TEXT NOT NULL,           -- 'YYYY-MM-DD' = month_last_day + 90 dias
    total_receitas       INTEGER NOT NULL,
    total_despesas       INTEGER NOT NULL,
    total_transferencias INTEGER NOT NULL,
    saldo_periodo        INTEGER NOT NULL,
    saldo_inicial        INTEGER NOT NULL,
    saldo_final          INTEGER NOT NULL,
    tx_count             INTEGER NOT NULL,
    recalculated_at      TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

-- Agregado congelado na granularidade de SUBCATEGORIA (categoria = soma das filhas).
CREATE TABLE IF NOT EXISTS report_monthly_snapshot (
    reference      TEXT    NOT NULL REFERENCES report_monthly_closing(reference) ON DELETE CASCADE,
    subcategory_id TEXT    NOT NULL,
    category_id    TEXT    NOT NULL,
    type           TEXT    NOT NULL,
    total          INTEGER NOT NULL,
    tx_count       INTEGER NOT NULL,
    PRIMARY KEY (reference, subcategory_id)
);

CREATE INDEX IF NOT EXISTS idx_report_snapshot_reference ON report_monthly_snapshot(reference);
CREATE INDEX IF NOT EXISTS idx_report_snapshot_category  ON report_monthly_snapshot(reference, category_id);
CREATE INDEX IF NOT EXISTS idx_report_snapshot_type      ON report_monthly_snapshot(reference, type)
