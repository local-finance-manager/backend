package budget

import (
	"context"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/google/uuid"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Deps são as dependências do serviço (injetadas no main.go).
type Deps struct {
	Repo           Repository
	Income         IncomeReader
	Txns           TransactionWriter
	Caixinha       CaixinhaAporter // materializa aporte quando o destino aponta p/ caixinha
	InvestSubcatID string          // subcategoria default p/ investimento sem preset (A4)
}

// Service orquestra os casos de uso de alocação de receitas.
type Service struct {
	repo         Repository
	income       IncomeReader
	txns         TransactionWriter
	caixinha     CaixinhaAporter
	investSubcat string
	now          func() time.Time
}

// NewService cria o serviço.
func NewService(d Deps) *Service {
	return &Service{repo: d.Repo, income: d.Income, txns: d.Txns, caixinha: d.Caixinha, investSubcat: d.InvestSubcatID, now: time.Now}
}

// ─── Tipos de resposta (camelCase) ───────────────────────────────────────────

type IncomeView struct {
	Total        int64               `json:"total"`
	AllRealized  bool                `json:"allRealized"`
	PendingCount int                 `json:"pendingCount"`
	Items        []shared.IncomeItem `json:"items"`
}

type DestinationView struct {
	ID                        string  `json:"id"`
	Reference                 string  `json:"reference"`
	Name                      string  `json:"name"`
	Kind                      string  `json:"kind"`
	Mode                      string  `json:"mode"`
	Percentage                *int    `json:"percentage"`
	FixedAmount               *int64  `json:"fixedAmount"`
	ComputedAmount            int64   `json:"computedAmount"`
	Status                    string  `json:"status"` // "planejado" | "materializado"
	MaterializedTransactionID *string `json:"materializedTransactionId"`
	MaterializedAmount        *int64  `json:"materializedAmount"`
	PresetSubcategoryID       *string `json:"presetSubcategoryId"`
	PresetPaymentMethod       *string `json:"presetPaymentMethod"`
	PresetDescription         *string `json:"presetDescription"`
	CaixinhaID                *string `json:"caixinhaId"`
	DisplayOrder              int     `json:"displayOrder"`
}

type PlanView struct {
	Reference         string            `json:"reference"`
	Income            IncomeView        `json:"income"`
	AllocatedAmount   int64             `json:"allocatedAmount"`
	AllocatedPercent  int               `json:"allocatedPercent"`
	UnallocatedAmount int64             `json:"unallocatedAmount"`
	AvailableAmount   int64             `json:"availableAmount"`
	CanMaterialize    bool              `json:"canMaterialize"`
	Destinations      []DestinationView `json:"destinations"`
}

// MaterializeInput são os campos (todos opcionais) que sobrescrevem o lançamento
// gerado a partir do destino/presets.
type MaterializeInput struct {
	SubcategoryID  *string
	Amount         *int64
	CompetenceDate *string
	PaymentDate    *string
	Description    *string
	PaymentMethod  *string
}

// ─── Plano ───────────────────────────────────────────────────────────────────

// GetPlan monta o plano do mês: renda, destinos com valores calculados e totais.
func (s *Service) GetPlan(ctx context.Context, reference string) (PlanView, error) {
	total, allRealized, items, err := s.income.MonthIncome(ctx, reference)
	if err != nil {
		return PlanView{}, domainerr.NewInternal("erro ao ler a renda do mês")
	}
	dests, err := s.repo.ListDestinations(ctx, reference)
	if err != nil {
		return PlanView{}, err
	}
	return s.buildPlan(reference, total, allRealized, items, dests), nil
}

func (s *Service) buildPlan(reference string, total int64, allRealized bool, items []shared.IncomeItem, dests []Destination) PlanView {
	comp := ComputePlan(total, dests)

	var available int64 = total
	pending := 0
	for _, it := range items {
		if it.Status == "pendente" {
			pending++
		}
	}
	views := make([]DestinationView, len(dests))
	for i, d := range dests {
		status := "planejado"
		if d.IsMaterialized() {
			status = "materializado"
			// disponível = renda − despesas/investimentos já materializados
			if d.MaterializedAmount != nil {
				available -= *d.MaterializedAmount
			}
		}
		views[i] = DestinationView{
			ID: d.ID, Reference: d.Reference, Name: d.Name, Kind: string(d.Kind), Mode: string(d.Mode),
			Percentage: d.Percentage, FixedAmount: d.FixedAmount, ComputedAmount: comp.ByDestination[d.ID],
			Status: status, MaterializedTransactionID: d.MaterializedTxID, MaterializedAmount: d.MaterializedAmount,
			PresetSubcategoryID: d.PresetSubcategoryID, PresetPaymentMethod: d.PresetPaymentMethod,
			PresetDescription: d.PresetDescription, CaixinhaID: d.CaixinhaID, DisplayOrder: d.DisplayOrder,
		}
	}

	return PlanView{
		Reference: reference,
		Income: IncomeView{
			Total: total, AllRealized: allRealized, PendingCount: pending, Items: items,
		},
		AllocatedAmount:   comp.AllocatedAmount,
		AllocatedPercent:  comp.AllocatedPercent,
		UnallocatedAmount: comp.UnallocatedAmount,
		AvailableAmount:   available,
		CanMaterialize:    allRealized && total > 0,
		Destinations:      views,
	}
}

// ─── CRUD de destinos ────────────────────────────────────────────────────────

// CreateDestination cria um destino, validando o limite de 100% com os já existentes.
func (s *Service) CreateDestination(ctx context.Context, in DestinationInput) (Destination, error) {
	if err := ValidateDestination(in); err != nil {
		return Destination{}, err
	}
	total, _, _, err := s.income.MonthIncome(ctx, in.Reference)
	if err != nil {
		return Destination{}, domainerr.NewInternal("erro ao ler a renda do mês")
	}
	existing, err := s.repo.ListDestinations(ctx, in.Reference)
	if err != nil {
		return Destination{}, err
	}
	now := s.now().UTC()
	d := fromInput(uuid.New().String(), in, now)
	if err := ValidateAllocation(total, append(existing, d)); err != nil {
		return Destination{}, err
	}
	if err := s.repo.CreateDestination(ctx, d); err != nil {
		return Destination{}, err
	}
	return d, nil
}

// UpdateDestination edita um destino planejado (materializado é imutável).
func (s *Service) UpdateDestination(ctx context.Context, id string, in DestinationInput) (Destination, error) {
	if err := ValidateDestination(in); err != nil {
		return Destination{}, err
	}
	cur, err := s.repo.GetDestination(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	if cur.IsMaterialized() {
		return Destination{}, ErrAlreadyMaterialized
	}
	total, _, _, err := s.income.MonthIncome(ctx, cur.Reference)
	if err != nil {
		return Destination{}, domainerr.NewInternal("erro ao ler a renda do mês")
	}
	existing, err := s.repo.ListDestinations(ctx, cur.Reference)
	if err != nil {
		return Destination{}, err
	}
	updated := fromInput(id, in, cur.CreatedAt)
	updated.Reference = cur.Reference
	merged := make([]Destination, 0, len(existing))
	for _, d := range existing {
		if d.ID == id {
			merged = append(merged, updated)
		} else {
			merged = append(merged, d)
		}
	}
	if err := ValidateAllocation(total, merged); err != nil {
		return Destination{}, err
	}
	if err := s.repo.UpdateDestination(ctx, updated); err != nil {
		return Destination{}, err
	}
	return updated, nil
}

// DeleteDestination remove um destino planejado (desfaça a materialização antes).
func (s *Service) DeleteDestination(ctx context.Context, id string) error {
	cur, err := s.repo.GetDestination(ctx, id)
	if err != nil {
		return err
	}
	if cur.IsMaterialized() {
		return ErrAlreadyMaterialized
	}
	return s.repo.DeleteDestination(ctx, id)
}

// ─── Materialização ──────────────────────────────────────────────────────────

// MaterializeResult é o retorno de uma materialização.
type MaterializeResult struct {
	DestinationID string `json:"destinationId"`
	Status        string `json:"status"`
	TransactionID string `json:"transactionId"`
	Amount        int64  `json:"amount"`
}

// Materialize cria o lançamento (realizado) de um destino e o vincula (atômico via
// compensação: se o vínculo falhar, o lançamento criado é desfeito).
func (s *Service) Materialize(ctx context.Context, id string, in MaterializeInput) (MaterializeResult, error) {
	cur, err := s.repo.GetDestination(ctx, id)
	if err != nil {
		return MaterializeResult{}, err
	}
	if cur.IsMaterialized() {
		return MaterializeResult{}, ErrAlreadyMaterialized
	}

	total, allRealized, _, err := s.income.MonthIncome(ctx, cur.Reference)
	if err != nil {
		return MaterializeResult{}, domainerr.NewInternal("erro ao ler a renda do mês")
	}
	if !allRealized || total <= 0 {
		return MaterializeResult{}, ErrIncomePending
	}

	dests, err := s.repo.ListDestinations(ctx, cur.Reference)
	if err != nil {
		return MaterializeResult{}, err
	}
	comp := ComputePlan(total, dests)
	amount := comp.ByDestination[id]
	if in.Amount != nil {
		amount = *in.Amount
	}

	txID, err := s.materializeTx(ctx, cur, amount, in)
	if err != nil {
		return MaterializeResult{}, err // erros de domínio (mês bloqueado, validação) propagam
	}
	ok, err := s.repo.SetMaterialized(ctx, id, txID, amount, s.now().UTC())
	if err != nil || !ok {
		_ = s.txns.Delete(ctx, txID) // compensação: não deixa lançamento órfão
		if err != nil {
			return MaterializeResult{}, err
		}
		return MaterializeResult{}, ErrAlreadyMaterialized
	}
	return MaterializeResult{DestinationID: id, Status: "materializado", TransactionID: txID, Amount: amount}, nil
}

// materializeTx cria o lançamento do destino: um APORTE na caixinha (quando o destino
// aponta para uma) ou um lançamento normal (despesa/transferência). Devolve o txID.
func (s *Service) materializeTx(ctx context.Context, d Destination, amount int64, in MaterializeInput) (string, error) {
	if d.CaixinhaID != nil && *d.CaixinhaID != "" {
		if s.caixinha == nil {
			return "", domainerr.NewInternal("aporte em caixinha indisponível")
		}
		date := s.today()
		if in.PaymentDate != nil && *in.PaymentDate != "" {
			date = *in.PaymentDate
		}
		desc := d.PresetDescription
		if in.Description != nil {
			desc = in.Description
		}
		return s.caixinha.RegisterAporte(ctx, *d.CaixinhaID, amount, date, desc)
	}
	tx, err := s.buildTransaction(d, amount, in)
	if err != nil {
		return "", err
	}
	return s.txns.Create(ctx, tx)
}

// buildTransaction monta o NewTransaction a partir do destino + presets + overrides.
func (s *Service) buildTransaction(d Destination, amount int64, in MaterializeInput) (shared.NewTransaction, error) {
	subcat := s.effectiveSubcategory(d)
	if in.SubcategoryID != nil && *in.SubcategoryID != "" {
		subcat = *in.SubcategoryID
	}
	if subcat == "" {
		return shared.NewTransaction{}, ErrMissingPreset
	}
	pay := s.today()
	if in.PaymentDate != nil && *in.PaymentDate != "" {
		pay = *in.PaymentDate
	}
	comp := s.monthLastDay(d.Reference)
	if in.CompetenceDate != nil && *in.CompetenceDate != "" {
		comp = *in.CompetenceDate
	}
	pm := ""
	if d.PresetPaymentMethod != nil {
		pm = *d.PresetPaymentMethod
	}
	if in.PaymentMethod != nil && *in.PaymentMethod != "" {
		pm = *in.PaymentMethod
	}
	desc := d.PresetDescription
	if in.Description != nil {
		desc = in.Description
	}
	return shared.NewTransaction{
		Title: d.Name, Description: desc, Amount: amount, SubcategoryID: subcat,
		PaymentMethod: pm, Status: "realizado", CompetenceDate: comp, PaymentDate: &pay,
	}, nil
}

// effectiveSubcategory resolve a subcategoria: preset, ou o default de investimento
// (A4) quando o destino é de investimento sem preset; vazio para despesa sem preset.
func (s *Service) effectiveSubcategory(d Destination) string {
	if d.PresetSubcategoryID != nil && *d.PresetSubcategoryID != "" {
		return *d.PresetSubcategoryID
	}
	if d.Kind == KindInvestimento {
		return s.investSubcat
	}
	return ""
}

// Undo desfaz a materialização: exclui o lançamento (respeitando o bloqueio de mês
// via TransactionWriter) e volta o destino a planejado.
func (s *Service) Undo(ctx context.Context, id string) error {
	cur, err := s.repo.GetDestination(ctx, id)
	if err != nil {
		return err
	}
	if !cur.IsMaterialized() {
		return ErrNotMaterialized
	}
	if err := s.txns.Delete(ctx, *cur.MaterializedTxID); err != nil {
		return err // mês bloqueado etc. propaga
	}
	return s.repo.ClearMaterialized(ctx, id)
}

// BulkResult é o retorno do materializar-todos.
type BulkResult struct {
	Materialized []MaterializeResult  `json:"materialized"`
	Skipped      []SkippedDestination `json:"skipped"`
}

// SkippedDestination é um destino não materializado no lote (preset incompleto).
type SkippedDestination struct {
	DestinationID string `json:"destinationId"`
	Name          string `json:"name"`
	Reason        string `json:"reason"`
}

// MaterializeAll materializa todos os destinos planejados com preset completo.
func (s *Service) MaterializeAll(ctx context.Context, reference string) (BulkResult, error) {
	total, allRealized, _, err := s.income.MonthIncome(ctx, reference)
	if err != nil {
		return BulkResult{}, domainerr.NewInternal("erro ao ler a renda do mês")
	}
	if !allRealized || total <= 0 {
		return BulkResult{}, ErrIncomePending
	}
	dests, err := s.repo.ListDestinations(ctx, reference)
	if err != nil {
		return BulkResult{}, err
	}
	comp := ComputePlan(total, dests)

	res := BulkResult{Materialized: []MaterializeResult{}, Skipped: []SkippedDestination{}}
	for _, d := range dests {
		if d.IsMaterialized() {
			continue
		}
		isCaixinha := d.CaixinhaID != nil && *d.CaixinhaID != ""
		if !isCaixinha && s.effectiveSubcategory(d) == "" {
			res.Skipped = append(res.Skipped, SkippedDestination{
				DestinationID: d.ID, Name: d.Name, Reason: "sem subcategoria de preset",
			})
			continue
		}
		amount := comp.ByDestination[d.ID]
		if amount <= 0 {
			continue
		}
		txID, cerr := s.materializeTx(ctx, d, amount, MaterializeInput{})
		if cerr != nil {
			return res, cerr // mês bloqueado etc. aborta o lote
		}
		ok, serr := s.repo.SetMaterialized(ctx, d.ID, txID, amount, s.now().UTC())
		if serr != nil || !ok {
			_ = s.txns.Delete(ctx, txID)
			if serr != nil {
				return res, serr
			}
			continue
		}
		res.Materialized = append(res.Materialized, MaterializeResult{
			DestinationID: d.ID, Status: "materializado", TransactionID: txID, Amount: amount,
		})
	}
	return res, nil
}

// ─── Templates / copiar mês ──────────────────────────────────────────────────

// ListTemplates lista os templates disponíveis.
func (s *Service) ListTemplates(ctx context.Context) ([]Template, error) {
	return s.repo.ListTemplates(ctx)
}

// CreateTemplate cria um template a partir de itens informados.
func (s *Service) CreateTemplate(ctx context.Context, name string, items []TemplateItem) (Template, error) {
	t := Template{ID: uuid.New().String(), Name: name, Items: items}
	for i := range t.Items {
		t.Items[i].ID = uuid.New().String()
		t.Items[i].DisplayOrder = i
	}
	if err := s.repo.CreateTemplate(ctx, t); err != nil {
		return Template{}, err
	}
	return t, nil
}

// ApplyTemplate cria, no mês, destinos a partir de um template (planejados).
func (s *Service) ApplyTemplate(ctx context.Context, reference, templateID string) error {
	t, err := s.repo.GetTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	dests := make([]Destination, 0, len(t.Items))
	for i, it := range t.Items {
		dests = append(dests, Destination{
			ID: uuid.New().String(), Reference: reference, Name: it.Name, Kind: it.Kind, Mode: it.Mode,
			Percentage: it.Percentage, FixedAmount: it.FixedAmount, PresetSubcategoryID: it.PresetSubcategoryID,
			PresetPaymentMethod: it.PresetPaymentMethod, PresetDescription: it.PresetDescription,
			CaixinhaID: it.CaixinhaID, DisplayOrder: i, CreatedAt: now, UpdatedAt: now,
		})
	}
	return s.repo.CreateDestinations(ctx, dests)
}

// CopyPrevious clona os destinos do mês anterior (como planejados) para `reference`.
func (s *Service) CopyPrevious(ctx context.Context, reference string) error {
	prev, err := prevReference(reference)
	if err != nil {
		return err
	}
	src, err := s.repo.ListDestinations(ctx, prev)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	dests := make([]Destination, 0, len(src))
	for i, d := range src {
		dests = append(dests, Destination{
			ID: uuid.New().String(), Reference: reference, Name: d.Name, Kind: d.Kind, Mode: d.Mode,
			Percentage: d.Percentage, FixedAmount: d.FixedAmount, PresetSubcategoryID: d.PresetSubcategoryID,
			PresetPaymentMethod: d.PresetPaymentMethod, PresetDescription: d.PresetDescription,
			CaixinhaID: d.CaixinhaID, DisplayOrder: i, CreatedAt: now, UpdatedAt: now,
		})
	}
	return s.repo.CreateDestinations(ctx, dests)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (s *Service) today() string { return s.now().UTC().Format("2006-01-02") }

func (s *Service) monthLastDay(reference string) string {
	t, err := time.Parse("2006-01", reference)
	if err != nil {
		return s.today()
	}
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func prevReference(reference string) (string, error) {
	t, err := time.Parse("2006-01", reference)
	if err != nil {
		return "", domainerr.NewBadRequest("referência inválida: use YYYY-MM", domainerr.WithDisplayable())
	}
	p := t.AddDate(0, -1, 0)
	return p.Format("2006-01"), nil
}

func fromInput(id string, in DestinationInput, createdAt time.Time) Destination {
	return Destination{
		ID: id, Reference: in.Reference, Name: in.Name, Kind: in.Kind, Mode: in.Mode,
		Percentage: in.Percentage, FixedAmount: in.FixedAmount, PresetSubcategoryID: in.PresetSubcategoryID,
		PresetPaymentMethod: in.PresetPaymentMethod, PresetDescription: in.PresetDescription,
		CaixinhaID: in.CaixinhaID, DisplayOrder: in.DisplayOrder, CreatedAt: createdAt, UpdatedAt: time.Now().UTC(),
	}
}
