package patrimonio_test

import (
	"context"
	"errors"
	"testing"

	"github.com/local-finance-manager/backend/internal/patrimonio"
	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

type fakeRepo struct {
	items map[string]patrimonio.Caixinha
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]patrimonio.Caixinha{}} }

func (r *fakeRepo) Create(_ context.Context, c patrimonio.Caixinha) error {
	r.items[c.ID] = c
	return nil
}
func (r *fakeRepo) Get(_ context.Context, id string) (patrimonio.Caixinha, error) {
	c, ok := r.items[id]
	if !ok {
		return patrimonio.Caixinha{}, patrimonio.ErrCaixinhaNotFound
	}
	return c, nil
}
func (r *fakeRepo) List(_ context.Context, includeArchived bool) ([]patrimonio.Caixinha, error) {
	out := []patrimonio.Caixinha{}
	for _, c := range r.items {
		if !includeArchived && c.Archived {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
func (r *fakeRepo) Update(_ context.Context, c patrimonio.Caixinha) error {
	if _, ok := r.items[c.ID]; !ok {
		return patrimonio.ErrCaixinhaNotFound
	}
	r.items[c.ID] = c
	return nil
}
func (r *fakeRepo) SetArchived(_ context.Context, id string, archived bool) error {
	c, ok := r.items[id]
	if !ok {
		return patrimonio.ErrCaixinhaNotFound
	}
	c.Archived = archived
	r.items[id] = c
	return nil
}
func (r *fakeRepo) SetMarketValue(_ context.Context, id string, valor int64, data string) error {
	c, ok := r.items[id]
	if !ok {
		return patrimonio.ErrCaixinhaNotFound
	}
	c.ValorMercado = &valor
	c.DataValorMercado = &data
	r.items[id] = c
	return nil
}
func (r *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return patrimonio.ErrCaixinhaNotFound
	}
	delete(r.items, id)
	return nil
}

type fakeMovements struct {
	balances map[string]int64
	movs     []shared.CaixinhaMovement
	opening  map[string][]string // caixinhaID -> ids de saldo inicial
}

func (m *fakeMovements) ListByCaixinha(_ context.Context, id string, _ shared.Pagination) ([]shared.CaixinhaMovement, int, error) {
	out := []shared.CaixinhaMovement{}
	for _, mv := range m.movs {
		if mv.CaixinhaID == id {
			out = append(out, mv)
		}
	}
	return out, len(out), nil
}
func (m *fakeMovements) BalanceByCaixinha(_ context.Context, id string) (int64, error) {
	return m.balances[id], nil
}
func (m *fakeMovements) BalancesAll(_ context.Context) (map[string]int64, error) {
	return m.balances, nil
}
func (m *fakeMovements) OpeningMovementIDs(_ context.Context, id string) ([]string, error) {
	if m.opening == nil {
		return nil, nil
	}
	return m.opening[id], nil
}

type fakeWriter struct {
	registered []shared.NewCaixinhaMovement
	deleted    []string
	err        error
}

func (w *fakeWriter) Register(_ context.Context, in shared.NewCaixinhaMovement) (string, error) {
	if w.err != nil {
		return "", w.err
	}
	w.registered = append(w.registered, in)
	return "tx-" + in.Direction, nil
}
func (w *fakeWriter) Delete(_ context.Context, txID string) error {
	w.deleted = append(w.deleted, txID)
	return nil
}

type fakeDisponivel struct{ v int64 }

func (d fakeDisponivel) DisponivelAtual(_ context.Context) (int64, error) { return d.v, nil }

func newService(repo *fakeRepo, mov *fakeMovements, w *fakeWriter, disp int64) *patrimonio.Service {
	return patrimonio.NewService(patrimonio.Deps{
		Repo: repo, Movements: mov, Writer: w, Disponivel: fakeDisponivel{v: disp},
	})
}

// ─── Testes ───────────────────────────────────────────────────────────────────

func TestCreateAndList(t *testing.T) {
	repo := newFakeRepo()
	mov := &fakeMovements{balances: map[string]int64{}}
	svc := newService(repo, mov, &fakeWriter{}, 0)
	ctx := context.Background()

	meta := int64(600000)
	v, err := svc.CreateCaixinha(ctx, patrimonio.CreateCaixinhaInput{Name: "Reserva", Type: patrimonio.TypeReserva, MetaValor: &meta})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mov.balances[v.ID] = 300000 // metade da meta

	list, err := svc.ListCaixinhas(ctx, false)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if list[0].Saldo != 300000 || list[0].Progress == nil || *list[0].Progress != 5000 {
		t.Fatalf("view inesperada: saldo=%d prog=%v", list[0].Saldo, list[0].Progress)
	}
}

func TestAportarEResgatar(t *testing.T) {
	repo := newFakeRepo()
	mov := &fakeMovements{balances: map[string]int64{"cx1": 50000}}
	w := &fakeWriter{}
	svc := newService(repo, mov, w, 0)
	ctx := context.Background()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}

	if _, err := svc.Aportar(ctx, patrimonio.MovementInput{CaixinhaID: "cx1", Amount: 10000, Date: "2026-07-01"}); err != nil {
		t.Fatalf("aportar: %v", err)
	}
	if len(w.registered) != 1 || w.registered[0].Direction != patrimonio.DirectionAporte {
		t.Fatalf("aporte não registrado: %+v", w.registered)
	}

	// resgate dentro do saldo
	if _, err := svc.Resgatar(ctx, patrimonio.MovementInput{CaixinhaID: "cx1", Amount: 50000, Date: "2026-07-02"}); err != nil {
		t.Fatalf("resgatar: %v", err)
	}
	// resgate acima do saldo bloqueia
	_, err := svc.Resgatar(ctx, patrimonio.MovementInput{CaixinhaID: "cx1", Amount: 50001, Date: "2026-07-03"})
	if !errors.Is(err, patrimonio.ErrResgateExcedeSaldo) {
		t.Fatalf("esperava ErrResgateExcedeSaldo, veio %v", err)
	}
}

func TestResgatar_CaixinhaInexistente(t *testing.T) {
	svc := newService(newFakeRepo(), &fakeMovements{balances: map[string]int64{}}, &fakeWriter{}, 0)
	_, err := svc.Resgatar(context.Background(), patrimonio.MovementInput{CaixinhaID: "nope", Amount: 1, Date: "2026-07-01"})
	if !errors.Is(err, patrimonio.ErrCaixinhaNotFound) {
		t.Fatalf("esperava ErrCaixinhaNotFound, veio %v", err)
	}
}

func TestDeleteCaixinha_BloqueiaComSaldo(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{balances: map[string]int64{"cx1": 100}}
	svc := newService(repo, mov, &fakeWriter{}, 0)
	ctx := context.Background()

	if err := svc.DeleteCaixinha(ctx, "cx1"); !errors.Is(err, patrimonio.ErrExcluirComSaldo) {
		t.Fatalf("esperava ErrExcluirComSaldo, veio %v", err)
	}
	mov.balances["cx1"] = 0
	if err := svc.DeleteCaixinha(ctx, "cx1"); err != nil {
		t.Fatalf("delete com saldo zero deveria passar: %v", err)
	}
}

func TestMarketValue_SoInvestimento(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	repo.items["cx2"] = patrimonio.Caixinha{ID: "cx2", Name: "Ações", Type: patrimonio.TypeInvestimento}
	mov := &fakeMovements{balances: map[string]int64{"cx2": 100000}}
	svc := newService(repo, mov, &fakeWriter{}, 0)
	ctx := context.Background()

	if _, err := svc.AtualizarValorMercado(ctx, patrimonio.MarketValueInput{ID: "cx1", ValorMercado: 1, Data: "2026-07-01"}); !errors.Is(err, patrimonio.ErrValorMercadoTipo) {
		t.Fatalf("esperava ErrValorMercadoTipo, veio %v", err)
	}
	v, err := svc.AtualizarValorMercado(ctx, patrimonio.MarketValueInput{ID: "cx2", ValorMercado: 120000, Data: "2026-07-01"})
	if err != nil {
		t.Fatalf("market value investimento: %v", err)
	}
	if v.Ganho == nil || *v.Ganho != 20000 {
		t.Fatalf("ganho esperado 20000, veio %v", v.Ganho)
	}
}

func TestOverview(t *testing.T) {
	repo := newFakeRepo()
	vm := int64(120000)
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "Reserva", Type: patrimonio.TypeReserva}
	repo.items["cx2"] = patrimonio.Caixinha{ID: "cx2", Name: "Ações", Type: patrimonio.TypeInvestimento, ValorMercado: &vm}
	mov := &fakeMovements{balances: map[string]int64{"cx1": 60000, "cx2": 100000}}
	svc := newService(repo, mov, &fakeWriter{}, 40000)
	ctx := context.Background()

	ov, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Guardado != 160000 {
		t.Fatalf("guardado esperado 160000, veio %d", ov.Guardado)
	}
	if ov.Disponivel != 40000 {
		t.Fatalf("disponivel esperado 40000, veio %d", ov.Disponivel)
	}
	if ov.GanhoTotal != 20000 {
		t.Fatalf("ganho total esperado 20000, veio %d", ov.GanhoTotal)
	}
	// patrimonio = disponivel(40000) + guardado(160000) + ganho(20000)
	if ov.PatrimonioTotal != 220000 {
		t.Fatalf("patrimonio esperado 220000, veio %d", ov.PatrimonioTotal)
	}
	if len(ov.Caixinhas) != 2 {
		t.Fatalf("esperava 2 caixinhas, veio %d", len(ov.Caixinhas))
	}
}

