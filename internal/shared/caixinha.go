package shared

// DTOs neutros trocados entre o módulo `patrimonio` (consumidor) e o módulo
// `transaction` (produtor). Moram em `shared` para evitar import direto entre
// módulos (mesmo padrão de CardTransaction / NewTransaction). Data-carriers puros.

// CaixinhaMovement é uma linha do extrato de uma caixinha (aporte/resgate),
// projeção neutra lida do livro-caixa. As tags JSON são necessárias porque é
// serializada direto na resposta do extrato (o front lê camelCase).
type CaixinhaMovement struct {
	TransactionID string `json:"transactionId"`
	CaixinhaID    string `json:"caixinhaId"`
	Direction     string `json:"direction"` // "aporte" | "resgate"
	Amount        int64  `json:"amount"`    // centavos, sempre > 0
	Date          string `json:"date"`      // YYYY-MM-DD (data de caixa = payment_date)
	Description   string `json:"description"`
}

// NewCaixinhaMovement é o pedido neutro para registrar um aporte/resgate. O
// produtor (transaction) cria o lançamento neutro (type=transferencia) marcado
// com caixinha_id e a subcategoria de sistema da direção.
type NewCaixinhaMovement struct {
	CaixinhaID  string
	Direction   string  // "aporte" | "resgate"
	Amount      int64   // centavos > 0
	Date        string  // YYYY-MM-DD
	Description *string
	// Opening marca um SALDO INICIAL (abertura): conta no guardado mas NÃO mexe no
	// disponível de caixa (dinheiro que o usuário já tinha antes de usar o app).
	Opening bool
	// Rendimento marca um RENDIMENTO (juros/dividendos reinvestidos): conta no guardado
	// e no patrimônio, neutro ao disponível e sem contar como receita.
	Rendimento bool
}
