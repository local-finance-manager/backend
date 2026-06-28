package shared

// DTOs neutros trocados entre o módulo `report` (consumidor) e os módulos que
// produzem os dados (`transaction`, `category`). Moram em `shared` para evitar
// import direto entre módulos (mesmo padrão de CardTransaction). São data-carriers
// puros — sem comportamento.

// SubcategoryAggregate é o total agregado de uma subcategoria num mês (centavos).
type SubcategoryAggregate struct {
	SubcategoryID string
	CategoryID    string
	Type          string // "despesa" | "receita" | "transferencia"
	Total         int64  // soma em centavos
	TxCount       int
}

// MonthlyTotals são os totais de um mês (centavos), apenas realizado (ou pendente,
// no caso do agregador de projeção). Saldo inicial/final seguem o conceito E6.
type MonthlyTotals struct {
	Receitas       int64
	Despesas       int64
	Transferencias int64
	SaldoPeriodo   int64 // receitas - despesas
	SaldoInicial   int64 // acumulado de abertura (carryover + ajustes de saldo)
	SaldoFinal     int64 // saldoInicial + saldoPeriodo
	TxCount        int
}

// SubcategoryNode é uma subcategoria com seu nome (para compor a resposta do relatório).
type SubcategoryNode struct {
	ID   string
	Name string
}

// CategoryNode é uma categoria com suas subcategorias e metadados de exibição.
type CategoryNode struct {
	CategoryID    string
	CategoryName  string
	CategoryColor string
	Type          string
	Subcategories []SubcategoryNode
}
