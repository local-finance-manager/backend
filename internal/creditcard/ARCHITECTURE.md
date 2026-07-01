# Arquitetura — Módulo `creditcard`

## Responsabilidade

Gerencia cartões de crédito e suas **faturas**. Cartões são CRUD persistido;
faturas são **projeções calculadas em tempo de leitura** a partir dos lançamentos
do módulo `transaction` (apenas o *pagamento* de uma fatura é persistido — D4).

Cobre: ciclo de fatura (fechamento/vencimento/competência), status derivado da
fatura, utilização do limite com classificação, breakdown por categoria, resumo
mensal por competência, pagamento/estorno de fatura e arquivamento de cartão.

---

## Estrutura de arquivos

```
internal/creditcard/
├── ARCHITECTURE.md     # este documento
├── creditcard.go       # domínio: entidade, enums, erros, regras de ciclo/utilização, validação
├── invoice.go          # domínio: projeções de fatura + agregações puras (BuildInvoice, UsedLimit, breakdown)
├── repository.go       # interfaces: CreditCardRepository, InvoicePaymentRepository, CardTransactionReader, SubcategoryReader (ports)
├── usecases.go         # tipos de I/O + interfaces de caso de uso
├── usecases_impl.go    # implementações dos use cases (cartões, faturas, pagamento, resumo)
├── facade.go           # adapter produtor: CreditCardChecker (consumido por transaction)
├── sqlite.go           # adapters SQLite: CreditCardRepository + InvoicePaymentRepository
├── handler.go          # HTTP handler + HandlerDeps + conversão domain → response (snake_case)
├── routes.go           # registro de rotas Chi v5
├── creditcard_test.go  # testes de domínio (ciclo, utilização, validação)
├── invoice_test.go     # testes das agregações de fatura
├── facade_test.go      # teste do CreditCardChecker
├── usecases_test.go    # testes de casos de uso com fakes + compile-time checks
└── sqlite_test.go      # testes de integração SQLite (:memory:)
```

---

## Camadas e dependências

```
HTTP (handler.go / routes.go)
        │  depende de
Use Cases (usecases.go + usecases_impl.go)
        │  depende de
Ports (repository.go: CreditCardRepository, InvoicePaymentRepository, CardTransactionReader)
        │  implementado por
SQLite Adapters (sqlite.go) + CardReader injetado (módulo transaction)
        │  depende de
domain (creditcard.go, invoice.go) + shared (pagination.go, cardtxn.go)
```

A camada de domínio não importa nada interno (apenas `shared`). As funções de
domínio de `creditcard.go`/`invoice.go` são **puras** (calendário, agregações) —
o que as torna fáceis de testar exaustivamente sem infra.

---

## Decisão central (D4): fatura não é armazenada

Uma fatura é **recalculada a cada leitura** a partir dos lançamentos do cartão. O
único estado persistido sobre faturas é o **pagamento** (`InvoicePayment`), via
`InvoicePaymentRepository`. Isso evita desnormalização e mantém a fatura sempre
coerente com os lançamentos atuais.

Fluxo de uma fatura:
1. O use case lê as compras do cartão via `CardTransactionReader.ListByCard`.
2. `bucketByReference` agrupa por `reference` (YYYY-MM) usando `InvoiceReferenceFor`.
3. `BuildInvoice` calcula datas do ciclo, total, count, status e breakdown.
4. Os pagamentos são derivados das compras pagas (por `payment_date`). `StatusPaga` ⇔ sem compra em aberto.

---

## Decisão (D1/D7): leitura e validação cross-module

`creditcard` **não importa** `transaction` e vice-versa (regra do monolito modular,
guia §3.3). A ponte é feita por interfaces + um DTO neutro em `shared`:

- **Lê lançamentos** via port `CardTransactionReader` (definido aqui, consumidor).
  Implementado por `transaction.CardReader` (produtor) e injetado no `main.go`.
  O retorno é `shared.CardTransaction` — data-carrier puro comum aos dois módulos.
- **Valida vínculo** expondo `CreditCardChecker` (`facade.go`, produtor): satisfaz
  `transaction.CreditCardChecker` e `installment.CreditCardChecker` por structural typing.
  `CheckLinkable` retorna `ErrCreditCardNotFound`/`ErrCannotLinkArchivedCard` ou `nil`.
