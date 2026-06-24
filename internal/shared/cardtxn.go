package shared

// CardTransaction é a projeção neutra de um lançamento vinculado a cartão,
// trocada entre o módulo que produz (transaction) e o que consome (creditcard).
// Mora em shared para evitar import direto entre os dois módulos (guia §3.3):
// satisfação de interface em Go é nominal, então um retorno rico precisa de um
// tipo comum aos dois lados. É um data-carrier puro — sem comportamento.
type CardTransaction struct {
	ID              string
	Title           string
	Amount          int64   // centavos
	CompetenceDate  string  // YYYY-MM-DD
	PaymentDate     *string // YYYY-MM-DD ou nil
	Status          string  // pendente|realizado|cancelado
	SubcategoryID   string
	SubcategoryName string
	CategoryID      string
	CategoryName    string
	CategoryColor   string
	CreditCardID    string // sempre preenchido nesta projeção
	// Parcelamento (nil quando o lançamento não é parcela) — para exibir "k/N" na fatura.
	InstallmentGroupID *string
	InstallmentNumber  *int
	InstallmentTotal   *int
}
