// Package tariff implements Egyptian electrical tariff calculations:
// a flat-rate mode and the 7-tier Egyptian progressive (incremental)
// residential structure defined in the PRD.
//
// The tiered engine allocates kWh usage sequentially across consumption
// brackets (0-50, 51-100, 101-200, 201-350, 351-650, 651-1000, 1000+)
// using configurable rates. Default rates mirror the official Egyptian
// residential tariff effective September 2024. Callers may override both
// bracket limits and rates, which supports the editable bracket limits of
// the frontend setup wizard.
package tariff

import (
	"errors"
	"fmt"
	"math"
)

// DefaultUSDPerEGP is used for USD display when the request does not supply
// a conversion rate.
const DefaultUSDPerEGP = 48.5

// maxKWh bounds the accepted consumption so downstream money math cannot
// overflow to +/-Inf (which would otherwise produce an unmarshalable payload).
const maxKWh = 1e9

// maxRateEGP bounds per-kWh rates and the USD conversion factor.
const maxRateEGP = 1e6

// OpenUpperBound marks a tier that extends to infinity (no upper limit).
// Client-provided upper bounds at or above this threshold are treated as open.
const OpenUpperBound = 1e12

// Tier describes one billing bracket.
type Tier struct {
	Name     string  `json:"name,omitempty"` // optional human label, e.g. "Tier 1"
	LowerKWh float64 `json:"lower_kwh"`      // inclusive lower bound
	UpperKWh float64 `json:"upper_kwh"`      // exclusive upper bound; <=0 or >= OpenUpperBound means open-ended
	RateEGP  float64 `json:"rate_egp"`       // EGP per kWh
}

// TierLine is an itemized cost line for one bracket.
type TierLine struct {
	Name     string  `json:"name"`
	Label    string  `json:"label"` // e.g. "0-50 kWh" or "1000+ kWh"
	LowerKWh float64 `json:"lower_kwh"`
	UpperKWh float64 `json:"upper_kwh"`
	KWhUsed  float64 `json:"kwh_used"`
	RateEGP  float64 `json:"rate_egp"`
	CostEGP  float64 `json:"cost_egp"`
	CostUSD  float64 `json:"cost_usd"`
}

// Request is the POST /api/tariff/calculate payload.
type Request struct {
	KWh         float64 `json:"kwh"`                   // total consumption to bill
	Mode        string  `json:"mode"`                  // "flat" or "tiered"
	FlatRateEGP float64 `json:"flat_rate_egp"`         // EGP per kWh for flat mode
	Tiers       []Tier  `json:"tiers,omitempty"`       // optional override of the default Egyptian brackets
	USDPerEGP   float64 `json:"usd_per_egp,omitempty"` // optional conversion rate for USD display
}

// Breakdown is the itemized billing result returned to the caller.
type Breakdown struct {
	Mode             string     `json:"mode"`
	KWh              float64    `json:"kwh"`
	Currency         string     `json:"currency"`
	TotalCostEGP     float64    `json:"total_cost_egp"`
	TotalCostUSD     float64    `json:"total_cost_usd"`
	EffectiveRateEGP float64    `json:"effective_rate_per_kwh_egp"`
	Tiers            []TierLine `json:"tiers,omitempty"`
}

// DefaultEgyptianTiers returns the 7 official Egyptian residential brackets
// with the rates effective since September 2024.
func DefaultEgyptianTiers() []Tier {
	return []Tier{
		{Name: "Tier 1", LowerKWh: 0, UpperKWh: 50, RateEGP: 0.68},
		{Name: "Tier 2", LowerKWh: 50, UpperKWh: 100, RateEGP: 0.78},
		{Name: "Tier 3", LowerKWh: 100, UpperKWh: 200, RateEGP: 0.95},
		{Name: "Tier 4", LowerKWh: 200, UpperKWh: 350, RateEGP: 1.55},
		{Name: "Tier 5", LowerKWh: 350, UpperKWh: 650, RateEGP: 1.95},
		{Name: "Tier 6", LowerKWh: 650, UpperKWh: 1000, RateEGP: 2.10},
		{Name: "Tier 7", LowerKWh: 1000, UpperKWh: 0, RateEGP: 2.23},
	}
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

// ensureBounded rejects NaN/Inf, negative values, and magnitudes above max so
// that a single absurd input can never poison the billing math downstream.
func ensureBounded(value float64, name string, max float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be a finite number", name)
	}
	if value < 0 {
		return fmt.Errorf("%s must be non-negative", name)
	}
	if value > max {
		return fmt.Errorf("%s must not exceed %g", name, max)
	}
	return nil
}

