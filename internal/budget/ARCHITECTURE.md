# Arquitetura — Módulo `budget` (Gestão e Alocação de Receitas)

## Responsabilidade

Plano mensal de alocação da renda. O usuário cria **destinos** (fatias da renda do mês,
por percentual ou valor fixo) e, quando **toda** a renda do mês está realizada,
**materializa** cada destino em um lançamento real (despesa, ou investimento →
transferência que reduz o saldo disponível). Owner das tabelas `allocation_destination`,
`allocation_template` e `allocation_template_item`. Inclui templates (ex.: 50/30/20) e
copiar destinos do mês anterior.

---

## Estrutura de arquivos

```
internal/budget/
├── ARCHITECTURE.md   # este documento
├── budget.go         # domínio puro: Kind/Mode, ComputePlan (rateio cent-exato A2), validações
├── repository.go     # interface Repository + tipos Template/Item + PORTS consumidos
├── sqlite.go         # adapter SQLite (destinos + templates; SetMaterialized condicional)
├── service.go        # orquestração: plano, CRUD, materializar/desfazer/bulk, templates, copiar mês
├── handler.go        # HTTP handler (camelCase — contrato Apêndice B)
├── routes.go         # rotas Chi v5 sob /api/income
├── budget_test.go    # domínio: rateio exato, validações
├── service_test.go   # service + repo SQLite (:memory:) com fakes dos ports
├── coverage_test.go  # ramos de erro do service (overrides, compensação, templates)
└── routes_test.go    # stack HTTP completo (handler+service+sqlite) + erros de repo
```

---

## Camadas e dependências

```
HTTP (handler/routes) → Service → Repository (SQLite, owner) + Ports injetados
                                   └ domínio puro (budget.go)
```

O módulo **não importa** `transaction`/`category`/`report`. Consome ports via DTOs
neutros em `shared` (`IncomeItem`, `NewTransaction`).

---

## Decisão central: materializar = criar lançamento + vincular, atômico por compensação

Um destino planejado vira um lançamento real via o port `TransactionWriter` (que passa
pelos use cases de `transaction` — derivam o tipo da subcategoria, validam e aplicam o
**guard de mês fechado**). Em seguida o vínculo é gravado com um `UPDATE ... WHERE
materialized_transaction_id IS NULL` (idempotência otimista). Se o vínculo falhar
(corrida → 0 linhas), o lançamento recém-criado é **excluído** (compensação) para não
deixar órfão. Não há transação distribuída entre os dois módulos; a consistência é
garantida por compensação no serviço.

## Cálculo do plano: % sobre a renda total + rateio de centavos exato

`ComputePlan` aplica cada **percentual sobre a base inteira** (renda total do mês):
`valor = round(percentage/10000 × base)` (Apêndice C). Os destinos de **valor fixo**
são o próprio `fixedAmount`. A soma dos percentuais é distribuída pelo método do
**maior resto** (`distributePercent`), garantindo que a soma dos centavos atribuídos
seja exatamente igual ao total percentual aplicado à base (sem perder/criar centavo).
Tudo em inteiros — dinheiro em centavos, percentual em pontos-base (10000 = 100%).

> **Nota (revisão da decisão A2):** a v1 do spec tinha A2 = "fixos consomem a base
> antes; % sobre o restante", o que contradizia a fórmula da Apêndice C. Decisão
> revista (com o usuário): **percentuais incidem sobre a renda total**; os fixos
> somam por cima. A validação garante que `fixos + (Σ% × base)` não excede a renda.

## Decisão A4: subcategoria default de investimento

Investimento sem `presetSubcategoryId` usa `sub-trf-aporte` ("Aporte em Investimentos",
seed de transferência) — injetado no `main.go` como `InvestSubcatID`. Por isso um
investimento **entra** no materializar-em-lote mesmo sem preset; uma despesa sem preset
é **pulada** (listada em `skipped`).

## Disponível vs. alocado

- **Alocado / não alocado** = soma dos destinos (planejados + materializados) vs. resto
  da renda — visão de planejamento.
- **Disponível** = renda − valores **materializados** (despesas e investimentos já
  executados) — visão de caixa.

---

## Ports (interfaces consumidas, injetadas no `main.go`)

| Port | Implementado por | Uso |
|------|------------------|-----|
| `IncomeReader` | `transaction.IncomeReader` | renda do mês (receitas por competência, exclui ajustes de saldo), se tudo realizado |
| `TransactionWriter` | `transaction.BudgetWriter` | cria/exclui lançamento via use cases (herda o guard de mês fechado) |

Isolamento simétrico ao `report`: o `budget` define os ports; os produtores ficam no
módulo dono da tabela (`transaction`) e são ligados no Composition Root.

---

## Regra de materialização (RF-ALOC)

- Materializar exige **`allRealized && total > 0`** (toda a renda do mês realizada) →
  senão `ErrIncomePending`.
- Destino já materializado não materializa de novo → `ErrAlreadyMaterialized`.
- Editar/excluir destino materializado é bloqueado (desfaça antes) → `ErrAlreadyMaterialized`.
- Desfazer exclui o lançamento via `TransactionWriter.Delete` (respeita o bloqueio de
  mês fechado do `report`) e zera o vínculo.
- Superalocação (fixos > base, ou Σpercentual > 100%) é bloqueada na criação/edição →
  `ErrOverAllocated` (decisão A1).
- Templates e copiar mês criam destinos **planejados** (nunca materializam).

---

## Rotas registradas (sob `/api/income`)

| Método | Caminho | Handler |
|--------|---------|---------|
| GET | /plan?reference=YYYY-MM | GetPlan |
| POST | /destinations | CreateDestination |
| PUT | /destinations/{id} | UpdateDestination |
| DELETE | /destinations/{id} | DeleteDestination |
| POST | /destinations/{id}/materialize | Materialize |
| DELETE | /destinations/{id}/materialize | Undo |
| POST | /plan/{reference}/materialize-all | MaterializeAll |
| POST | /plan/{reference}/apply-template | ApplyTemplate |
| POST | /plan/{reference}/copy-previous | CopyPrevious |
| GET | /templates | ListTemplates |
| POST | /templates | CreateTemplate |

---

## Tipos de erro (domainerr)

| Situação | Erro | HTTP |
|----------|------|------|
| renda pendente ao materializar | `ErrIncomePending` | 409 (spec sugeria 422; lib só tem 409) |
| superalocação (> 100% ou fixos > base) | `ErrOverAllocated` | 409 |
| destino já materializado | `ErrAlreadyMaterialized` | 409 |
| desfazer destino não materializado | `ErrNotMaterialized` | 404/409 |
| destino/template inexistente | `ErrDestinationNotFound` / `ErrTemplateNotFound` | 404 |
| materializar sem subcategoria (despesa sem preset, sem override) | `ErrMissingPreset` | 409 |
