-- Subcategoria de sistema para o SALDO INICIAL de uma caixinha: dinheiro que o
-- usuario ja tinha guardado antes de comecar a usar o app. Direcao aporte (conta no
-- guardado), mas is_balance_adjustment=1 para NAO mexer no disponivel de caixa
-- (mesmo principio do Saldo Inicial do E6). Fica escondida dos Lancamentos por ter caixinha_id
INSERT INTO subcategories (id, category_id, name, icon, color, can_be_deleted, is_balance_adjustment, caixinha_direction, created_at, updated_at)
VALUES ('sub-caixinha-saldo-inicial', 'cat-caixinha', 'Saldo inicial da caixinha', 'flag', '#8E44AD', 0, 1, 'aporte', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z');
