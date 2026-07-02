-- Destino da Receitas pode apontar para uma CAIXINHA: ao materializar, gera um APORTE
-- na caixinha (em vez de uma transferencia generica). caixinha_id nulo = comportamento antigo
ALTER TABLE allocation_destination ADD COLUMN caixinha_id TEXT REFERENCES caixinhas(id);
ALTER TABLE allocation_template_item ADD COLUMN caixinha_id TEXT REFERENCES caixinhas(id);
