# Arquitetura — Módulo `transaction`

## Responsabilidade

Gerencia os lançamentos financeiros (despesas, receitas e transferências).
Fornece CRUD completo, confirmação de lançamento (`pendente → realizado`),
listagem filtrada com **sumário financeiro agregado** no mesmo payload, e expõe
um leitor de compras por cartão para o módulo `creditcard`.

O **tipo** do lançamento (`despesa`/`receita`/`transferencia`) não é informado pelo
cliente — é **derivado da subcategoria** via facade do módulo `category`.

---

## Estrutura de arquivos

```
internal/transaction/
├── ARCHITECTURE.md       # este documento
├── transaction.go        # domínio: entidade, enums, transições de status, inputs, erros, validação
├── repository.go         # interfaces: TransactionRepository + ports cross-module (SubcategoryFacade, CreditCardChecker)
├── usecases.go           # interfaces de caso de uso
├── usecases_impl.go      # implementações: Get, List, Create, Update, Confirm, Delete
├── card_reader.go        # adapter produtor: CardReader (lê transactions p/ o módulo creditcard)
├── sqlite.go             # adapter SQLite do TransactionRepository
├── handler.go            # HTTP handler + HandlerDeps + parse de filtros + conversão domain → response
├── routes.go             # registro de rotas Chi v5
├── transaction_test.go   # testes de domínio (validações, transições)
├── usecases_test.go      # testes de casos de uso com fakes + compile-time checks
└── sqlite_test.go        # testes de integração SQLite (:memory:)
```

---

## Camadas e dependências

```
HTTP (handler.go / routes.go)
        │  depende de
Use Cases (usecases.go + usecases_impl.go)
        │  depende de
Ports (repository.go: TransactionRepository, SubcategoryFacade, CreditCardChecker)
        │  implementado por
SQLite Adapter (sqlite.go) + facades injetados (category, creditcard)
        │  depende de
domain (transaction.go) + shared (shared/pagination.go, shared/cardtxn.go)
```

A camada de domínio não importa nada interno ao projeto (apenas `shared` para
`Pagination`). Use cases dependem só das interfaces de `repository.go`, nunca de
concreto.

---

## Decisão: tipo derivado da subcategoria (cross-module)

O cliente envia `subcategory_id`; o `type` é resolvido no use case chamando
`SubcategoryFacade.GetSubcategoryType`. A interface mora **no consumidor**
(`transaction`) e devolve uma `string` primitiva — sem acoplar tipos com o pacote
`category`. A implementação concreta é `category.SubcategoryFacade`, injetada no
`main.go`.

- **Create**: sempre deriva o tipo.
- **Update**: só re-deriva quando a `subcategory_id` mudou (otimização — evita ida
  ao banco quando o tipo não pode ter mudado).

> Regra de negócio relacionada: `ErrTypeChangeForbidden` documenta que o tipo de um
> lançamento não muda diretamente — só indiretamente ao trocar de subcategoria.

---

## Decisão: vínculo com cartão de crédito (cross-module)

Quando `credit_card_id` é informado, o use case valida o vínculo via
`CreditCardChecker.CheckLinkable` (cartão existe e está ativo). A interface é
definida aqui (consumidor) e devolve apenas `error` (domainerr) — sem acoplamento
de tipo. Implementada por `creditcard.NewCreditCardChecker` e injetada no `main.go`.

Regra de domínio (validação): `credit_card_id` só pode ser preenchido quando
`payment_method = cartao_credito`.

A direção inversa (creditcard lendo as compras) usa `CardReader` (ver abaixo).

---

## Decisão: `CardReader` como adapter produtor

`CardReader` (`card_reader.go`) lê a tabela `transactions` — dona da coluna
`credit_card_id` — e devolve a projeção neutra `shared.CardTransaction`. Satisfaz
`creditcard.CardTransactionReader` por **structural typing** (sem import entre os
módulos). É definido aqui (produtor) e injetado no `main.go`.

Mantém a regra do monolito modular: `transaction` não importa `creditcard` e
vice-versa; a ponte é `shared.CardTransaction` + injeção no Composition Root.

