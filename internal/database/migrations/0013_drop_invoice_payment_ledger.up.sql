-- Pagar fatura passou a ser apenas marcar as COMPRAS como pagas (Opção 1), sem ledger e
-- sem lançamento sintetico de "Pagamento de Fatura". Remove a tabela do ledger e apaga os
-- lançamentos sinteticos antigos (as compras correspondentes seguem marcadas como pagas,
-- pela data de pagamento, entao o gasto continua refletido no caixa sem dupla contagem).

DROP TABLE IF EXISTS credit_card_invoice_payment;

DELETE FROM transactions WHERE subcategory_id = 'sub-trf-pgto-fatura';
