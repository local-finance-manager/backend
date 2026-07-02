package patrimonio

import "testing"

func TestValidateCreate_RequiresNameAndValidType(t *testing.T) {
	err := ValidateCreate(CreateCaixinhaInput{Name: "  ", Type: "xpto"})
	if err == nil {
		t.Fatal("esperava erro para nome vazio e tipo inválido")
	}
}

func TestValidateCreate_OK(t *testing.T) {
	meta := int64(100000)
	if err := ValidateCreate(CreateCaixinhaInput{Name: "Reserva", Type: TypeReserva, MetaValor: &meta}); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
}

func TestValidateCreate_MetaAndMarketRules(t *testing.T) {
	neg := int64(-1)
	if err := ValidateCreate(CreateCaixinhaInput{Name: "X", Type: TypeObjetivo, MetaValor: &neg}); err == nil {
		t.Fatal("meta <= 0 deve falhar")
	}
	if err := ValidateCreate(CreateCaixinhaInput{Name: "X", Type: TypeInvestimento, ValorMercado: &neg}); err == nil {
		t.Fatal("valor de mercado negativo deve falhar")
	}
	bad := "2026-13-40"
	if err := ValidateCreate(CreateCaixinhaInput{Name: "X", Type: TypeObjetivo, DataAlvo: &bad}); err == nil {
		t.Fatal("data alvo inválida deve falhar")
	}
}

func TestValidateUpdate_RequiresID(t *testing.T) {
	if err := ValidateUpdate(UpdateCaixinhaInput{ID: "", Name: "X", Type: TypeReserva}); err == nil {
		t.Fatal("id vazio deve falhar")
	}
}

func TestValidateMovement(t *testing.T) {
	if err := ValidateMovement(MovementInput{CaixinhaID: "", Amount: 0, Date: ""}); err == nil {
		t.Fatal("movimento vazio deve falhar")
	}
	if err := ValidateMovement(MovementInput{CaixinhaID: "cx1", Amount: 100, Date: "2026-07-01"}); err != nil {
		t.Fatalf("movimento válido não deveria falhar: %v", err)
	}
}

func TestValidateMarketValue(t *testing.T) {
	if err := ValidateMarketValue(MarketValueInput{ID: "cx1", ValorMercado: 100, Data: "2026-07-01"}); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if err := ValidateMarketValue(MarketValueInput{ID: "", ValorMercado: -1, Data: "bad"}); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestProgress(t *testing.T) {
	meta := int64(100000)
	c := Caixinha{Type: TypeReserva, MetaValor: &meta}
	if p := c.Progress(50000); p == nil || *p != 5000 {
		t.Fatalf("esperava 5000 bp, veio %v", p)
	}
	// teto em 100%
	if p := c.Progress(200000); p == nil || *p != 10000 {
		t.Fatalf("esperava teto 10000 bp, veio %v", p)
	}
	// sem meta → nil
	c2 := Caixinha{Type: TypeObjetivo}
	if p := c2.Progress(500); p != nil {
		t.Fatalf("sem meta deveria ser nil, veio %v", *p)
	}
}

func TestGanhoInvestimento(t *testing.T) {
	vm := int64(120000)
	c := Caixinha{Type: TypeInvestimento, ValorMercado: &vm}
	if g := c.GanhoInvestimento(100000); g == nil || *g != 20000 {
		t.Fatalf("esperava 20000, veio %v", g)
	}
	// perda
	if g := c.GanhoInvestimento(130000); g == nil || *g != -10000 {
		t.Fatalf("esperava -10000, veio %v", g)
	}
	// tipo não investimento → nil
	c2 := Caixinha{Type: TypeReserva, ValorMercado: &vm}
	if g := c2.GanhoInvestimento(100); g != nil {
		t.Fatalf("não-investimento deveria ser nil, veio %v", *g)
	}
	// sem valor de mercado → nil
	c3 := Caixinha{Type: TypeInvestimento}
	if g := c3.GanhoInvestimento(100); g != nil {
		t.Fatalf("sem valor de mercado deveria ser nil, veio %v", *g)
	}
}
