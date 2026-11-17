package architecture

// Executable design examples for docs/architecture/BUSINESS_V1_1.md.
// These are not production services or a substitute for persistence tests.
import (
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"testing"
)

type businessSupply struct {
	id       string
	output   output
	assigned int64
}

type businessLine struct {
	required int64
	supply   []businessSupply
}

func businessProductionComplete(lines []businessLine) (bool, error) {
	if len(lines) == 0 {
		return false, fmt.Errorf("no demand")
	}
	// A source can supply several lines, but its aggregate claim is bounded.
	claimed := map[string]int64{}
	sources := map[string]output{}
	complete := true
	for _, line := range lines {
		if line.required <= 0 || line.required > maxSafeInteger {
			return false, fmt.Errorf("invalid demand")
		}
		var assigned int64
		for _, s := range line.supply {
			capacity, err := available(s.output)
			if err != nil || s.id == "" || s.assigned <= 0 {
				return false, fmt.Errorf("invalid supply")
			}
			if previous, exists := sources[s.id]; exists && previous != s.output {
				return false, fmt.Errorf("inconsistent source revision")
			}
			sources[s.id] = s.output
			if s.assigned > capacity-claimed[s.id] || s.assigned > line.required-assigned {
				return false, fmt.Errorf("capacity exceeded")
			}
			claimed[s.id] += s.assigned
			assigned += s.assigned
		}
		complete = complete && assigned == line.required
	}
	return complete, nil
}

func TestBusinessCompletionUsesAcceptedAssignedUnitsPerLine(t *testing.T) {
	accepted := output{"completed", 5, 5, 4, 1, true}
	for _, tc := range []struct {
		name     string
		lines    []businessLine
		complete bool
		invalid  bool
	}{
		{"shared accepted run", []businessLine{{2, []businessSupply{{"a", accepted, 2}}}, {2, []businessSupply{{"a", accepted, 2}}}}, true, false},
		{"partial line", []businessLine{{5, []businessSupply{{"a", accepted, 4}}}}, false, false},
		{"unstarted demand", []businessLine{{3, nil}}, false, false},
		{"every line required", []businessLine{{4, []businessSupply{{"a", accepted, 4}}}, {1, nil}}, false, false},
		{"multiple runs", []businessLine{{6, []businessSupply{{"a", accepted, 4}, {"b", accepted, 2}}}}, true, false},
		{"rejected units cannot fill demand", []businessLine{{5, []businessSupply{{"a", accepted, 5}}}}, false, true},
		{"same source claimed twice", []businessLine{{3, []businessSupply{{"a", accepted, 3}}}, {2, []businessSupply{{"a", accepted, 2}}}}, false, true},
		{"line overallocated", []businessLine{{2, []businessSupply{{"a", accepted, 3}}}}, false, true},
		{"empty order", nil, false, true},
		{"zero demand", []businessLine{{0, nil}}, false, true},
		{"unsafe quantity", []businessLine{{maxSafeInteger + 1, nil}}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := businessProductionComplete(tc.lines)
			if got != tc.complete || (err != nil) != tc.invalid {
				t.Fatalf("complete=%v err=%v; want %v invalid=%v", got, err, tc.complete, tc.invalid)
			}
		})
	}
	for _, o := range []output{
		{"planned", 5, 0, 0, 0, true},
		{"in_progress", 5, 5, 4, 0, true},
		{"failed", 5, 5, 0, 5, true},
		{"completed", 5, 5, 5, 0, false}, // Legacy reported output is unverified.
	} {
		if ok, err := businessProductionComplete([]businessLine{{1, []businessSupply{{"a", o, 1}}}}); ok || err == nil {
			t.Fatalf("accepted unusable output: %+v", o)
		}
	}
}

// Largest remainder with stable-ID tie breaking. big.Int avoids overflowing
// intermediate products even though input/output cents are JSON-safe integers.
func businessCostShares(cents int64, weights map[string]int64) (map[string]int64, error) {
	if cents < 0 || cents > maxSafeInteger || len(weights) == 0 {
		return nil, fmt.Errorf("invalid cost")
	}
	var total int64
	for id, weight := range weights {
		if id == "" || weight <= 0 || weight > maxSafeInteger-total {
			return nil, fmt.Errorf("invalid weights")
		}
		total += weight
	}
	type remainder struct {
		id    string
		value int64
	}
	remainders := make([]remainder, 0, len(weights))
	shares := make(map[string]int64, len(weights))
	left := cents
	for id, weight := range weights {
		product := new(big.Int).Mul(big.NewInt(cents), big.NewInt(weight))
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(product, big.NewInt(total), r)
		shares[id] = q.Int64()
		left -= shares[id]
		remainders = append(remainders, remainder{id, r.Int64()})
	}
	sort.Slice(remainders, func(i, j int) bool {
		if remainders[i].value == remainders[j].value {
			return remainders[i].id < remainders[j].id
		}
		return remainders[i].value > remainders[j].value
	})
	for i := int64(0); i < left; i++ {
		shares[remainders[i].id]++
	}
	return shares, nil
}

