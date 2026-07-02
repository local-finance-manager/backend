# Módulo `patrimonio` — Arquitetura

Gestão de Patrimônio: **caixinhas** (envelopes) de saldo persistente construído por
**aporte** (guardar) e **resgate** (liberar). Ver requisitos em
`requisitos/gestao-patrimonio-caixinhas.md`.

## Responsabilidade e fronteiras

- **Owner** da tabela `caixinhas` (definição, metas, arquivamento).
- **NÃO** é dono dos movimentos: aporte/resgate são **lançamentos neutros** no livro-caixa
  do módulo `transaction` (`type=transferencia`, `caixinha_id` preenchido, subcategoria de
  sistema que carrega a direção em `subcategories.caixinha_direction`).
- Fala com `transaction` **só por ports** (nunca importa o módulo):
  - `MovementWriter` — cria/exclui o lançamento do movimento (impl.: `transaction.CaixinhaWriter`).
  - `MovementReader` — lê extrato e saldos derivados (impl.: `transaction.CaixinhaReader`).
  - `DisponivelReader` — saldo disponível de caixa corrente (impl.: `transaction.CaixinhaReader`).
- DTOs neutros em `internal/shared/caixinha.go` (`CaixinhaMovement`, `NewCaixinhaMovement`).

## Modelo de saldo (o porquê)

O saldo de uma caixinha **não é persistido**: é derivado (`Σ aportes − Σ resgates`), lido via
`MovementReader.BalanceByCaixinha`. Isso evita saldo duplicado/inconsistente — uma fonte de
verdade (os lançamentos). O **disponível** (caixa gastável) é calculado no `transaction.GetSummary`:
`aporte` reduz, `resgate` aumenta (`Summary.MovimentacaoCaixinhas`, entra no `SaldoFinal`). Assim
o resgate "volta a virar saldo" para pagar despesas, e o transporte de saldo de fim de mês (E6)
carrega o disponível já líquido dos aportes/resgates.

## Arquivos

| Arquivo | Papel |
|---|---|
| `patrimonio.go` | Domínio: `Caixinha`, enums, validação, `Progress`, `GanhoInvestimento`, erros. |
| `repository.go` | Interface `Repository` (owner de `caixinhas`) + ports consumidos. |
| `sqlite.go` | `SQLiteRepository` (CRUD de caixinhas). |
| `service.go` | Casos de uso: CRUD, Aporte/Resgate (com invariante de saldo), Overview, Extrato, valor de mercado. |
| `handler.go` | HTTP (snake_case), mapeamento views → resposta, erros via `domainerr`. |
| `routes.go` | Rotas chi sob `/api/patrimonio`. |

## Invariantes

- Resgate **não** pode exceder o saldo (`ErrResgateExcedeSaldo`, 409).
- Excluir caixinha exige **saldo zero** (`ErrExcluirComSaldo`, 409); senão, arquivar.
- Valor de mercado só em caixinha `investimento` (`ErrValorMercadoTipo`, 409).
- Dinheiro em centavos `int64`; percentuais/metas em pontos-base; sem float.

## Fora do escopo v1

Transferência entre caixinhas; cotação automática de ações (valor de mercado é manual);
reserva de pagamento do cartão (a Opção 1 do cartão já cobre). Ver §10/§11 do requisito.
