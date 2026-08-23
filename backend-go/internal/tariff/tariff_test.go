package tariff

import (
	"math"
	"testing"
)

func assertFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("%s: got %.4f, want %.4f", name, got, want)
	}
}

func TestFlatRate(t *testing.T) {
	bd, err := Calculate(Request{KWh: 100, Mode: "flat", FlatRateEGP: 2.5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "total EGP", bd.TotalCostEGP, 250.0)
	assertFloat(t, "effective rate", bd.EffectiveRateEGP, 2.5)
	if len(bd.Tiers) != 1 {
		t.Fatalf("expected 1 tier line, got %d", len(bd.Tiers))
	}
}

func TestTieredAtExactly100KWh(t *testing.T) {
	// Tier 1: 50 * 0.68 = 34; Tier 2: 50 * 0.78 = 39; total 73.
	bd, err := Calculate(Request{KWh: 100, Mode: "tiered"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "total EGP", bd.TotalCostEGP, 73.0)
	if len(bd.Tiers) != 7 {
		t.Fatalf("expected 7 tiers, got %d", len(bd.Tiers))
	}
	assertFloat(t, "tier1 kwh", bd.Tiers[0].KWhUsed, 50)
	assertFloat(t, "tier2 kwh", bd.Tiers[1].KWhUsed, 50)
	assertFloat(t, "tier3 kwh", bd.Tiers[2].KWhUsed, 0)
	assertFloat(t, "tier1 cost", bd.Tiers[0].CostEGP, 34.0)
	assertFloat(t, "tier2 cost", bd.Tiers[1].CostEGP, 39.0)
	assertFloat(t, "effective rate", bd.EffectiveRateEGP, 0.73)
}

func TestTieredAt300KWh(t *testing.T) {
	// 34 + 39 + 100*0.95(95) + 100*1.55(155) = 323.
	bd, err := Calculate(Request{KWh: 300, Mode: "tiered"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "total EGP", bd.TotalCostEGP, 323.0)
}

func TestTieredAt1000KWh(t *testing.T) {
	// 34 + 39 + 95 + 150*1.55(232.5) + 300*1.95(585) + 350*2.10(735) = 1720.5.
	bd, err := Calculate(Request{KWh: 1000, Mode: "tiered"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "total EGP", bd.TotalCostEGP, 1720.5)
	assertFloat(t, "tier7 kwh", bd.Tiers[6].KWhUsed, 0)
}

func TestTieredBeyondLastTier(t *testing.T) {
	// 1200 kWh fills tier 7 with 200 kWh at 2.23.
	bd, err := Calculate(Request{KWh: 1200, Mode: "tiered"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "tier7 kwh", bd.Tiers[6].KWhUsed, 200)
	assertFloat(t, "tier6 kwh", bd.Tiers[5].KWhUsed, 350)
}

func TestTieredZeroConsumption(t *testing.T) {
	bd, err := Calculate(Request{KWh: 0, Mode: "tiered"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "total EGP", bd.TotalCostEGP, 0.0)
}

func TestCustomTiers(t *testing.T) {
	custom := []Tier{
		{Name: "A", LowerKWh: 0, UpperKWh: 100, RateEGP: 1.0},
		{Name: "B", LowerKWh: 100, UpperKWh: 0, RateEGP: 2.0},
	}
	bd, err := Calculate(Request{KWh: 250, Mode: "tiered", Tiers: custom})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "tier A kwh", bd.Tiers[0].KWhUsed, 100)
	assertFloat(t, "tier B kwh", bd.Tiers[1].KWhUsed, 150)
	assertFloat(t, "total EGP", bd.TotalCostEGP, 400.0)
}

func TestValidationErrors(t *testing.T) {
	cases := []Request{
		{KWh: -1, Mode: "tiered"},
		{KWh: 100, Mode: "flat"}, // missing flat rate
		{KWh: 100, Mode: "nonsense"},
		{KWh: 100, Mode: "tiered", Tiers: []Tier{{LowerKWh: 10, UpperKWh: 50, RateEGP: 1}}},
		{KWh: 100, Mode: "tiered", Tiers: []Tier{{LowerKWh: 0, UpperKWh: 50, RateEGP: 1}, {LowerKWh: 60, UpperKWh: 0, RateEGP: 1}}},
		{KWh: 100, Mode: "tiered", Tiers: []Tier{{LowerKWh: 0, UpperKWh: 50, RateEGP: 1}}}, // no open top tier
	}
	for i, tc := range cases {
		if _, err := Calculate(tc); err == nil {
			t.Fatalf("case %d: expected an error, got none", i)
		}
	}
}

func TestRejectsAbsurdMagnitudes(t *testing.T) {
	// Regression: kwh=1e308 used to overflow total to +Inf and produce an
	// unmarshalable payload; every case here must return an error.
	cases := []Request{
		{KWh: 1e308, Mode: "flat", FlatRateEGP: 2.5},
		{KWh: math.Inf(1), Mode: "flat", FlatRateEGP: 2.5},
		{KWh: math.NaN(), Mode: "tiered"},
		{KWh: 1e9 + 1, Mode: "tiered"}, // above maxKWh cap
		{KWh: 100, Mode: "flat", FlatRateEGP: math.Inf(1)},
		{KWh: 100, Mode: "flat", FlatRateEGP: math.NaN()},
		// Note: USDPerEGP <= 0 (including -Inf) maps to the default rate by
		// contract, so only positive out-of-range values are rejected here.
		{KWh: 100, Mode: "flat", FlatRateEGP: 2.5, USDPerEGP: 2e6}, // above maxRateEGP cap
		{KWh: 100, Mode: "tiered", Tiers: []Tier{{LowerKWh: 0, UpperKWh: -5, RateEGP: 1}}},
		{KWh: 100, Mode: "tiered", Tiers: []Tier{{LowerKWh: 0, UpperKWh: 50, RateEGP: math.NaN()}}},
		{KWh: 100, Mode: "tiered", Tiers: []Tier{{LowerKWh: 0, UpperKWh: 50, RateEGP: 1}, {LowerKWh: 50, UpperKWh: 0, RateEGP: -math.Inf(1)}}},
	}
	for i, tc := range cases {
		bd, err := Calculate(tc)
		if err == nil {
			t.Fatalf("case %d: expected an error, got none (breakdown %+v)", i, bd)
		}
		if bd.Tiers != nil {
			t.Fatalf("case %d: expected empty breakdown on error, got %+v", i, bd)
		}
	}
}

func TestUSDAssertion(t *testing.T) {
	bd, err := Calculate(Request{KWh: 100, Mode: "flat", FlatRateEGP: 48.5, USDPerEGP: 48.5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFloat(t, "total USD", bd.TotalCostUSD, 100.0)
	assertFloat(t, "total EGP", bd.TotalCostEGP, 4850.0)
}
