-- Reverte: "Pagamento de Fatura de Cartao" volta a ser transferencia.
UPDATE transactions
   SET type = 'transferencia', updated_at = '2026-06-30T00:00:00Z'
 WHERE subcategory_id = 'sub-trf-pgto-fatura';

UPDATE subcategories
   SET category_id = 'cat-transferencias', updated_at = '2026-06-30T00:00:00Z'
 WHERE id = 'sub-trf-pgto-fatura';
