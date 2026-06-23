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
├── repository.go       # interfaces: CreditCardRepository, InvoicePaymentRepository, CardTransactionReader (port)
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
4. O pagamento (se houver) vem do `InvoicePaymentRepository` e define `StatusPaga`.

---

## Decisão (D1/D7): leitura e validação cross-module

`creditcard` **não importa** `transaction` e vice-versa (regra do monolito modular,
guia §3.3). A ponte é feita por interfaces + um DTO neutro em `shared`:

- **Lê lançamentos** via port `CardTransactionReader` (definido aqui, consumidor).
  Implementado por `transaction.CardReader` (produtor) e injetado no `main.go`.
  O retorno é `shared.CardTransaction` — data-carrier puro comum aos dois módulos.
- **Valida vínculo** expondo `CreditCardChecker` (`facade.go`, produtor): satisfaz
  `transaction.CreditCardChecker` por structural typing. `CheckLinkable` retorna
  `ErrCreditCardNotFound`/`ErrCannotLinkArchivedCard` ou `nil`.

---

## Regras de ciclo (RF-CC-04) — funções puras em `creditcard.go`

| Função | O que retorna |
|--------|---------------|
| `ClosingDate(ref, closingDay)` | data de fechamento (clamp p/ mês curto) |
| `DueDate(ref, closingDay, dueDay)` | vencimento; se `dueDay <= closingDay`, vence no mês seguinte |
| `CycleStart(ref, closingDay)` | dia seguinte ao fechamento anterior |
| `InvoiceReferenceFor(purchaseDate, closingDay)` | reference (YYYY-MM) a que a compra pertence |
| `BestPurchaseDay(closingDay)` | melhor dia de compra (cosmético) |
| `DeriveInvoiceStatus(...)` | `futura/aberta/fechada/vencida` (ou `paga` se houver pagamento) |

> Comparações de data usam **strings `YYYY-MM-DD` lexicograficamente** (formato
> zero-padded e ordenável) — sem aritmética de fuso. `clampDay` resolve dias
> inexistentes (ex.: fechamento dia 31 em fevereiro → 28/29).

---

## Utilização do limite (RF-CC-07 / D11)

- `UsedLimit` soma **apenas** as faturas em `{aberta, fechada, vencida}`. Faturas
  `paga` e `futura` não comprometem o limite.
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
    ListInvoices, GetInvoice, PayInvoice, UndoPayment, MonthSummary // (use cases)
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
| PATCH  | /api/credit-cards/{id}/invoices/{reference}/pay          | PayInvoice          |
| DELETE | /api/credit-cards/{id}/invoices/{reference}/pay          | UndoInvoicePayment  |

> `{reference}` é `YYYY-MM`. Excluir um cartão com lançamentos é bloqueado
> (`ErrCardHasTransactions`) — o caminho esperado é **arquivar**.
