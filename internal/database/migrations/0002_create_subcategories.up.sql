CREATE TABLE IF NOT EXISTS subcategories (
    id             TEXT PRIMARY KEY,
    category_id    TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    name           TEXT NOT NULL,
    icon           TEXT,
    color          TEXT,
    can_be_deleted INTEGER NOT NULL DEFAULT 1,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_subcategories_category_id ON subcategories(category_id);
