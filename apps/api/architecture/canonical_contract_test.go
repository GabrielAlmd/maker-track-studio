package architecture

// These worked examples specify ARC-01 arithmetic. Runtime parity, source
// validation and persistence are covered by the implementation issues.
import (
	"math/big"
	"testing"
)

type referenceOutput struct {
	accepted  int64
	unitCents *big.Rat
	eligible  bool
}

func referenceUnitCents(outputs ...referenceOutput) *big.Rat {
	var accepted int64
	total := new(big.Rat)
	for _, output := range outputs {
		if !output.eligible || output.accepted <= 0 || output.accepted > maxSafeInteger-accepted ||
			output.unitCents == nil || output.unitCents.Sign() < 0 {
			return nil
		}
		accepted += output.accepted
		total.Add(total, new(big.Rat).Mul(output.unitCents, big.NewRat(output.accepted, 1)))
	}
	return perAcceptedUnit(total, accepted)
}

func perAcceptedUnit(attributableCents *big.Rat, accepted int64) *big.Rat {
	if attributableCents == nil || accepted <= 0 {
		return nil
	}
	return new(big.Rat).Quo(attributableCents, big.NewRat(accepted, 1))
}

func roundedContractCents(cents *big.Rat) (int64, bool) {
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(cents.Num(), cents.Denom(), remainder)
	remainder.Abs(remainder).Lsh(remainder, 1)
	if remainder.Cmp(cents.Denom()) >= 0 {
		whole.Add(whole, big.NewInt(int64(cents.Sign())))
	}
	if !whole.IsInt64() || whole.Int64() > maxSafeInteger || whole.Int64() < -maxSafeInteger {
		return 0, false
	}
	return whole.Int64(), true
}

func TestCommercialReferenceWeightsAcceptedUnits(t *testing.T) {
	machineA := referenceOutput{60, big.NewRat(150, 1), true}
	machineB := referenceOutput{40, big.NewRat(130, 1), true}
	for _, tc := range []struct {
		name    string
		outputs []referenceOutput
		want    string
	}{
		{"60 at 150 plus 40 at 130", []referenceOutput{machineA, machineB}, "142"},
		{"order of inputs does not change the result", []referenceOutput{machineB, machineA}, "142"},
		{"single eligible machine", []referenceOutput{machineA}, "150"},
		{"known zero cost", []referenceOutput{{2, big.NewRat(0, 1), true}}, "0"},
		{"no reliable mix", nil, "<nil>"},
		{"zero expected accepted output", []referenceOutput{{0, big.NewRat(100, 1), true}}, "<nil>"},
		{"missing price", []referenceOutput{machineA, {40, nil, true}}, "<nil>"},
		{"archived or incompatible member requires review", []referenceOutput{machineA, {40, machineB.unitCents, false}}, "<nil>"},
		{"aggregate quantity outside JSON-safe bounds", []referenceOutput{machineA, {maxSafeInteger, machineB.unitCents, true}}, "<nil>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := referenceUnitCents(tc.outputs...)
			if got == nil && tc.want == "<nil>" {
				return
			}
			if got == nil || got.RatString() != tc.want {
				t.Fatalf("reference cents per accepted unit = %v, want %s", got, tc.want)
			}
		})
	}
}

func TestEstimateLineRoundsAfterScaling(t *testing.T) {
	// Each component is half a cent. Rounding the two components separately
	// would give 2 cents instead of the 1-cent total.
	total := new(big.Rat).Add(big.NewRat(1, 2), big.NewRat(1, 2))
	if got, ok := roundedContractCents(total); !ok || got != 1 {
		t.Fatalf("total cents = %d, valid = %v, want 1 cent", got, ok)
	}

	// A 100.49-cent reference quoted for 3 pieces costs 301.47 cents;
	// multiplying the displayed one-euro average would lose a cent.
	unit := referenceUnitCents(referenceOutput{1, big.NewRat(10049, 100), true})
	if unit == nil {
		t.Fatal("reference unexpectedly unavailable")
	}
	line := new(big.Rat).Mul(unit, big.NewRat(3, 1))
	if got, ok := roundedContractCents(line); !ok || got != 301 {
		t.Fatalf("line cents = %d, valid = %v, want 301 cents", got, ok)
	}
	if got, ok := roundedContractCents(big.NewRat(201, 2)); !ok || got != 101 {
		t.Fatalf("half-cent tie = %d, valid = %v, want 101 cents", got, ok)
	}
	if _, ok := roundedContractCents(big.NewRat(maxSafeInteger*2+1, 2)); ok {
		t.Fatal("accepted an amount rounded above the JSON-safe limit")
	}
}

func TestReferencePriceSuggestionsKeepMarginAndMarkupDistinct(t *testing.T) {
	// A 25% markup on 142 cents yields 177.5 cents. A 25% margin
	// instead yields 189 1/3 cents. The selling price is rounded once.
	cost := big.NewRat(142, 1)
	markup := new(big.Rat).Mul(cost, big.NewRat(12500, 10000))
	margin := new(big.Rat).Quo(cost, big.NewRat(7500, 10000))
	if got, ok := roundedContractCents(markup); !ok || got != 178 {
		t.Fatalf("markup suggestion = %d, valid = %v, want 178 cents", got, ok)
	}
	if got, ok := roundedContractCents(margin); !ok || got != 189 {
		t.Fatalf("margin suggestion = %d, valid = %v, want 189 cents", got, ok)
	}
}

func TestActualCostUsesAttributableOutputAcrossMachines(t *testing.T) {
	// One 101-cent mixed run on machine A. The bracket share, including its
	// rejected output, is 67 cents for two accepted brackets. The second
	// item's 34 cents stay with its expense, since no caps were accepted.
	mixed, err := businessCostShares(101, map[string]int64{"bracket": 2, "cap": 1})
	if err != nil || mixed["bracket"] != 67 || mixed["cap"] != 34 {
		t.Fatalf("mixed run shares = %v, err = %v, want bracket=67 and cap=34", mixed, err)
	}
	if got := perAcceptedUnit(big.NewRat(mixed["cap"], 1), 0); got != nil {
		t.Fatalf("zero accepted output produced a unit cost: %s", got)
	}
	if got := perAcceptedUnit(nil, 2); got != nil {
		t.Fatalf("missing attributable cost produced a unit cost: %s", got)
	}
	// Add 97 cents from 3 accepted brackets on machine B. The total is
	// 164 cents for 5 accepted brackets, or exactly 32.8 cents per unit.
	brackets := perAcceptedUnit(big.NewRat(mixed["bracket"]+97, 1), 5)
	if brackets == nil || brackets.Cmp(big.NewRat(164, 5)) != 0 {
		t.Fatalf("aggregate actual cost per accepted unit = %v, want 32.8 cents", brackets)
	}
}
