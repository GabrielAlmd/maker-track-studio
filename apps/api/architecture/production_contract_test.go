// Package architecture contains executable design examples, not runtime APIs.
// See docs/architecture/PRODUCTION_V1_1.md. Persistence remains gated on BUS-01.
package architecture

import (
	"fmt"
	"reflect"
	"testing"
)

const maxSafeInteger int64 = 9007199254740991

type plate struct {
	repetitions, minutes, grams int64
	items                       map[string]int64
}

type totals struct {
	units          map[string]int64
	minutes, grams int64
}

func addProduct(total, value, multiplier int64) (int64, error) {
	if value <= 0 || multiplier <= 0 || total < 0 || total > maxSafeInteger || value > (maxSafeInteger-total)/multiplier {
		return 0, fmt.Errorf("invalid or overflowing quantity")
	}
	return total + value*multiplier, nil
}

// Test-only reference formula. It intentionally does not implement persistence.
func planned(plates []plate) (totals, error) {
	result := totals{units: map[string]int64{}}
	for _, p := range plates {
		if len(p.items) == 0 {
			return totals{}, fmt.Errorf("empty plate")
		}
		var err error
		if result.minutes, err = addProduct(result.minutes, p.minutes, p.repetitions); err != nil {
			return totals{}, err
		}
		if result.grams, err = addProduct(result.grams, p.grams, p.repetitions); err != nil {
			return totals{}, err
		}
		for model, count := range p.items {
			if model == "" {
				return totals{}, fmt.Errorf("missing identity")
			}
			if result.units[model], err = addProduct(result.units[model], count, p.repetitions); err != nil {
				return totals{}, err
			}
		}
	}
	return result, nil
}

func TestProductionPlanMixedPlatesAndVariableEstimates(t *testing.T) {
	plates := []plate{{3, 60, 30, map[string]int64{"bracket": 2, "cap": 1}}, {2, 25, 12, map[string]int64{"bracket": 1}}}
	got, err := planned(plates)
	want := totals{map[string]int64{"bracket": 8, "cap": 3}, 230, 114}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
	plates[1].minutes = 40
	got, err = planned(plates)
	if err != nil || got.minutes != 260 || got.grams != 114 || !reflect.DeepEqual(got.units, want.units) {
		t.Fatalf("override scaled unrelated estimates: %+v %v", got, err)
	}
}

func TestProductionPlanRejectsInvalidCountsAndOverflow(t *testing.T) {
	for _, p := range []plate{
		{0, 60, 30, map[string]int64{"a": 1}},
		{-1, 60, 30, map[string]int64{"a": 1}},
		{1, 60, 30, map[string]int64{"a": 0}},
		{1, 60, 30, map[string]int64{"a": -1}},
		{2, 1, 1, map[string]int64{"a": maxSafeInteger}},
		{2, maxSafeInteger, 1, map[string]int64{"a": 1}},
		{2, 1, maxSafeInteger, map[string]int64{"a": 1}},
		{1, 60, 30, nil},
	} {
		if _, err := planned([]plate{p}); err == nil {
			t.Errorf("accepted invalid plate: %+v", p)
		}
	}
	_, err := planned([]plate{{1, 1, 1, map[string]int64{"a": maxSafeInteger}}, {1, 1, 1, map[string]int64{"a": 1}}})
	if err == nil {
		t.Fatal("accepted aggregate overflow")
	}
	if n, err := addProduct(0, maxSafeInteger, 1); err != nil || n != maxSafeInteger {
		t.Fatal("rejected exact boundary")
	}
}

type output struct {
	status                                 string
	planned, attempted, accepted, rejected int64
	verified                               bool
}

func available(o output) (int64, error) {
	if o.planned <= 0 || o.planned > maxSafeInteger || o.attempted < 0 || o.attempted > o.planned || o.accepted < 0 || o.rejected < 0 || o.accepted > o.attempted || o.rejected > o.attempted-o.accepted {
		return 0, fmt.Errorf("invalid output counts")
	}
	switch o.status {
	case "planned":
		if o.attempted != 0 {
			return 0, fmt.Errorf("unstarted output")
		}
	case "in_progress":
		return 0, nil
	case "completed", "failed", "cancelled":
		if o.accepted+o.rejected != o.attempted {
			return 0, fmt.Errorf("unclassified terminal output")
		}
		if o.status != "completed" && o.accepted != 0 {
			return 0, fmt.Errorf("non-completed accepted output")
		}
	default:
		return 0, fmt.Errorf("unknown status")
	}
	if o.status == "completed" && o.verified {
		return o.accepted, nil
	}
	return 0, nil
}

func TestProductionAcceptedOutputContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		output  output
		want    int64
		invalid bool
	}{
		{"success", output{"completed", 2, 2, 2, 0, true}, 2, false},
		{"partial", output{"completed", 2, 2, 1, 1, true}, 1, false},
		{"failed", output{"failed", 2, 2, 0, 2, true}, 0, false},
		{"planned repetition", output{"planned", 2, 0, 0, 0, true}, 0, false},
		{"pending inspection", output{"in_progress", 2, 2, 1, 0, true}, 0, false},
		{"historical unverified", output{"completed", 2, 2, 2, 0, false}, 0, false},
		{"cancel before start", output{"cancelled", 2, 0, 0, 0, true}, 0, false},
		{"overproduction", output{"completed", 2, 3, 3, 0, true}, 0, true},
		{"negative", output{"completed", 2, 2, -1, 3, true}, 0, true},
		{"unclassified terminal", output{"completed", 2, 2, 1, 0, true}, 0, true},
		{"failed accepted", output{"failed", 2, 2, 1, 1, true}, 0, true},
		{"planned output", output{"planned", 2, 2, 2, 0, true}, 0, true},
		{"unknown status", output{"unknown", 2, 0, 0, 0, true}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := available(tc.output)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got %d, %v; want %d invalid=%t", got, err, tc.want, tc.invalid)
			}
		})
	}
}
