package budget

import "testing"

func pct(v int) *int       { return &v }
func cents(v int64) *int64 { return &v }

func dPercent(id string, p int) Destination {
	return Destination{ID: id, Mode: ModePercentual, Kind: KindDespesa, Percentage: pct(p)}
}
func dFixed(id string, a int64) Destination {
	return Destination{ID: id, Mode: ModeValorFixo, Kind: KindDespesa, FixedAmount: cents(a)}
}

func TestComputePlan_PercentualExato(t *testing.T) {
	base := int64(500000)                                            // R$ 5.000
	dests := []Destination{dPercent("a", 2500), dPercent("b", 2000)} // 25% + 20%
	c := ComputePlan(base, dests)
	if c.ByDestination["a"] != 125000 || c.ByDestination["b"] != 100000 {
		t.Fatalf("valores: %+v", c.ByDestination)
	}
	if c.AllocatedAmount != 225000 || c.UnallocatedAmount != 275000 {
		t.Errorf("alocado/não-alocado errados: %d / %d", c.AllocatedAmount, c.UnallocatedAmount)
	}
	if c.AllocatedPercent != 4500 {
		t.Errorf("allocatedPercent=%d want 4500 (45%%)", c.AllocatedPercent)
	}
}

func TestComputePlan_RateioCentExato(t *testing.T) {
	base := int64(10001)                                             // valor ímpar p/ forçar sobra de centavo
	dests := []Destination{dPercent("a", 5000), dPercent("b", 5000)} // 50% + 50%
	c := ComputePlan(base, dests)
	soma := c.ByDestination["a"] + c.ByDestination["b"]
	if soma != 10001 {
		t.Fatalf("soma dos percentuais deve bater 100%% da base: got %d want 10001", soma)
	}
	// o centavo extra vai para o de menor id no empate de resto
	if c.ByDestination["a"] != 5001 || c.ByDestination["b"] != 5000 {
		t.Errorf("distribuição do centavo: %+v", c.ByDestination)
	}
}

func TestComputePlan_PercentualSobreBaseInteira(t *testing.T) {
	base := int64(10000)
	dests := []Destination{dFixed("f", 2000), dPercent("p", 5000)} // fixo 2000 + 50% da renda
	c := ComputePlan(base, dests)
	if c.ByDestination["f"] != 2000 {
		t.Errorf("fixo=%d want 2000", c.ByDestination["f"])
	}
	// percentual incide sobre a base inteira: 50% de 10000 = 5000
	if c.ByDestination["p"] != 5000 {
		t.Errorf("percentual sobre a base=%d want 5000", c.ByDestination["p"])
	}
	if c.AllocatedAmount != 7000 || c.UnallocatedAmount != 3000 {
		t.Errorf("alocado/não-alocado=%d/%d want 7000/3000", c.AllocatedAmount, c.UnallocatedAmount)
	}
}

func TestComputePlan_FixoMaisPercentualPodeExceder(t *testing.T) {
	// 100% em percentual + um fixo passa da renda → bloqueado pela validação.
	base := int64(10000)
	dests := []Destination{dPercent("p", 10000), dFixed("f", 1)}
	if err := ValidateAllocation(base, dests); err != ErrOverAllocated {
		t.Errorf("fixo + 100%% deveria exceder a renda, got %v", err)
	}
}

func TestComputePlan_BaseZero(t *testing.T) {
	c := ComputePlan(0, []Destination{dPercent("a", 5000)})
	if c.ByDestination["a"] != 0 || c.AllocatedAmount != 0 || c.UnallocatedAmount != 0 {
		t.Errorf("base zero deveria zerar tudo: %+v", c)
	}
}

func TestValidateAllocation(t *testing.T) {
	base := int64(100000)
	if err := ValidateAllocation(base, []Destination{dPercent("a", 7000), dPercent("b", 3000)}); err != nil {
		t.Errorf("100%% exato deveria passar: %v", err)
	}
	if err := ValidateAllocation(base, []Destination{dPercent("a", 7000), dPercent("b", 4000)}); err != ErrOverAllocated {
		t.Errorf("110%% deveria bloquear, got %v", err)
	}
	if err := ValidateAllocation(base, []Destination{dFixed("f", 150000)}); err != ErrOverAllocated {
		t.Errorf("fixo > base deveria bloquear, got %v", err)
	}
}

func TestValidateDestination(t *testing.T) {
	ok := DestinationInput{Reference: "2026-06", Name: "Aluguel", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500)}
	if err := ValidateDestination(ok); err != nil {
		t.Errorf("destino válido: %v", err)
	}
	if err := ValidateDestination(DestinationInput{Name: "", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500)}); err == nil {
		t.Error("nome vazio deveria falhar")
	}
	if err := ValidateDestination(DestinationInput{Name: "X", Kind: "outro", Mode: ModePercentual, Percentage: pct(1)}); err == nil {
		t.Error("kind inválido deveria falhar")
	}
	if err := ValidateDestination(DestinationInput{Name: "X", Kind: KindDespesa, Mode: ModePercentual}); err == nil {
		t.Error("percentual sem percentage deveria falhar")
	}
	if err := ValidateDestination(DestinationInput{Name: "X", Kind: KindDespesa, Mode: ModeValorFixo}); err == nil {
		t.Error("valor_fixo sem fixedAmount deveria falhar")
	}
}
