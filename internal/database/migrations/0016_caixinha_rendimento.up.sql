-- Subcategoria de sistema para RENDIMENTO de caixinha (juros/dividendos reinvestidos):
-- direcao aporte (conta no guardado/patrimonio) e is_balance_adjustment=1 para NAO mexer
-- no disponivel nem contar como receita. Separada do aporte comum para dar visibilidade
-- de quanto veio de rendimento vs quanto foi guardado pelo usuario
INSERT INTO subcategories (id, category_id, name, icon, color, can_be_deleted, is_balance_adjustment, caixinha_direction, created_at, updated_at)
VALUES ('sub-caixinha-rendimento', 'cat-caixinha', 'Rendimento da caixinha', 'trending-up', '#8E44AD', 0, 1, 'aporte', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z');
