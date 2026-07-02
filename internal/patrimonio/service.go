package patrimonio

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Views ────────────────────────────────────────────────────────────────────

// CaixinhaView é uma caixinha enriquecida com o saldo derivado e indicadores.
type CaixinhaView struct {
	Caixinha
	Saldo    int64  // aportes − resgates (guardado), centavos
	Progress *int   // bp 0..10000 rumo à meta (nil se sem meta)
	Ganho    *int64 // investimento: valorMercado − saldo (nil se n/a)
	Percent  int    // bp do total guardado (preenchido no Overview)
}

// Overview é o painel de patrimônio.
type Overview struct {
	PatrimonioTotal int64
	Disponivel      int64
	Guardado        int64
	GanhoTotal      int64
	Caixinhas       []CaixinhaView
}

// Extrato é a resposta paginada do extrato de uma caixinha.
type Extrato struct {
	Movimentos []shared.CaixinhaMovement
	Total      int
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Deps injeta o repositório e os ports do módulo transaction.
type Deps struct {
	Repo       Repository
	Movements  MovementReader
	Writer     MovementWriter
	Disponivel DisponivelReader
}

// Service orquestra os casos de uso do patrimônio.
type Service struct{ d Deps }

// NewService cria o service.
func NewService(d Deps) *Service { return &Service{d: d} }

// ─── CRUD ─────────────────────────────────────────────────────────────────────

// CreateCaixinha cria uma caixinha.
func (s *Service) CreateCaixinha(ctx context.Context, in CreateCaixinhaInput) (CaixinhaView, error) {
	if err := ValidateCreate(in); err != nil {
		return CaixinhaView{}, err
	}
	now := time.Now().UTC()
	c := Caixinha{
		ID:           uuid.New().String(),
		Name:         in.Name,
		Type:         in.Type,
		MetaValor:    in.MetaValor,
		DataAlvo:     in.DataAlvo,
		ValorMercado: in.ValorMercado,
		Color:        in.Color,
		Icon:         in.Icon,
		DisplayOrder: in.DisplayOrder,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.d.Repo.Create(ctx, c); err != nil {
		return CaixinhaView{}, err
	}
	return CaixinhaView{Caixinha: c, Saldo: 0, Progress: c.Progress(0), Ganho: c.GanhoInvestimento(0)}, nil
}

// UpdateCaixinha atualiza os campos mutáveis de uma caixinha.
func (s *Service) UpdateCaixinha(ctx context.Context, in UpdateCaixinhaInput) (CaixinhaView, error) {
	if err := ValidateUpdate(in); err != nil {
		return CaixinhaView{}, err
	}
	c, err := s.d.Repo.Get(ctx, in.ID)
	if err != nil {
		return CaixinhaView{}, err
	}
	c.Name = in.Name
	c.Type = in.Type
	c.MetaValor = in.MetaValor
	c.DataAlvo = in.DataAlvo
	c.ValorMercado = in.ValorMercado
	c.Color = in.Color
	c.Icon = in.Icon
	c.DisplayOrder = in.DisplayOrder
	c.UpdatedAt = time.Now().UTC()
	if err := s.d.Repo.Update(ctx, c); err != nil {
		return CaixinhaView{}, err
	}
	return s.buildView(ctx, c)
}

// GetCaixinha devolve uma caixinha com saldo/indicadores.
func (s *Service) GetCaixinha(ctx context.Context, id string) (CaixinhaView, error) {
	c, err := s.d.Repo.Get(ctx, id)
	if err != nil {
		return CaixinhaView{}, err
	}
	return s.buildView(ctx, c)
}

// ListCaixinhas lista as caixinhas (com saldo/indicadores).
func (s *Service) ListCaixinhas(ctx context.Context, includeArchived bool) ([]CaixinhaView, error) {
	cs, err := s.d.Repo.List(ctx, includeArchived)
	if err != nil {
		return nil, err
	}
	balances, err := s.d.Movements.BalancesAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CaixinhaView, 0, len(cs))
	for _, c := range cs {
		saldo := balances[c.ID]
		out = append(out, CaixinhaView{
			Caixinha: c, Saldo: saldo, Progress: c.Progress(saldo), Ganho: c.GanhoInvestimento(saldo),
		})
	}
	return out, nil
}

// ArchiveCaixinha arquiva/desarquiva.
func (s *Service) ArchiveCaixinha(ctx context.Context, id string, archived bool) error {
	return s.d.Repo.SetArchived(ctx, id, archived)
}

// DeleteCaixinha exclui uma caixinha; bloqueia se houver saldo.
func (s *Service) DeleteCaixinha(ctx context.Context, id string) error {
	if _, err := s.d.Repo.Get(ctx, id); err != nil {
		return err
	}
	saldo, err := s.d.Movements.BalanceByCaixinha(ctx, id)
	if err != nil {
		return err
	}
	if saldo != 0 {
		return ErrExcluirComSaldo
	}
	return s.d.Repo.Delete(ctx, id)
}

// AtualizarValorMercado atualiza o valor de mercado de uma caixinha de investimento.
func (s *Service) AtualizarValorMercado(ctx context.Context, in MarketValueInput) (CaixinhaView, error) {
	if err := ValidateMarketValue(in); err != nil {
		return CaixinhaView{}, err
	}
	c, err := s.d.Repo.Get(ctx, in.ID)
	if err != nil {
		return CaixinhaView{}, err
	}
	if c.Type != TypeInvestimento {
		return CaixinhaView{}, ErrValorMercadoTipo
	}
	if err := s.d.Repo.SetMarketValue(ctx, in.ID, in.ValorMercado, in.Data); err != nil {
		return CaixinhaView{}, err
	}
	c.ValorMercado = &in.ValorMercado
	c.DataValorMercado = &in.Data
	return s.buildView(ctx, c)
}

// ─── Movimentos ───────────────────────────────────────────────────────────────

// Aportar guarda dinheiro numa caixinha (reduz o disponível).
func (s *Service) Aportar(ctx context.Context, in MovementInput) (string, error) {
	if err := ValidateMovement(in); err != nil {
		return "", err
	}
	if _, err := s.d.Repo.Get(ctx, in.CaixinhaID); err != nil {
		return "", err
	}
	return s.d.Writer.Register(ctx, shared.NewCaixinhaMovement{
		CaixinhaID: in.CaixinhaID, Direction: DirectionAporte,
		Amount: in.Amount, Date: in.Date, Description: in.Description,
	})
}

// Resgatar libera dinheiro de uma caixinha (aumenta o disponível). Bloqueia se o
// valor exceder o saldo.
func (s *Service) Resgatar(ctx context.Context, in MovementInput) (string, error) {
	if err := ValidateMovement(in); err != nil {
		return "", err
	}
	if _, err := s.d.Repo.Get(ctx, in.CaixinhaID); err != nil {
		return "", err
	}
	saldo, err := s.d.Movements.BalanceByCaixinha(ctx, in.CaixinhaID)
	if err != nil {
		return "", err
	}
	if in.Amount > saldo {
		return "", ErrResgateExcedeSaldo
	}
	return s.d.Writer.Register(ctx, shared.NewCaixinhaMovement{
		CaixinhaID: in.CaixinhaID, Direction: DirectionResgate,
		Amount: in.Amount, Date: in.Date, Description: in.Description,
	})
}

// DefinirSaldoInicial estabelece o saldo GUARDADO inicial de uma caixinha (dinheiro que
// o usuário já tinha antes de usar o app), SEM mexer no disponível. Substitui qualquer
// saldo inicial anterior (apaga o antigo e cria o novo). Valor 0 remove o saldo inicial.
func (s *Service) DefinirSaldoInicial(ctx context.Context, caixinhaID string, valor int64, date string) error {
	if err := ValidateSaldoInicial(caixinhaID, valor, date); err != nil {
		return err
	}
	if _, err := s.d.Repo.Get(ctx, caixinhaID); err != nil {
		return err
	}
	prev, err := s.d.Movements.OpeningMovementIDs(ctx, caixinhaID)
	if err != nil {
		return err
	}
	for _, id := range prev {
		if err := s.d.Writer.Delete(ctx, id); err != nil {
			return err
		}
	}
	if valor <= 0 {
		return nil // só limpou o saldo inicial anterior
	}
	_, err = s.d.Writer.Register(ctx, shared.NewCaixinhaMovement{
		CaixinhaID: caixinhaID, Direction: DirectionAporte, Amount: valor, Date: date, Opening: true,
	})
	return err
}

// RegistrarRendimento adiciona um rendimento (juros/dividendos) ao guardado da caixinha,
// neutro ao disponível e sem contar como receita. Aditivo (cada rendimento é uma entrada).
func (s *Service) RegistrarRendimento(ctx context.Context, in MovementInput) (string, error) {
	if err := ValidateMovement(in); err != nil {
		return "", err
	}
	if _, err := s.d.Repo.Get(ctx, in.CaixinhaID); err != nil {
		return "", err
	}
	return s.d.Writer.Register(ctx, shared.NewCaixinhaMovement{
		CaixinhaID: in.CaixinhaID, Direction: DirectionAporte, Amount: in.Amount,
		Date: in.Date, Description: in.Description, Rendimento: true,
	})
}

// Extrato devolve o extrato paginado de uma caixinha.
func (s *Service) Extrato(ctx context.Context, id string, p shared.Pagination) (Extrato, error) {
	if _, err := s.d.Repo.Get(ctx, id); err != nil {
		return Extrato{}, err
	}
	movs, total, err := s.d.Movements.ListByCaixinha(ctx, id, p)
	if err != nil {
		return Extrato{}, err
	}
	return Extrato{Movimentos: movs, Total: total}, nil
}

// DeleteMovimento exclui um aporte/resgate pelo id do lançamento.
func (s *Service) DeleteMovimento(ctx context.Context, txID string) error {
	return s.d.Writer.Delete(ctx, txID)
}

// ─── Overview ─────────────────────────────────────────────────────────────────

// Overview monta o painel de patrimônio.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	cs, err := s.d.Repo.List(ctx, false)
	if err != nil {
		return Overview{}, err
	}
	balances, err := s.d.Movements.BalancesAll(ctx)
	if err != nil {
		return Overview{}, err
	}
	disponivel, err := s.d.Disponivel.DisponivelAtual(ctx)
	if err != nil {
		return Overview{}, err
	}

	var guardado, ganhoTotal int64
	for _, v := range balances {
		guardado += v
	}
	views := make([]CaixinhaView, 0, len(cs))
	for _, c := range cs {
		saldo := balances[c.ID]
		g := c.GanhoInvestimento(saldo)
		if g != nil {
			ganhoTotal += *g
		}
		views = append(views, CaixinhaView{
			Caixinha: c, Saldo: saldo, Progress: c.Progress(saldo), Ganho: g,
			Percent: percentBp(saldo, guardado),
		})
	}
	// percent depende do guardado total; recomputa agora que ele está fechado
	for i := range views {
		views[i].Percent = percentBp(views[i].Saldo, guardado)
	}

	return Overview{
		PatrimonioTotal: disponivel + guardado + ganhoTotal,
		Disponivel:      disponivel,
		Guardado:        guardado,
		GanhoTotal:      ganhoTotal,
		Caixinhas:       views,
	}, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *Service) buildView(ctx context.Context, c Caixinha) (CaixinhaView, error) {
	saldo, err := s.d.Movements.BalanceByCaixinha(ctx, c.ID)
	if err != nil {
		return CaixinhaView{}, err
	}
	return CaixinhaView{
		Caixinha: c, Saldo: saldo, Progress: c.Progress(saldo), Ganho: c.GanhoInvestimento(saldo),
	}, nil
}

func percentBp(part, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(part * 10000 / total)
}
