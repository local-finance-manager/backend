package patrimonio

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Handler trata as requisições HTTP do módulo de patrimônio (snake_case).
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return false
	}
	return true
}

// ─── DTOs de request ──────────────────────────────────────────────────────────

type caixinhaReq struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	MetaValor    *int64  `json:"meta_valor"`
	DataAlvo     *string `json:"data_alvo"`
	ValorMercado *int64  `json:"valor_mercado"`
	Color        *string `json:"color"`
	Icon         *string `json:"icon"`
	DisplayOrder int     `json:"display_order"`
}

type movementReq struct {
	Amount      int64   `json:"amount"`
	Date        string  `json:"date"`
	Description *string `json:"description"`
}

type marketValueReq struct {
	ValorMercado int64  `json:"valor_mercado"`
	Data         string `json:"data"`
}

type saldoInicialReq struct {
	Valor int64  `json:"valor"`
	Data  string `json:"data"`
}

// ─── DTOs de response (snake_case) ────────────────────────────────────────────

type caixinhaResp struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	MetaValor        *int64  `json:"meta_valor"`
	DataAlvo         *string `json:"data_alvo"`
	ValorMercado     *int64  `json:"valor_mercado"`
	DataValorMercado *string `json:"data_valor_mercado"`
	Color            *string `json:"color"`
	Icon             *string `json:"icon"`
	DisplayOrder     int     `json:"display_order"`
	Archived         bool    `json:"archived"`
	Saldo            int64   `json:"saldo"`
	Progress         *int    `json:"progress"`
	Ganho            *int64  `json:"ganho"`
	Percent          int     `json:"percent"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func toCaixinhaResp(v CaixinhaView) caixinhaResp {
	return caixinhaResp{
		ID: v.ID, Name: v.Name, Type: string(v.Type), MetaValor: v.MetaValor, DataAlvo: v.DataAlvo,
		ValorMercado: v.ValorMercado, DataValorMercado: v.DataValorMercado, Color: v.Color, Icon: v.Icon,
		DisplayOrder: v.DisplayOrder, Archived: v.Archived, Saldo: v.Saldo, Progress: v.Progress,
		Ganho: v.Ganho, Percent: v.Percent,
		CreatedAt: v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: v.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

type overviewResp struct {
	PatrimonioTotal int64          `json:"patrimonio_total"`
	Disponivel      int64          `json:"disponivel"`
	Guardado        int64          `json:"guardado"`
	GanhoTotal      int64          `json:"ganho_total"`
	Caixinhas       []caixinhaResp `json:"caixinhas"`
}

type movementResp struct {
	TransactionID string `json:"transaction_id"`
	CaixinhaID    string `json:"caixinha_id"`
	Direction     string `json:"direction"`
	Amount        int64  `json:"amount"`
	Date          string `json:"date"`
	Description   string `json:"description"`
}

func toMovementResp(m shared.CaixinhaMovement) movementResp {
	return movementResp{
		TransactionID: m.TransactionID, CaixinhaID: m.CaixinhaID, Direction: m.Direction,
		Amount: m.Amount, Date: m.Date, Description: m.Description,
	}
}

func (req caixinhaReq) toCreateInput() CreateCaixinhaInput {
	return CreateCaixinhaInput{
		Name: req.Name, Type: CaixinhaType(req.Type), MetaValor: req.MetaValor, DataAlvo: req.DataAlvo,
		ValorMercado: req.ValorMercado, Color: req.Color, Icon: req.Icon, DisplayOrder: req.DisplayOrder,
	}
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// GetOverview — GET /api/patrimonio/overview
func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := h.svc.Overview(r.Context())
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	resp := overviewResp{
		PatrimonioTotal: ov.PatrimonioTotal, Disponivel: ov.Disponivel,
		Guardado: ov.Guardado, GanhoTotal: ov.GanhoTotal,
		Caixinhas: make([]caixinhaResp, len(ov.Caixinhas)),
	}
	for i, c := range ov.Caixinhas {
		resp.Caixinhas[i] = toCaixinhaResp(c)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListCaixinhas — GET /api/patrimonio/caixinhas?archived=false
func (h *Handler) ListCaixinhas(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("archived") == "true"
	cs, err := h.svc.ListCaixinhas(r.Context(), includeArchived)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	data := make([]caixinhaResp, len(cs))
	for i, c := range cs {
		data[i] = toCaixinhaResp(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// CreateCaixinha — POST /api/patrimonio/caixinhas
func (h *Handler) CreateCaixinha(w http.ResponseWriter, r *http.Request) {
	var req caixinhaReq
	if !decode(w, r, &req) {
		return
	}
	v, err := h.svc.CreateCaixinha(r.Context(), req.toCreateInput())
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCaixinhaResp(v))
}

// GetCaixinha — GET /api/patrimonio/caixinhas/{id}
func (h *Handler) GetCaixinha(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GetCaixinha(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCaixinhaResp(v))
}

// UpdateCaixinha — PUT /api/patrimonio/caixinhas/{id}
func (h *Handler) UpdateCaixinha(w http.ResponseWriter, r *http.Request) {
	var req caixinhaReq
	if !decode(w, r, &req) {
		return
	}
	in := UpdateCaixinhaInput{
		ID: chi.URLParam(r, "id"), Name: req.Name, Type: CaixinhaType(req.Type), MetaValor: req.MetaValor,
		DataAlvo: req.DataAlvo, ValorMercado: req.ValorMercado, Color: req.Color, Icon: req.Icon, DisplayOrder: req.DisplayOrder,
	}
	v, err := h.svc.UpdateCaixinha(r.Context(), in)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCaixinhaResp(v))
}

// DeleteCaixinha — DELETE /api/patrimonio/caixinhas/{id}
func (h *Handler) DeleteCaixinha(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteCaixinha(r.Context(), chi.URLParam(r, "id")); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Archive — PATCH /api/patrimonio/caixinhas/{id}/archive
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ArchiveCaixinha(r.Context(), chi.URLParam(r, "id"), true); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unarchive — PATCH /api/patrimonio/caixinhas/{id}/unarchive
func (h *Handler) Unarchive(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ArchiveCaixinha(r.Context(), chi.URLParam(r, "id"), false); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateMarketValue — PATCH /api/patrimonio/caixinhas/{id}/market-value
func (h *Handler) UpdateMarketValue(w http.ResponseWriter, r *http.Request) {
	var req marketValueReq
	if !decode(w, r, &req) {
		return
	}
	v, err := h.svc.AtualizarValorMercado(r.Context(), MarketValueInput{
		ID: chi.URLParam(r, "id"), ValorMercado: req.ValorMercado, Data: req.Data,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCaixinhaResp(v))
}

// DefinirSaldoInicial — PATCH /api/patrimonio/caixinhas/{id}/saldo-inicial
func (h *Handler) DefinirSaldoInicial(w http.ResponseWriter, r *http.Request) {
	var req saldoInicialReq
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.DefinirSaldoInicial(r.Context(), chi.URLParam(r, "id"), req.Valor, req.Data); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Aportar — POST /api/patrimonio/caixinhas/{id}/aportar
func (h *Handler) Aportar(w http.ResponseWriter, r *http.Request) {
	h.movimento(w, r, h.svc.Aportar)
}

// Resgatar — POST /api/patrimonio/caixinhas/{id}/resgatar
func (h *Handler) Resgatar(w http.ResponseWriter, r *http.Request) {
	h.movimento(w, r, h.svc.Resgatar)
}

// Rendimento — POST /api/patrimonio/caixinhas/{id}/rendimento
func (h *Handler) Rendimento(w http.ResponseWriter, r *http.Request) {
	h.movimento(w, r, h.svc.RegistrarRendimento)
}

// movimento decodifica o corpo, monta o MovementInput (caixinha via URL) e delega a
// fn (Aportar/Resgatar), respondendo 201 com o id do lançamento criado.
func (h *Handler) movimento(
	w http.ResponseWriter, r *http.Request,
	fn func(context.Context, MovementInput) (string, error),
) {
	var req movementReq
	if !decode(w, r, &req) {
		return
	}
	id, err := fn(r.Context(), MovementInput{
		CaixinhaID: chi.URLParam(r, "id"), Amount: req.Amount, Date: req.Date, Description: req.Description,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// Extrato — GET /api/patrimonio/caixinhas/{id}/extrato
func (h *Handler) Extrato(w http.ResponseWriter, r *http.Request) {
	p := shared.ParsePagination(r, shared.DefaultPagination(), []string{"created_at"})
	ex, err := h.svc.Extrato(r.Context(), chi.URLParam(r, "id"), p)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	data := make([]movementResp, len(ex.Movimentos))
	for i, m := range ex.Movimentos {
		data[i] = toMovementResp(m)
	}
	writeJSON(w, http.StatusOK, shared.NewPagedResult(data, ex.Total, p))
}

// DeleteMovimento — DELETE /api/patrimonio/movimentos/{txId}
func (h *Handler) DeleteMovimento(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteMovimento(r.Context(), chi.URLParam(r, "txId")); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
