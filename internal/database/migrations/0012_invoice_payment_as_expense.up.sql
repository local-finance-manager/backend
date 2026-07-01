-- Pagar a fatura do cartao deve contar como DESPESA no regime de caixa (D14): o dinheiro
-- sai da conta quando a fatura e paga. Antes "Pagamento de Fatura de Cartao" era uma
-- transferencia (neutra), entao a despesa sumia do fluxo de caixa. Move a subcategoria
-- para a categoria de despesa "Cartao de Credito" e reclassifica os pagamentos ja feitos.

UPDATE subcategories
   SET category_id = 'cat-cartao', updated_at = '2026-06-30T00:00:00Z'
 WHERE id = 'sub-trf-pgto-fatura';

UPDATE transactions
   SET type = 'despesa', updated_at = '2026-06-30T00:00:00Z'
 WHERE subcategory_id = 'sub-trf-pgto-fatura';