> **Exceção de escrita (E1 — pagamento de fatura):** ao registrar o pagamento de uma fatura,
> o módulo `creditcard` escreve em `transactions` **dentro da própria `tx`** (baixa em lote
> das compras → `realizado` + criação do lançamento de pagamento), para garantir atomicidade
> cross-module (Opção A, como o `installment`). É a única exceção em que outro módulo grava
> nesta tabela; ver `creditcard/ARCHITECTURE.md` (Decisão E1).

---

## Padrão: use case structs individuais

Cada caso de uso é uma struct privada que implementa uma interface pública,
construída por `NewXxx`. Compile-time satisfaction verificada em `usecases_test.go`.
Casos que precisam de cross-module recebem os facades no construtor:

```go
func NewCreateTransaction(
    repo TransactionRepository,
    facade SubcategoryFacade,
    cardChecker CreditCardChecker,
) CreateTransactionUseCase
```

---

## Injeção de dependência via HandlerDeps

```go
type HandlerDeps struct {
    GetTransaction     GetTransactionUseCase
    ListTransactions   ListTransactionsUseCase
    CreateTransaction  CreateTransactionUseCase
    UpdateTransaction  UpdateTransactionUseCase
    ConfirmTransaction ConfirmTransactionUseCase
    DeleteTransaction  DeleteTransactionUseCase
}
```

Cada campo é uma interface — substituível por fake nos testes.

---

## Listagem: dados + sumário no mesmo payload

`ListTransactions` **não** usa `shared.PagedResult[T]` porque carrega um sumário
financeiro junto da página. A resposta é:

```json
{
  "data": [ ... ],
  "summary": {
    "totalDespesas": 0,
    "totalReceitas": 0,
    "saldoPeriodo": 0,
    "totalPendente": 0,
    "countTotal": 0,
    "saldoInicial": 0,
    "saldoFinal": 0
  },
  "pagination": { "page": 1, "limit": 50, "total": 0, "total_pages": 0, "sort": "...", "sort_dir": "..." }
}
```

- `total`/`total_pages` vêm de `summary.CountTotal` (mesmo filtro, sem `LIMIT`).
- O sumário considera apenas lançamentos `realizado` para receitas/despesas;
  `totalPendente` soma os `pendente` de qualquer tipo.

### Saldo acumulado (E6 — `saldoInicial` / `saldoFinal`)

Além do **fluxo do período** (`saldoPeriodo = totalReceitas − totalDespesas`), o resumo expõe
o **saldo acumulado** (running balance), distinto do fluxo:

- `saldoInicial` = **carryover** (Σ `receita − despesa` de lançamentos `realizado`, sem cartão,
  com `competence_date < competence_date_from`) **+** **ajustes de saldo** (Σ `amount` de
  lançamentos `realizado` em subcategorias com `is_balance_adjustment`, até `competence_date_to`).
- `saldoFinal` = `saldoInicial + saldoPeriodo`.
- Os **ajustes de saldo** (E6/`category.is_balance_adjustment`) são `transferencia` → já ficam
  fora de `totalReceitas`/`totalDespesas`; o flag os inclui no saldo acumulado (RF-SALDO-02/03).
- `saldoInicial`/`saldoFinal` usam **apenas as datas** do filtro (ignoram tipo/categoria/cartão/
  busca) — um "saldo" filtrado por categoria não faria sentido. `GetSummary` faz 3 agregações
  (período, carryover, ajustes); ver `carryoverBalance`/`adjustmentsTotal` em `sqlite.go`.

### Paginação (defaults do módulo)

- `Limit = 50`
- `OrderBy = "competence_date"`, `Order = "DESC"` (data de competência é a ordenação natural)
- `allowedOrderBy = [competence_date, payment_date, amount, created_at, title]`

### Filtros (query params, parseados no handler)

`type`, `status`, `payment_method`, `subcategory_id`, `category_id`, `account_id`,
`competence_date_from/to`, `payment_date_from/to`, `search` (LIKE em `title`),
`credit_card_id`. Datas de filtro são parâmetros próprios — **não** usam os campos
de data do `shared.Pagination`.

---

## Máquina de status

```
pendente   → realizado, cancelado
realizado  → pendente, cancelado
cancelado  → pendente            (cancelado → realizado é PROIBIDO; passe por pendente)
```

