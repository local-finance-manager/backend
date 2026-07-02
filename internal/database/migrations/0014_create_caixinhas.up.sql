-- Caixinhas (envelopes de patrimonio): saldo persistente construido por aporte e resgate
CREATE TABLE IF NOT EXISTS caixinhas (
    id                 TEXT    PRIMARY KEY,
    name               TEXT    NOT NULL,
    type               TEXT    NOT NULL CHECK(type IN ('reserva','objetivo','investimento')),
    meta_valor         INTEGER,
    data_alvo          TEXT,
    valor_mercado      INTEGER,
    data_valor_mercado TEXT,
    color              TEXT,
    icon               TEXT,
    display_order      INTEGER NOT NULL DEFAULT 0,
    archived           INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT    NOT NULL,
    updated_at         TEXT    NOT NULL
);

-- Movimentos de caixinha vivem em transactions (livro-caixa unico) marcados por caixinha_id
ALTER TABLE transactions ADD COLUMN caixinha_id TEXT REFERENCES caixinhas(id);

CREATE INDEX IF NOT EXISTS idx_transactions_caixinha ON transactions(caixinha_id);

-- Direcao do movimento vive na subcategoria de sistema (mesmo precedente do is_balance_adjustment)
ALTER TABLE subcategories ADD COLUMN caixinha_direction TEXT;

-- Categoria e subcategorias de sistema para os movimentos neutros de caixinha
INSERT INTO categories (id, name, type, icon, color, can_be_deleted, created_at, updated_at)
VALUES ('cat-caixinha', 'Movimentação de Caixinha', 'transferencia', 'piggy-bank', '#8E44AD', 0,
        '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z');

INSERT INTO subcategories (id, category_id, name, icon, color, can_be_deleted, is_balance_adjustment, caixinha_direction, created_at, updated_at)
VALUES
  ('sub-caixinha-aporte',  'cat-caixinha', 'Aporte em caixinha',  'arrow-down-circle', '#8E44AD', 0, 0, 'aporte',  '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z'),
  ('sub-caixinha-resgate', 'cat-caixinha', 'Resgate de caixinha', 'arrow-up-circle',   '#8E44AD', 0, 0, 'resgate', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z');