// validate reports whether every numeric field of the breakdown is finite.
// It is the last line of defense: even if an overflow path slips past input
// validation, the caller gets an error instead of an unmarshalable payload.
func (b Breakdown) validate() error {
	const outOfRange = "computed billing result is out of representable range"
	for _, v := range []float64{b.KWh, b.TotalCostEGP, b.TotalCostUSD, b.EffectiveRateEGP} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New(outOfRange)
		}
	}
	for _, line := range b.Tiers {
		for _, v := range []float64{line.KWhUsed, line.CostEGP, line.CostUSD} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return errors.New(outOfRange)
			}
		}
	}
	return nil
}

func tierLabel(t Tier) string {
	if t.UpperKWh >= OpenUpperBound {
		return fmt.Sprintf("%.0f+ kWh", t.LowerKWh)
	}
	if t.LowerKWh == 0 {
		return fmt.Sprintf("0-%0.f kWh", t.UpperKWh)
	}
	return fmt.Sprintf("%.0f-%0.f kWh", t.LowerKWh+1, t.UpperKWh)
}

// normalizeTiers validates and normalizes the tier list so that it starts at
// zero, is contiguous, strictly increasing, and ends with an open bracket.
func normalizeTiers(tiers []Tier) ([]Tier, error) {
	if len(tiers) == 0 {
		return nil, errors.New("tiers must not be empty")
	}
	normalized := make([]Tier, len(tiers))
	copy(normalized, tiers)
	for i := range normalized {
		if normalized[i].LowerKWh < 0 || normalized[i].RateEGP < 0 {
			return nil, errors.New("tier lower bounds and rates must be non-negative")
		}
		if normalized[i].UpperKWh < 0 {
			return nil, errors.New("tier upper bounds must be non-negative")
		}
		if err := ensureBounded(normalized[i].LowerKWh, "tier lower bound", maxKWh); err != nil {
			return nil, err
		}
		if err := ensureBounded(normalized[i].RateEGP, "tier rate", maxRateEGP); err != nil {
			return nil, err
		}
		// Bounds that will be coerced to open-ended are exempt from the
		// magnitude cap; concrete upper bounds must stay within maxKWh.
		if normalized[i].UpperKWh > 0 && normalized[i].UpperKWh < OpenUpperBound {
			if err := ensureBounded(normalized[i].UpperKWh, "tier upper bound", maxKWh); err != nil {
				return nil, err
			}
		}
		if normalized[i].UpperKWh >= OpenUpperBound || normalized[i].UpperKWh == 0 {
			normalized[i].UpperKWh = math.MaxFloat64
		}
	}
	for i := range normalized {
		if i > 0 && normalized[i].LowerKWh != normalized[i-1].UpperKWh {
			return nil, fmt.Errorf("tiers must be contiguous; tier %d lower %v != previous upper %v", i, normalized[i].LowerKWh, normalized[i-1].UpperKWh)
		}
		if i > 0 && normalized[i].LowerKWh <= normalized[i-1].LowerKWh {
			return nil, fmt.Errorf("tiers must be strictly increasing at index %d", i)
		}
	}
	if normalized[0].LowerKWh != 0 {
		return nil, errors.New("first tier must start at 0 kWh")
	}
	if normalized[len(normalized)-1].UpperKWh != math.MaxFloat64 {
		return nil, errors.New("last tier must be open-ended")
	}
	return normalized, nil
}

