ALTER TABLE subcategories ADD COLUMN is_balance_adjustment INTEGER NOT NULL DEFAULT 0;

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, is_balance_adjustment, created_at, updated_at) VALUES
('sub-trf-saldo-inicial',  'cat-transferencias', 'Saldo Inicial',                 NULL, NULL, 0, 1, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-trf-saldo-anterior', 'cat-transferencias', 'Saldo do Mês Anterior',         NULL, NULL, 0, 1, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-trf-pgto-fatura',    'cat-transferencias', 'Pagamento de Fatura de Cartão', NULL, NULL, 0, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')