- Transições validadas por `CanTransitionTo` no use case (precisa do estado atual).
- `Confirm` é o atalho `* → realizado` (exige `payment_date`); rejeita se a
  transição não for permitida a partir do status atual.
- Regra de `payment_date`: **obrigatória** quando `status = realizado`; **nula** caso
  contrário (validada com checks condicionais — ver guia §5.1).

---

## Dinheiro e datas

- `Amount` em **centavos** (`int64`), sempre positivo. Nunca float.
- `competence_date` / `payment_date` são strings `YYYY-MM-DD` (data de negócio,
  sem fuso). `created_at`/`updated_at` são `time.Time` serializados em RFC3339/UTC.

---

## Tipos de erro (domainerr)

| Situação                                   | Erro                       | HTTP |
|--------------------------------------------|----------------------------|------|
| lançamento não encontrado                  | `ErrTransactionNotFound`   | 404  |
| campo inválido/ausente (acumulado)         | `NewBadRequest`/`Composite`| 400  |
| transição de status inválida               | `ErrInvalidTransition`     | 400  |
| tentativa de trocar o tipo diretamente     | `ErrTypeChangeForbidden`   | 400  |
| cartão inexistente/arquivado (CheckLinkable) | erro do `creditcard`     | 404/409 |
| erro inesperado                            | `NewInternal`              | 500  |

---

## Rotas registradas

| Método | Caminho                              | Handler             |
|--------|--------------------------------------|---------------------|
| GET    | /api/transactions                    | ListTransactions    |
| POST   | /api/transactions                    | CreateTransaction   |
| GET    | /api/transactions/{id}               | GetTransaction      |
| PUT    | /api/transactions/{id}               | UpdateTransaction   |
| DELETE | /api/transactions/{id}               | DeleteTransaction   |
| PATCH  | /api/transactions/{id}/confirm       | ConfirmTransaction  |
| PATCH  | /api/transactions/{id}/cancel        | CancelTransaction   |

> `PUT` tem semântica de full-replace dos campos mutáveis.
> `PATCH .../confirm` é a transição dedicada para `realizado`; `PATCH .../cancel` para `cancelado`.

---

## Integração com `report` (relatórios/fechamento)

- **Produz** `ReportAggregator` (`report_aggregator.go`): agrega a tabela `transactions`
  por mês (realizado/pendente) + saldo acumulado (E6) e a distribuição de despesa por
  forma de pagamento. Satisfaz `report.RealizedAggregator`/`PendingAggregator`/
  `PaymentBreakdownReader` por structural typing (retorna `shared.*`). Base **acrual,
  incluindo cartão** (≠ do regime de caixa D14 do resumo) — ver `report/ARCHITECTURE.md`.
- **Consome** `MonthGuard` (port, definido em `repository.go`; implementado por
  `report.Service`, injetado no `main.go`, **nil-safe**): os use cases de mutação
  (create/update/confirm/cancel/delete) chamam `EnsureEditable` (bloqueia mês fechado-
  bloqueado, RF-REL-10) antes da escrita e `AfterChange` (recalcula o snapshot de mês
  fechado-ajustável, RF-REL-09) depois. Update/edição que cruza meses valida e recalcula
  **ambos** os meses (origem e destino).

---

## Parcelamento (colunas de installment)

A tabela `transactions` tem 3 colunas nullable de parcelamento (migration `0007`):
`installment_group_id` (FK → `installment_groups`, `ON DELETE CASCADE`),
`installment_number` (k) e `installment_total` (N).

- **Quem escreve as parcelas:** o módulo `internal/installment` (não o `transaction`).
  As parcelas são lançamentos `pendente`/`cartao_credito`/`despesa` gerados atomicamente
  pelo `installment` na mesma `tx` do grupo. Ver `installment/ARCHITECTURE.md`.
- **Aqui:** `Create`/`Update` à vista deixam essas colunas `NULL`; o `scanDetail` e os
  `SELECT` (`getSQL`/`listBaseSQL`) as leem e o handler as expõe (`installment_group_id`,
  `installment_number`, `installment_total`) para a UI mostrar "k/N" e linkar ao grupo.
- **Filtro:** `?installment_group_id=` lista as parcelas de uma compra.
