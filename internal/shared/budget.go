package shared

// DTOs neutros trocados entre o módulo `budget` (consumidor) e o módulo
// `transaction` (produtor). Moram em `shared` para evitar import direto entre
// módulos (mesmo padrão de CardTransaction). Data-carriers puros.

// IncomeItem é uma receita do mês na visão do plano de alocação. As tags JSON são
// necessárias porque este DTO é serializado direto na resposta de GET /api/income/plan
// (campo `items`), que o front lê em camelCase.
type IncomeItem struct {
	TransactionID string `json:"transactionId"`
	Title         string `json:"title"`
	Amount        int64  `json:"amount"` // centavos
	Status        string `json:"status"` // "pendente" | "realizado"
}

// NewTransaction é o pedido neutro para criar um lançamento (usado na
// materialização de um destino). O produtor (transaction) deriva o tipo a partir
// da subcategoria e aplica suas validações/guards.
type NewTransaction struct {
	Title          string
	Description    *string
	Amount         int64
	SubcategoryID  string
	PaymentMethod  string  // enum de lançamentos; vazio → default do produtor
	Status         string  // "pendente" | "realizado" | "cancelado"
	CompetenceDate string  // YYYY-MM-DD
	PaymentDate    *string // YYYY-MM-DD; obrigatório quando realizado
}
