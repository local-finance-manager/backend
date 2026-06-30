-- Destinos do plano mensal de alocação de receitas.
CREATE TABLE IF NOT EXISTS allocation_destination (
    id                          TEXT    PRIMARY KEY,
    reference                   TEXT    NOT NULL,
    name                        TEXT    NOT NULL,
    kind                        TEXT    NOT NULL CHECK(kind IN ('despesa','investimento')),
    mode                        TEXT    NOT NULL CHECK(mode IN ('percentual','valor_fixo')),
    percentage                  INTEGER,
    fixed_amount                INTEGER,
    preset_subcategory_id       TEXT,
    preset_payment_method       TEXT,
    preset_description          TEXT,
    display_order               INTEGER NOT NULL DEFAULT 0,
    materialized_transaction_id TEXT,
    materialized_amount         INTEGER,
    materialized_at             TEXT,
    created_at                  TEXT    NOT NULL,
    updated_at                  TEXT    NOT NULL,
    CHECK (
        (mode = 'percentual' AND percentage IS NOT NULL AND percentage BETWEEN 1 AND 10000)
        OR (mode = 'valor_fixo' AND fixed_amount IS NOT NULL AND fixed_amount > 0)
    )
);

CREATE INDEX IF NOT EXISTS idx_allocation_destination_reference ON allocation_destination(reference);

-- Templates reutilizáveis (conjunto de destinos sem mês).
CREATE TABLE IF NOT EXISTS allocation_template (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS allocation_template_item (
    id                    TEXT PRIMARY KEY,
    template_id           TEXT NOT NULL REFERENCES allocation_template(id) ON DELETE CASCADE,
    name                  TEXT NOT NULL,
    kind                  TEXT NOT NULL CHECK(kind IN ('despesa','investimento')),
    mode                  TEXT NOT NULL CHECK(mode IN ('percentual','valor_fixo')),
    percentage            INTEGER,
    fixed_amount          INTEGER,
    preset_subcategory_id TEXT,
    preset_payment_method TEXT,
    preset_description    TEXT,
    display_order         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_allocation_template_item_template ON allocation_template_item(template_id)