func TestExtratoEDeleteMovimento(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{
		balances: map[string]int64{"cx1": 100},
		movs: []shared.CaixinhaMovement{
			{TransactionID: "m1", CaixinhaID: "cx1", Direction: "aporte", Amount: 100, Date: "2026-07-01"},
		},
	}
	w := &fakeWriter{}
	svc := newService(repo, mov, w, 0)
	ctx := context.Background()

	ex, err := svc.Extrato(ctx, "cx1", shared.DefaultPagination())
	if err != nil || ex.Total != 1 || len(ex.Movimentos) != 1 {
		t.Fatalf("extrato: %v total=%d", err, ex.Total)
	}
	if err := svc.DeleteMovimento(ctx, "m1"); err != nil {
		t.Fatalf("delete movimento: %v", err)
	}
	if len(w.deleted) != 1 || w.deleted[0] != "m1" {
		t.Fatalf("delete não propagou: %+v", w.deleted)
	}
}

func TestDefinirSaldoInicial(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "Reserva", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{balances: map[string]int64{}, opening: map[string][]string{"cx1": {"old-op"}}}
	w := &fakeWriter{}
	svc := newService(repo, mov, w, 0)
	ctx := context.Background()

	// define saldo inicial: apaga o anterior e registra um novo (opening)
	if err := svc.DefinirSaldoInicial(ctx, "cx1", 500000, "2026-07-01"); err != nil {
		t.Fatalf("definir: %v", err)
	}
	if len(w.deleted) != 1 || w.deleted[0] != "old-op" {
		t.Fatalf("deveria apagar o saldo inicial anterior: %+v", w.deleted)
	}
	if len(w.registered) != 1 || !w.registered[0].Opening || w.registered[0].Amount != 500000 {
		t.Fatalf("deveria registrar opening de 500000: %+v", w.registered)
	}

	// valor 0 apenas limpa (não registra novo)
	w2 := &fakeWriter{}
	svc2 := newService(repo, &fakeMovements{balances: map[string]int64{}, opening: map[string][]string{"cx1": {"op2"}}}, w2, 0)
	if err := svc2.DefinirSaldoInicial(ctx, "cx1", 0, "2026-07-01"); err != nil {
		t.Fatalf("limpar: %v", err)
	}
	if len(w2.deleted) != 1 || len(w2.registered) != 0 {
		t.Fatalf("valor 0 deveria só limpar: del=%v reg=%v", w2.deleted, w2.registered)
	}

	// caixinha inexistente
	if err := svc.DefinirSaldoInicial(ctx, "nope", 100, "2026-07-01"); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("esperava ErrCaixinhaNotFound, veio %v", err)
	}
	// validação (data vazia)
	if err := svc.DefinirSaldoInicial(ctx, "cx1", 100, ""); err == nil {
		t.Fatal("esperava erro de validação com data vazia")
	}
}

