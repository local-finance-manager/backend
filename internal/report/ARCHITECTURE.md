# Arquitetura — Módulo `report`

## Responsabilidade

Relatórios financeiros via **fechamento mensal materializado** (snapshot). Owner das
tabelas `report_monthly_closing` e `report_monthly_snapshot`. Cobre: fechar mês,
recalcular (janela de ajuste), bloqueio após 90 dias, relatório mensal (realizado e
projetivo), trimestral/semestral/anual (soma de snapshots), comparativos, KPIs e
insights. Cumpre o papel do módulo `report` reservado no guia (§3).

---

## Estrutura de arquivos

```
internal/report/
├── ARCHITECTURE.md   # este documento
├── report.go         # domínio puro: referências/datas, estados de bloqueio, erros
├── aggregate.go      # domínio: tipos de resposta + rollup (analítico), KPIs, comparativos, insights
├── repository.go     # interface Repository + tipo Closing + PORTS consumidos
├── sqlite.go         # adapter SQLite (closing/snapshot, soma de períodos, atômico)
├── service.go        # orquestração: Close, Recalculate, LockState/EnsureEditable/AfterChange, leituras
├── handler.go        # HTTP handler (camelCase — contrato Apêndice B)
├── routes.go         # rotas Chi v5
├── report_test.go    # testes de domínio (datas/estados, rollup, KPIs, deltas)
└── service_test.go   # testes de service + repo SQLite (:memory:) com fakes dos ports
```

---

## Camadas e dependências

```
HTTP (handler/routes) → Service → Repository (SQLite, owner) + Ports injetados
                                   └ domínio puro (report.go, aggregate.go)
```

O módulo **não importa** `transaction`/`category`/`creditcard`. Consome ports via
DTOs neutros em `shared` (`SubcategoryAggregate`, `MonthlyTotals`, `CategoryNode`).

---

## Decisão central: fechamento = snapshot (agregação materializada)

- **Mês fechado** → agregados congelados em `report_monthly_snapshot` (por subcategoria;
  categoria = `SUM ... GROUP BY category_id`). Totais do mês em `report_monthly_closing`.
- **Mensal fechado** = leitura direta do snapshot. **Mensal aberto** = cálculo ao vivo
  pelo `RealizedAggregator`.
- **Trimestral/semestral/anual** = soma dos snapshots dos meses **fechados** do intervalo
  (`SnapshotForRefs`/`ClosingsForRefs`). Nunca tocam a tabela de lançamentos. Meses não
  fechados são listados em `missingMonths`.

## Estados do mês (derivados, não armazenados)

| Estado | Condição |
|--------|----------|
| `aberto` | sem linha em `report_monthly_closing` |
| `fechado_ajustavel` | existe linha e `hoje <= hard_lock_at` (último dia + 90) |
| `fechado_bloqueado` | existe linha e `hoje > hard_lock_at` |

## Decisão: base do relatório = ACRUAL por competência, **incluindo cartão**

Diferente do resumo de lançamentos (regime de caixa, D14, que exclui cartão), o
relatório de gastos por categoria **inclui** as compras de cartão na competência em
que ocorreram — é o cerne do "onde estou sangrando dinheiro". `saldoInicial/saldoFinal`
seguem o conceito E6 (acumulado) na mesma base. A agregação é feita pelo
`transaction.ReportAggregator` (que conhece a tabela), não pelo report.

> **Limitação consciente do schema:** o snapshot (Apêndice A) não guarda forma de
> pagamento. Por isso `percentNoCredito` e a distribuição por forma de pagamento são
> calculados **ao vivo** (mini-query do `PaymentBreakdownReader`) e só no **mensal**;
> períodos longos os omitem.

---

## Ports (interfaces consumidas, injetadas no `main.go`)

| Port | Implementado por | Uso |
|------|------------------|-----|
| `RealizedAggregator` | `transaction.ReportAggregator` | agrega realizado do mês (fechar/recalcular/mensal aberto) |
| `PendingAggregator` | `transaction.ReportAggregator` | agrega pendentes (modo projetivo) |
| `PaymentBreakdownReader` | `transaction.ReportAggregator` | despesa por forma de pagamento (mensal) |
| `CategoryTreeReader` | `category.CategoryTreeFacade` | nomes/cores/tipos para compor a resposta |

Simetricamente, o `report.Service` satisfaz `transaction.MonthGuard`
(`EnsureEditable` + `AfterChange`), injetado no transaction para impor o bloqueio e
disparar o recálculo do snapshot ao alterar lançamento de mês fechado.

---

## Atomicidade e idempotência (RNF-REL-03/07)

`SaveClosing` roda numa transação: upsert do closing → `DELETE` do snapshot do mês →
`INSERT` das novas linhas. Fechar/recalcular o mesmo mês substitui (não duplica).
Recalcular o mesmo conjunto de lançamentos produz o mesmo snapshot.

---

## Rotas registradas

| Método | Caminho | Handler |
|--------|---------|---------|
| GET | /api/reports/monthly?reference=&mode= | Monthly |
| GET | /api/reports/quarterly?year=&quarter= | Quarterly |
| GET | /api/reports/semiannual?year=&half= | Semiannual |
| GET | /api/reports/annual?year= | Annual |
| GET | /api/reports/closings | ListClosings |
| POST | /api/reports/closings | CloseMonth |
| POST | /api/reports/closings/{reference}/recalculate | Recalculate |
| GET | /api/reports/closings/{reference}/lock-state | LockState |

---

## Tipos de erro (domainerr)

| Situação | Erro | HTTP |
|----------|------|------|
| fechar mês que não terminou | `ErrMonthNotEnded` | 409 |
| fechar mês já fechado | `ErrAlreadyClosed` | 409 |
| alterar lançamento de mês bloqueado | `ErrMonthBlocked` | 409 (spec sugeria 422; lib só tem 409) |
| referência inválida | `ErrInvalidReference` | 400 |