// Calculate produces an itemized cost breakdown for the given request.
// Input magnitudes are bounded and the computed breakdown is verified to be
// finite, so callers never receive NaN/Inf values that cannot be marshaled
// to JSON.
func Calculate(req Request) (Breakdown, error) {
	bd, err := calculate(req)
	if err != nil {
		return Breakdown{}, err
	}
	if err := bd.validate(); err != nil {
		return Breakdown{}, err
	}
	return bd, nil
}

func calculate(req Request) (Breakdown, error) {
	if req.KWh < 0 {
		return Breakdown{}, errors.New("kwh must be non-negative")
	}
	if err := ensureBounded(req.KWh, "kwh", maxKWh); err != nil {
		return Breakdown{}, err
	}
	usdPerEGP := req.USDPerEGP
	if usdPerEGP <= 0 {
		usdPerEGP = DefaultUSDPerEGP
	} else if err := ensureBounded(usdPerEGP, "usd_per_egp", maxRateEGP); err != nil {
		return Breakdown{}, err
	}

	switch req.Mode {
	case "flat":
		if req.FlatRateEGP <= 0 {
			return Breakdown{}, errors.New("flat_rate_egp must be positive")
		}
		if err := ensureBounded(req.FlatRateEGP, "flat_rate_egp", maxRateEGP); err != nil {
			return Breakdown{}, err
		}
		total := req.KWh * req.FlatRateEGP
		total = round4(total)
		effective := req.FlatRateEGP
		if req.KWh > 0 {
			effective = round4(total / req.KWh)
		}
		lines := []TierLine{
			{
				Name:     "Flat Rate",
				Label:    "Flat Rate",
				LowerKWh: 0,
				UpperKWh: req.KWh,
				KWhUsed:  req.KWh,
				RateEGP:  req.FlatRateEGP,
				CostEGP:  total,
				CostUSD:  round4(total / usdPerEGP),
			},
		}
		return Breakdown{
			Mode:             "flat",
			KWh:              round4(req.KWh),
			Currency:         "EGP",
			TotalCostEGP:     total,
			TotalCostUSD:     round4(total / usdPerEGP),
			EffectiveRateEGP: effective,
			Tiers:            lines,
		}, nil

	case "tiered", "":
		tiers := req.Tiers
		if len(tiers) == 0 {
			tiers = DefaultEgyptianTiers()
		}
		normalized, err := normalizeTiers(tiers)
		if err != nil {
			return Breakdown{}, err
		}

		remaining := req.KWh
		lines := make([]TierLine, 0, len(normalized))
		var total float64
		for _, t := range normalized {
			width := t.UpperKWh - t.LowerKWh
			used := math.Min(remaining, width)
			if used < 0 {
				used = 0
			}
			cost := used * t.RateEGP
			line := TierLine{
				Name:     t.Name,
				Label:    tierLabel(t),
				LowerKWh: t.LowerKWh,
				UpperKWh: t.UpperKWh,
				KWhUsed:  round4(used),
				RateEGP:  t.RateEGP,
				CostEGP:  round4(cost),
				CostUSD:  round4(cost / usdPerEGP),
			}
			lines = append(lines, line)
			total += cost
			remaining -= used
			if remaining <= 0 {
				remaining = 0
			}
		}

		total = round4(total)
		effective := 0.0
		if req.KWh > 0 {
			effective = round4(total / req.KWh)
		}
		return Breakdown{
			Mode:             "tiered",
			KWh:              round4(req.KWh),
			Currency:         "EGP",
			TotalCostEGP:     total,
			TotalCostUSD:     round4(total / usdPerEGP),
			EffectiveRateEGP: effective,
			Tiers:            lines,
		}, nil

	default:
		return Breakdown{}, fmt.Errorf("mode %q is not supported; use \"flat\" or \"tiered\"", req.Mode)
	}
}
