CREATE TABLE IF NOT EXISTS categories (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    type           TEXT NOT NULL CHECK(type IN ('despesa', 'receita', 'transferencia')),
    icon           TEXT,
    color          TEXT,
    can_be_deleted INTEGER NOT NULL DEFAULT 1,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