func TestRegistrarRendimento(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "Reserva", Type: patrimonio.TypeReserva}
	w := &fakeWriter{}
	svc := newService(repo, &fakeMovements{balances: map[string]int64{"cx1": 50000}}, w, 0)
	ctx := context.Background()

	if _, err := svc.RegistrarRendimento(ctx, patrimonio.MovementInput{CaixinhaID: "cx1", Amount: 300, Date: "2026-07-31"}); err != nil {
		t.Fatalf("rendimento: %v", err)
	}
	if len(w.registered) != 1 || !w.registered[0].Rendimento || w.registered[0].Direction != patrimonio.DirectionAporte {
		t.Fatalf("deveria registrar rendimento (aporte neutro): %+v", w.registered)
	}
	// caixinha inexistente
	if _, err := svc.RegistrarRendimento(ctx, patrimonio.MovementInput{CaixinhaID: "nope", Amount: 1, Date: "2026-07-31"}); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("esperava not found, veio %v", err)
	}
	// validação
	if _, err := svc.RegistrarRendimento(ctx, patrimonio.MovementInput{CaixinhaID: "cx1", Amount: 0, Date: ""}); err == nil {
		t.Fatal("esperava erro de validação")
	}
}

func TestCreateCaixinha_ValidationError(t *testing.T) {
	svc := newService(newFakeRepo(), &fakeMovements{balances: map[string]int64{}}, &fakeWriter{}, 0)
	if _, err := svc.CreateCaixinha(context.Background(), patrimonio.CreateCaixinhaInput{Name: "", Type: "x"}); err == nil {
		t.Fatal("esperava erro de validação")
	}
}

func TestUpdateAndArchive(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{balances: map[string]int64{"cx1": 0}}
	svc := newService(repo, mov, &fakeWriter{}, 0)
	ctx := context.Background()

	v, err := svc.UpdateCaixinha(ctx, patrimonio.UpdateCaixinhaInput{ID: "cx1", Name: "Reserva 2", Type: patrimonio.TypeReserva})
	if err != nil || v.Name != "Reserva 2" {
		t.Fatalf("update: %v %+v", err, v)
	}
	if err := svc.ArchiveCaixinha(ctx, "cx1", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := svc.GetCaixinha(ctx, "cx1"); err != nil {
		t.Fatalf("get: %v", err)
	}
}
