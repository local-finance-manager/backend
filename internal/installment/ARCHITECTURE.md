# Arquitetura — Módulo `installment`

## Responsabilidade

Gerencia **compras parceladas no cartão de crédito**. Cada parcela **é um lançamento
normal** na tabela `transactions` (com `credit_card_id`, competência própria, status);
o grupo (`installment_groups`) guarda o metadado imutável da compra. Por isso faturas,
resumos, filtros e anti-dupla-contagem funcionam **sem alteração** — o módulo apenas
gera as parcelas (atomicamente) e expõe a visão de grupo.

Faz: rateio exato de centavos, cronograma de competências (com clamp de mês), preview,
criação atômica, listagem/detalhe, edição da série, cancelar restantes, excluir, juros.
Não faz: alocar parcela à fatura (isso é do `creditcard`, via `competence_date`).

---

## Estrutura de arquivos

```
internal/installment/
├── ARCHITECTURE.md       # este documento
├── installment.go        # domínio: tipos, rateio, cronograma, juros, status, validação, erros
├── repository.go         # ports: Repository + SubcategoryReader + CreditCardChecker + InvoiceReferenceResolver
├── service.go            # casos de uso: Preview, Create, List, Get, UpdateSeries, CancelRemaining, Delete
├── sqlite.go             # adapter SQLite: grupo + parcelas (mesma tx); list/get/saldo; cancel; delete
├── handler.go            # HTTP handler + req/resp (snake_case) + conversão domínio → response
├── routes.go             # registro de rotas Chi v5
├── installment_test.go   # testes de domínio (rateio com invariante Σ=total, cronograma, validação)
├── service_test.go       # testes dos casos de uso com fakes dos 4 ports
└── sqlite_test.go        # testes SQLite (:memory:): geração atômica, cascade, saldo, cancel
```

---

## Camadas e dependências

```
HTTP (handler.go / routes.go)
        │
Service (service.go)
        │  depende de
Ports (repository.go: Repository + ports cross-module)
        │  implementado por
SQLite Adapter (sqlite.go)  +  facades de outros módulos (injetados no main.go)
        │
domínio (installment.go) + shared (pagination)
```

O domínio (`installment.go`) não importa nada interno. O módulo **não importa**
`internal/creditcard` nem `internal/category` — fala com eles por ports (ISP).

---

## Decisão central: o módulo escreve na tabela `transactions` (Opção A do requisito)

`installment` **possui** `installment_groups`, mas as **parcelas são linhas em
`transactions`** (tabela "do" módulo `transaction`). Para a geração ser atômica
(grupo + N parcelas tudo-ou-nada), o `SQLiteRepository` deste módulo faz `INSERT`/
`UPDATE`/`SELECT` em `transactions` dentro de uma **única `tx` (`BeginTx`)**.

É uma exceção **consciente e contida** à posse estrita de tabela: coordenar uma
transação de banco entre dois módulos seria pior (vazar `*sql.Tx`, acoplar). O módulo
`transaction` segue dono das leituras gerais (faturas, resumos, CRUD à vista);
`installment` toca `transactions` apenas para o que é seu (gerar/cancelar/contar parcelas).

> **Manutenção:** se as colunas obrigatórias de `transactions` mudarem, o `INSERT` de
> parcela em `sqlite.go` precisa acompanhar (colunas: id, title, amount, type,
> subcategory_id, payment_method, status, competence_date, credit_card_id,
> installment_group_id, installment_number, installment_total, created_at, updated_at).

---

## Integrações cross-module (ports injetados no `main.go`)

| Port (consumidor) | Implementado por | Para quê |
|---|---|---|
| `SubcategoryReader.GetSubcategoryType` | `category.SubcategoryFacade` (reuso) | validar `type=despesa` e definir o type da parcela |
| `CreditCardChecker.CheckLinkable` | `creditcard.CreditCardChecker` (reuso) | cartão existe e não está arquivado |
| `InvoiceReferenceResolver.ReferencesFor` | `creditcard.InvoiceReferenceFacade` (novo) | resolver a `reference` (YYYY-MM) de cada competência reusando o ciclo |

Todos satisfeitos por **structural typing** — nenhum import entre os pacotes internos.

> **Impacto no `usedLimit` do cartão (RF-PARC-10):** `creditcard.UsedLimit` passou a
> incluir faturas `futura`, de modo que parcelas futuras comprometem o limite na hora
> da compra. Isso vive no `creditcard` (não aqui), pois parcelas são transações de cartão
> comuns — o `creditcard` não precisa conhecer parcelamento.

---

## Domínio: rateio e cronograma (puros, exaustivamente testados)

- **Rateio (`ComputeAmounts`)**: `base = total/N`; as `resto = total%N` primeiras parcelas
  recebem `base+1`. **Invariante: `Σ parcelas == total`** sempre (testada para muitos
  `(total, N)`). Tudo `int64` (centavos).
- **`ResolvePlan`**: `by_total` usa o rateio; `by_installment` → `total = parcela × N`,
  todas iguais.
- **Cronograma (`CompetenceSchedule`)**: parcela `k` → `purchaseDate + (k-1) meses`, com
  clamp de mês curto (31/jan +1 → 28/29-fev).
- **`Interest`**: `total − principal` (quando `principal` informado e `total > principal`).
- **`DeriveGroupStatus`**: há pendente → `ativo`; sem pendente com cancelada → `cancelado`;
  todas realizadas → `quitado`.

---

## Geração atômica

`Repository.Create` abre `BeginTx`, insere o grupo, insere as N parcelas com prepared
statement e dá `Commit`. Falha em qualquer parcela → `Rollback` total (nunca uma compra
pela metade). `cancel-remaining` e `update-series` também rodam em `tx`.

---

## Paginação

`GET /api/installments` usa `shared.Pagination`. Defaults do módulo: `Limit=100`,
`OrderBy="created_at"`, `Order="DESC"`. `allowedOrderBy = [purchase_date, created_at, total_amount]`.
O filtro de status (`ativo`/`quitado`/`cancelado`) é aplicado via `HAVING` sobre os
agregados das parcelas. As parcelas de um grupo (`GET /:id`) **não** são paginadas
(coleção pequena, ≤ 72).

---

## Tipos de erro (domainerr)

| Situação | Erro | HTTP |
|---|---|---|
| grupo não encontrado | `ErrInstallmentGroupNotFound` (`NewNotFound`) | 404 |
| subcategoria não é despesa | `ErrOnlyExpensesInstallable` (`NewConflict`) | 409 |
| editar valor/parcelas/data da série | `ErrImmutableSeriesField` (`NewBadRequest`) | 400 |
| cartão inexistente/arquivado | propagado do `creditcard` (`NotFound`/`Conflict`) | 404/409 |
| validação de input (N, valores, datas) | acumulador → `BadRequest`/`Composite` | 400 |
| erro inesperado / falha de repo | `NewInternal` | 500 |

---

## Rotas registradas

| Método | Caminho | Handler |
|---|---|---|
| POST | /api/installments/preview | Preview (simula, não grava) |
| POST | /api/installments | Create (grupo + N parcelas, atômico) |
| GET | /api/installments | List (paginado; filtro credit_card_id, status) |
| GET | /api/installments/{id} | Get (grupo + parcelas) |
| PUT | /api/installments/{id} | UpdateSeries (title/description/subcategory) |
| PATCH | /api/installments/{id}/cancel-remaining | CancelRemaining |
| DELETE | /api/installments/{id} | Delete (cascade nas parcelas) |

> `/preview` (literal) é registrado antes de `/{id}` para a prioridade do radix tree do Chi.