- **Resolve fatura de parcelas** expondo `InvoiceReferenceFacade` (`facade.go`, produtor):
  satisfaz `installment.InvoiceReferenceResolver`. `ReferencesFor(cardID, dates)` busca o
  cartão 1× e mapeia `InvoiceReferenceFor` (ciclo) sobre as datas — o `installment` resolve
  a `reference` de cada parcela sem reimplementar o ciclo nem importar `creditcard`.
- **Deriva o tipo do lançamento de pagamento** via port `SubcategoryReader` (consumidor),
  satisfeito por `category.SubcategoryFacade` (`GetSubcategoryType`). Usado no pagamento de
  fatura (E1) para que o lançamento de pagamento nasça com o tipo da subcategoria escolhida.

---

## Decisão: pagar fatura = marcar as COMPRAS como pagas (Opção 1, sem lançamento sintético)

Pagar uma fatura **não cria lançamento nenhum** — apenas marca as compras EM ABERTO dela
como `realizado` com a data informada. As próprias compras (com suas categorias) são as
despesas; o "pagamento" é derivado agrupando as compras pagas por `payment_date`. Não há
ledger nem lançamento "Pagamento de Fatura". Derivados na `Invoice`: `PaidAmount` (Σ
realizado), `OutstandingAmount` (Σ pendente), `PaymentStatus` (`nenhum/parcial/paga`),
`Payments` (`[{paymentDate, amount}]`). `StatusPaga` ⇔ não há compra em aberto e total > 0.

`PayInvoice` (`POST .../invoices/{ref}/pay`, body `{payment_date}`) marca **todas** as
compras em aberto da fatura como pagas naquela data (`InvoicePaymentWriter.MarkInvoicePaid`).
Sem valor — paga o saldo aberto do momento. Permitido em fatura **aberta/fechada/vencida**
(não `futura`) e só com saldo > 0. **Pagar de novo** depois (com novas compras) quita só as
que estiverem em aberto, gerando outro "lote" (outra data).

`UndoInvoicePayment` (`DELETE .../invoices/{ref}/payments/{paymentDate}`) volta para
`pendente` as compras pagas naquela data (`RevertInvoicePayment`).

`UsedLimit` = soma das compras **em aberto** (pendentes) das faturas não pagas; pagar libera
o limite na hora.

**Regime de caixa (D14, Opção 1):** uma compra de cartão entra no resumo de Lançamentos
pela **data de pagamento** (data efetiva de caixa) quando paga, com a categoria real — não
mais excluída. O Relatório por categoria continua acrual (compra na competência). Cada lente
conta a compra uma vez (caixa = mês do pagamento; relatório = mês da compra).

> **Exceção de posse de tabela (Opção A — igual ao `installment`):** o `InvoicePaymentWriter`
> escreve em `transactions` (posse do módulo `transaction`) — só muda status/payment_date das
> compras. O `creditcard` continua **não importando** o pacote `transaction`.
>
> **Histórico:** a migration 0011 (ledger) + 0012 (pagamento como despesa) foram superadas
> pela 0013, que dropa o ledger e remove os lançamentos sintéticos antigos.

---

## Regras de ciclo (RF-CC-04) — funções puras em `creditcard.go`

| Função | O que retorna |
|--------|---------------|
| `ClosingDate(ref, closingDay)` | data de fechamento (clamp p/ mês curto) |
| `DueDate(ref, closingDay, dueDay)` | vencimento; se `dueDay <= closingDay`, vence no mês seguinte |
| `CycleStart(ref, closingDay)` | dia seguinte ao fechamento anterior |
| `InvoiceReferenceFor(purchaseDate, closingDay)` | reference (YYYY-MM) a que a compra pertence |
| `BestPurchaseDay(closingDay)` | melhor dia de compra (cosmético) |
| `DeriveInvoiceStatus(...)` | `futura/aberta/fechada/vencida` (ou `paga` quando `pago ≥ total`) |

> Comparações de data usam **strings `YYYY-MM-DD` lexicograficamente** (formato
> zero-padded e ordenável) — sem aritmética de fuso. `clampDay` resolve dias
> inexistentes (ex.: fechamento dia 31 em fevereiro → 28/29).

---

## Utilização do limite (RF-CC-07 / D11)

