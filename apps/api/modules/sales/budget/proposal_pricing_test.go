package budget

import (
	"testing"

	"github.com/google/uuid"
)

func TestProposalUnitPrice(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  string
		cost  int64
		value int64
		want  int64
		ok    bool
	}{
		{"20 percent margin", "margin", 800, 2000, 1000, true},
		{"20 percent markup", "markup", 800, 2000, 960, true},
		{"25 percent markup", "markup", 800, 2500, 1000, true},
		{"half up", "markup", 1, 5000, 2, true},
		{"manual zero", "manual", 0, 0, 0, true},
		{"zero cost target", "margin", 0, 5000, 0, false},
		{"100 percent margin", "margin", 800, 10000, 0, false},
		{"negative markup", "markup", 800, -1, 0, false},
		{"price overflow", "margin", maxSafeInteger, 9999, 0, false},
		{"wide multiplication", "margin", maxSafeInteger, 0, maxSafeInteger, true},
		{"missing selection", "", 800, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := unitPrice(tc.cost, PriceSelection{tc.mode, &tc.value})
			if ok != tc.ok || (ok && got != tc.want) {
				t.Fatalf("got (%d, %t), want (%d, %t)", got, ok, tc.want, tc.ok)
			}
		})
	}
	if _, ok := unitPrice(800, PriceSelection{Mode: "manual"}); ok {
		t.Fatal("missing manual price treated as explicit zero")
	}
	record := proposalView(Proposal{EstimatedCostCents: 800, NetCents: 1000})
	if *record.MarginPercent != 20 || *record.MarkupPercent != 25 || record.ProfitCents != 200 {
		t.Fatalf("unexpected comparison: %+v", record)
	}
	if proposalView(Proposal{}).MarginPercent != nil || proposalView(Proposal{}).MarkupPercent != nil {
		t.Fatal("zero denominators must be unavailable")
	}
}

func TestProposalDiscountAllocation(t *testing.T) {
	for _, tc := range []struct {
		subtotal int64
		discount Discount
		want     int64
		ok       bool
	}{
		{5, Discount{"percent", 5000}, 3, true},
		{maxSafeInteger, Discount{"percent", 10000}, maxSafeInteger, true},
		{5, Discount{"percent", 10001}, 0, false},
		{5, Discount{"fixed", 6}, 6, false},
		{5, Discount{"fixed", -1}, -1, false},
	} {
		if amount, ok := appliedDiscount(tc.subtotal, tc.discount); ok != tc.ok || (ok && amount != tc.want) {
			t.Fatalf("subtotal %d, discount %+v: got (%d, %t), want (%d, %t)",
				tc.subtotal, tc.discount, amount, ok, tc.want, tc.ok)
		}
	}
	lines := []ProposalLine{
		{ProposalLineInput: ProposalLineInput{BudgetLineID: uuid.UUID{2}}, NetCents: 1},
		{ProposalLineInput: ProposalLineInput{BudgetLineID: uuid.UUID{1}}, NetCents: 1},
		{ProposalLineInput: ProposalLineInput{BudgetLineID: uuid.UUID{3}}, NetCents: 5},
		{ProposalLineInput: ProposalLineInput{BudgetLineID: uuid.UUID{0}}, NetCents: 0},
	}
	allocateOrderDiscount(lines, 7, 4)
	want := []int64{0, 1, 3, 0}
	for i, line := range lines {
		if line.OrderDiscountCents != want[i] || line.NetCents < 0 {
			t.Fatalf("line %d: allocated %d, want %d, net %d", i, line.OrderDiscountCents, want[i], line.NetCents)
		}
	}
}