func TestBusinessMixedModelCostConservesRoundedCents(t *testing.T) {
	for _, tc := range []struct {
		cents   int64
		weights map[string]int64
		want    map[string]int64
	}{
		{100, map[string]int64{"b": 1, "a": 1, "c": 1}, map[string]int64{"a": 34, "b": 33, "c": 33}},
		{101, map[string]int64{"a": 1, "b": 2}, map[string]int64{"a": 34, "b": 67}},
		{1, map[string]int64{"c": 1, "b": 1, "a": 1}, map[string]int64{"a": 1, "b": 0, "c": 0}},
		{0, map[string]int64{"a": 1}, map[string]int64{"a": 0}},
		{maxSafeInteger, map[string]int64{"a": maxSafeInteger - 1, "b": 1}, map[string]int64{"a": maxSafeInteger - 1, "b": 1}},
	} {
		got, err := businessCostShares(tc.cents, tc.weights)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("shares=%v err=%v; want %v", got, err, tc.want)
		}
		var sum int64
		for _, n := range got {
			sum += n
		}
		if sum != tc.cents {
			t.Fatalf("lost cents: %d != %d", sum, tc.cents)
		}
	}
	for _, weights := range []map[string]int64{nil, {"": 1}, {"a": 0}, {"a": -1}, {"a": maxSafeInteger, "b": 1}} {
		if _, err := businessCostShares(100, weights); err == nil {
			t.Fatalf("accepted invalid weights %v", weights)
		}
	}
	for _, cents := range []int64{-1, maxSafeInteger + 1} {
		if _, err := businessCostShares(cents, map[string]int64{"a": 1}); err == nil {
			t.Fatalf("accepted invalid cents %d", cents)
		}
	}
}

func TestBusinessAcceptedUnitOrdinalsDoNotDuplicateResidualCents(t *testing.T) {
	// An item's 100-cent share across three accepted units: the residual cent
	// belongs to ordinal 1. Separate orders cannot each round 100/3 upward.
	share, accepted := int64(100), int64(3)
	unitCost := func(ordinal int64) int64 {
		cost := share / accepted
		if ordinal <= share%accepted {
			cost++
		}
		return cost
	}
	orderA := unitCost(1)
	orderB := unitCost(2) + unitCost(3)
	if orderA != 34 || orderB != 66 || orderA+orderB != share {
		t.Fatalf("allocation costs do not conserve item share: %d + %d", orderA, orderB)
	}
}

func businessPaymentState(payable, receipts, refunds int64) (string, int64, error) {
	if payable < 0 || receipts < 0 || refunds < 0 || payable > maxSafeInteger || receipts > maxSafeInteger || refunds > receipts {
		return "", 0, fmt.Errorf("invalid money")
	}
	net := receipts - refunds
	if net > payable {
		return "", 0, fmt.Errorf("overpayment")
	}
	balance := payable - net
	switch {
	case receipts > 0 && refunds == receipts:
		return "refunded", balance, nil
	case refunds > 0 && balance > 0:
		return "partially_refunded", balance, nil
	case balance == 0:
		return "paid", balance, nil
	case receipts > 0:
		return "partially_paid", balance, nil
	default:
		return "unpaid", balance, nil
	}
}

func TestBusinessPaymentStateIsIndependentOfProductionAndDelivery(t *testing.T) {
	for _, tc := range []struct {
		payable, receipts, refunds int64
		state                      string
		balance                    int64
	}{
		{1000, 0, 0, "unpaid", 1000},
		{1000, 400, 0, "partially_paid", 600},
		{1000, 1000, 0, "paid", 0},
		{1000, 1000, 200, "partially_refunded", 200},
		{1000, 400, 400, "refunded", 1000},
		{1000, 1200, 200, "paid", 0}, // A new payment settles the balance; history remains.
		{0, 0, 0, "paid", 0},
	} {
		state, balance, err := businessPaymentState(tc.payable, tc.receipts, tc.refunds)
		if err != nil || state != tc.state || balance != tc.balance {
			t.Fatalf("got %s/%d/%v; want %s/%d", state, balance, err, tc.state, tc.balance)
		}
	}
	for _, money := range [][3]int64{{100, 101, 0}, {100, 50, 51}, {-1, 0, 0}, {100, -1, 0}, {100, 0, -1}, {maxSafeInteger + 1, 0, 0}} {
		if _, _, err := businessPaymentState(money[0], money[1], money[2]); err == nil {
			t.Fatalf("accepted invalid payment totals: %v", money)
		}
	}
}