- `UsedLimit` soma as faturas em `{aberta, fechada, vencida, futura}` — ou seja, **tudo
  que não foi pago**. Faturas `paga` não comprometem o limite. **`futura` passou a contar
  desde RF-PARC-10** (parcelamento): parcelas futuras comprometem o limite na hora da
  compra (R$5.000 em 10x reserva R$5.000 já; libera R$500 a cada fatura paga).
- `UtilizationPercent` = `usedLimit * 100 / creditLimit` (pode passar de 100%).
- `ClassifyUtilization`: `<30 saudavel`, `30..69 atencao`, `70..90 alto`, `>90 critico`.
- Compras **canceladas não contam** em nenhum agregado (`counts()` exclui `cancelado`).

---

## Indicadores derivados (D10) e resumo mensal

- `CreditCardDetail` = cartão + `BestPurchaseDay`, `UsedLimit`, `AvailableLimit`,
  `UtilizationPercent`/`Level` e `OpenInvoice` (fatura do ciclo corrente; nunca nil,
  pode estar zerada). Devolvido por Get e List.
- `BuildMonthlySummary` agrega por **competência** (mês civil), com total, count,
  ticket médio e breakdown por categoria.

---

## Padrão: use case structs + HandlerDeps

Cada use case é struct privada com construtor `NewXxx`, retornando a interface
pública (compile-time check em `usecases_test.go`). Os use cases de fatura recebem
os três ports: `CreditCardRepository`, `InvoicePaymentRepository` e
`CardTransactionReader`.

```go
type HandlerDeps struct {
    Create, Get, List, Update, Delete, Archive,
    ListInvoices, GetInvoice, AddPayment, UndoPayment, MonthSummary // (use cases)
}
```

---

## Paginação (defaults do módulo)

| Recurso | Limit | OrderBy | Order | allowedOrderBy |
|---------|-------|---------|-------|----------------|
| cartões | 100 | `created_at` | DESC | `name`, `created_at` |
| fatura (lançamentos do ciclo) | 100 | `competence_date` | ASC | — |

`List` filtra por estado de arquivamento (`archived=false` → ativos, default;
`true` → arquivados). **Não** há visão "todos" — a UI tem abas separadas.

---

## Tipos de erro (domainerr)

| Situação                                      | Erro                        | HTTP |
|-----------------------------------------------|-----------------------------|------|
| cartão não encontrado                         | `ErrCreditCardNotFound`     | 404  |
| fatura não encontrada                         | `ErrInvoiceNotFound`        | 404  |
| excluir cartão com lançamentos vinculados     | `ErrCardHasTransactions`    | 409  |
| vincular lançamento a cartão arquivado        | `ErrCannotLinkArchivedCard` | 409  |
| pagar fatura ainda não fechada                | `ErrInvoiceNotClosed`       | 409  |
| pagar fatura já paga                          | `ErrInvoiceAlreadyPaid`     | 409  |
| campo inválido/ausente (acumulado)            | `NewBadRequest`/`Composite` | 400  |
| erro inesperado                               | `NewInternal`               | 500  |

> `domainerr` não expõe 422; conflitos com o estado do recurso usam `Conflict` (409).

---

## Rotas registradas

| Método | Caminho                                                  | Handler             |
|--------|----------------------------------------------------------|---------------------|
| GET    | /api/credit-cards                                        | ListCreditCards     |
| POST   | /api/credit-cards                                        | CreateCreditCard    |
| GET    | /api/credit-cards/{id}                                   | GetCreditCard       |
| PUT    | /api/credit-cards/{id}                                   | UpdateCreditCard    |
| DELETE | /api/credit-cards/{id}                                   | DeleteCreditCard    |
| PATCH  | /api/credit-cards/{id}/archive                           | ArchiveCreditCard   |
| PATCH  | /api/credit-cards/{id}/unarchive                         | UnarchiveCreditCard |
| GET    | /api/credit-cards/{id}/summary                           | CardSummary         |
| GET    | /api/credit-cards/{id}/invoices                          | ListInvoices        |
| GET    | /api/credit-cards/{id}/invoices/{reference}              | GetInvoice          |
| POST   | /api/credit-cards/{id}/invoices/{reference}/pay                     | PayInvoice          |
| DELETE | /api/credit-cards/{id}/invoices/{reference}/payments/{paymentDate}  | UndoInvoicePayment  |

> `{reference}` é `YYYY-MM`. Excluir um cartão com lançamentos é bloqueado
> (`ErrCardHasTransactions`) — o caminho esperado é **arquivar**.
