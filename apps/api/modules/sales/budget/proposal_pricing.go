package budget

import (
	"bytes"
	"math/bits"
	"sort"
)

const basisPoints = 10_000

type PriceSelection struct {
	Mode  string `json:"mode"`
	Value *int64 `json:"value"`
}

type Discount struct {
	Mode  string `json:"mode,omitempty"`
	Value int64  `json:"value,omitempty"`
}

// portion returns the whole quotient and remainder without overflowing int64
// during multiplication. A quotient above the money limit is rejected.
func portion(amount, numerator, denominator int64) (int64, int64, bool) {
	hi, lo := bits.Mul64(uint64(amount), uint64(numerator))
	if hi >= uint64(denominator) {
		return 0, 0, false
	}
	whole, remainder := bits.Div64(hi, lo, uint64(denominator))
	if whole > maxSafeInteger {
		return 0, 0, false
	}
	return int64(whole), int64(remainder), true
}

func roundPortion(amount, numerator, denominator int64) (int64, bool) {
	whole, remainder, ok := portion(amount, numerator, denominator)
	if remainder >= (denominator+1)/2 {
		whole++
	}
	return whole, ok && validMoney(whole)
}

func unitPrice(cost int64, selection PriceSelection) (int64, bool) {
	if selection.Value == nil || !validMoney(*selection.Value) {
		return 0, false
	}
	value := *selection.Value
	switch selection.Mode {
	case "manual":
		return value, true
	case "markup":
		if cost > 0 {
			return roundPortion(cost, basisPoints+value, basisPoints)
		}
	case "margin":
		if cost > 0 && value < basisPoints {
			return roundPortion(cost, basisPoints, basisPoints-value)
		}
	}
	return 0, false
}

func appliedDiscount(subtotal int64, discount Discount) (int64, bool) {
	switch discount.Mode {
	case "":
		return 0, discount.Value == 0
	case "fixed":
		return discount.Value, discount.Value >= 0 && discount.Value <= subtotal
	case "percent":
		if discount.Value >= 0 && discount.Value <= basisPoints {
			return roundPortion(subtotal, discount.Value, basisPoints)
		}
	}
	return 0, false
}

// The sum of the allocations exactly matches the order discount. Allocate
// whole cents first, then the remaining cents by largest proportional remainder.
func allocateOrderDiscount(lines []ProposalLine, subtotal, discount int64) {
	if discount == 0 {
		return
	}
	type share struct {
		index     int
		remainder int64
	}
	shares := make([]share, len(lines))
	leftover := discount
	for i, line := range lines {
		whole, remainder, _ := portion(line.NetCents, discount, subtotal)
		lines[i].OrderDiscountCents = whole
		leftover -= whole
		shares[i] = share{i, remainder}
	}
	sort.Slice(shares, func(i, j int) bool {
		first, second := shares[i], shares[j]
		if first.remainder != second.remainder {
			return first.remainder > second.remainder
		}
		return bytes.Compare(lines[first.index].BudgetLineID[:], lines[second.index].BudgetLineID[:]) < 0
	})
	for _, share := range shares[:leftover] {
		lines[share.index].OrderDiscountCents++
	}
	for i := range lines {
		lines[i].NetCents -= lines[i].OrderDiscountCents
	}
}

func percentOf(profit, denominator int64) *float64 {
	if denominator == 0 {
		return nil
	}
	value := 100 * float64(profit) / float64(denominator)
	return &value
}
