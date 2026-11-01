package saleshandlers

import (
	"errors"
	"fmt"
	corehandlers "github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"math"
	"math/big"

	"github.com/google/uuid"
)

var errSalesMoneyOverflow = errors.New("Sales monetary calculation exceeds supported range")

func calculateSalesOrderProfitability(order salesmodels.SalesOrder, cfg map[string]float64) (salesdto.SalesOrderProfitabilityDTO, error) {
	if order.FulfilmentStatus == salesmodels.SalesOrderFulfilmentCompleted {
		return calculateFrozenSalesOrderProfitability(order)
	}
	return calculateLiveSalesOrderProfitability(order, cfg)
}

func calculateLiveSalesOrderProfitability(order salesmodels.SalesOrder, cfg map[string]float64) (salesdto.SalesOrderProfitabilityDTO, error) {
	result, _, err := resolveLiveSalesOrderProfitability(order, cfg)
	return result, err
}

func resolveLiveSalesOrderProfitability(
	order salesmodels.SalesOrder,
	cfg map[string]float64,
) (salesdto.SalesOrderProfitabilityDTO, map[uuid.UUID]int64, error) {
	revenue, err := salesOrderRevenueCents(order)
	if err != nil {
		return salesdto.SalesOrderProfitabilityDTO{}, nil, err
	}
	additional, err := salesOrderAdditionalCostsCents(order)
	if err != nil {
		return salesdto.SalesOrderProfitabilityDTO{}, nil, err
	}

	result := salesdto.SalesOrderProfitabilityDTO{
		State:                "calculated",
		RevenueCents:         revenue,
		AdditionalCostsCents: additional,
		Margin:               salesdto.SalesMarginDTO{State: "not_calculated", Reason: "Revenue is zero"},
	}
	allocationCosts := make(map[uuid.UUID]int64)

	var production int64
	jobCosts := make(map[uuid.UUID]int64)
	for _, item := range order.Items {
		for _, allocation := range item.Allocations {
			if allocation.PrintJobID == nil || allocation.PrintJob == nil {
				return unavailableSalesProfitability(result, "A referenced Print Job is unavailable"), nil, nil
			}
			job := *allocation.PrintJob
			if job.UnitsPrinted <= 0 {
				return unavailableSalesProfitability(result, fmt.Sprintf("Print Job %s has no printable units", job.ID)), nil, nil
			}

			jobCostCents, exists := jobCosts[job.ID]
			if !exists {
				costState := corehandlers.CalculateProductionPrintJobCostState(job, cfg)
				if costState.State != "calculated" || costState.Total == nil {
					reason := costState.Reason
					if reason == "" {
						reason = "Production cost is unavailable"
					}
					return unavailableSalesProfitability(
						result,
						fmt.Sprintf("Print Job %s: %s", job.ID, reason),
					), nil, nil
				}
				var ok bool
				jobCostCents, ok = eurosToCents(*costState.Total)
				if !ok || jobCostCents < 0 {
					return unavailableSalesProfitability(
						result,
						fmt.Sprintf("Print Job %s: Production cost is unavailable", job.ID),
					), nil, nil
				}
				jobCosts[job.ID] = jobCostCents
			}

			allocatedCost, ok := proportionalCostCents(
				jobCostCents,
				int64(allocation.UnitsAllocated),
				int64(job.UnitsPrinted),
			)
			if !ok {
				return salesdto.SalesOrderProfitabilityDTO{}, nil, errSalesMoneyOverflow
			}
			production, ok = checkedAddInt64(production, allocatedCost)
			if !ok {
				return salesdto.SalesOrderProfitabilityDTO{}, nil, errSalesMoneyOverflow
			}
			allocationCosts[allocation.ID] = allocatedCost
		}
	}

	total, ok := checkedAddInt64(production, additional)
	if !ok {
		return salesdto.SalesOrderProfitabilityDTO{}, nil, errSalesMoneyOverflow
	}
	netProfit, ok := checkedSubtractInt64(revenue, total)
	if !ok {
		return salesdto.SalesOrderProfitabilityDTO{}, nil, errSalesMoneyOverflow
	}
	result.ProductionCostCents = int64Pointer(production)
	result.TotalCostCents = int64Pointer(total)
	result.NetProfitCents = int64Pointer(netProfit)
	if revenue > 0 {
		basisPoints, ok := salesMarginBasisPoints(netProfit, revenue)
		if !ok {
			return salesdto.SalesOrderProfitabilityDTO{}, nil, errSalesMoneyOverflow
		}
		margin := float64(basisPoints) / 100
		result.Margin = salesdto.SalesMarginDTO{State: "calculated", Percent: &margin}
	}
	return result, allocationCosts, nil
}

func calculateFrozenSalesOrderProfitability(order salesmodels.SalesOrder) (salesdto.SalesOrderProfitabilityDTO, error) {
	revenue, err := salesOrderRevenueCents(order)
	if err != nil {
		return salesdto.SalesOrderProfitabilityDTO{}, err
	}
	additional, err := salesOrderAdditionalCostsCents(order)
	if err != nil {
		return salesdto.SalesOrderProfitabilityDTO{}, err
	}
	result := salesdto.SalesOrderProfitabilityDTO{
		State:                "calculated",
		RevenueCents:         revenue,
		AdditionalCostsCents: additional,
		Margin:               salesdto.SalesMarginDTO{State: "not_calculated", Reason: "Revenue is zero"},
	}
	if order.ProductionCostCents == nil || order.TotalCostCents == nil || order.NetProfitCents == nil {
		return unavailableSalesProfitability(result, "Completed Sales Order cost snapshot is unavailable"), nil
	}
	result.ProductionCostCents = int64Pointer(*order.ProductionCostCents)
	result.TotalCostCents = int64Pointer(*order.TotalCostCents)
	result.NetProfitCents = int64Pointer(*order.NetProfitCents)
	if revenue > 0 {
		if order.MarginBasisPoints == nil {
			return unavailableSalesProfitability(result, "Completed Sales Order margin snapshot is unavailable"), nil
		}
		margin := float64(*order.MarginBasisPoints) / 100
		result.Margin = salesdto.SalesMarginDTO{State: "calculated", Percent: &margin}
	}
	return result, nil
}

func salesMarginBasisPoints(netProfitCents, revenueCents int64) (int64, bool) {
	if revenueCents <= 0 {
		return 0, false
	}
	numerator := new(big.Int).Mul(big.NewInt(netProfitCents), big.NewInt(10000))
	denominator := big.NewInt(revenueCents)
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)

	twiceRemainder := new(big.Int).Lsh(new(big.Int).Abs(remainder), 1)
	if twiceRemainder.Cmp(denominator) >= 0 {
		if numerator.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	if !quotient.IsInt64() {
		return 0, false
	}
	return quotient.Int64(), true
}

func unavailableSalesProfitability(
	base salesdto.SalesOrderProfitabilityDTO,
	reason string,
) salesdto.SalesOrderProfitabilityDTO {
	base.State = "not_calculated"
	base.Reason = reason
	base.Margin = salesdto.SalesMarginDTO{State: "not_calculated", Reason: reason}
	return base
}

func salesOrderRevenueCents(order salesmodels.SalesOrder) (int64, error) {
	var revenue int64
	for _, item := range order.Items {
		line, ok := checkedMultiplyNonnegative(item.UnitPriceCents, int64(item.Quantity))
		if !ok {
			return 0, errSalesMoneyOverflow
		}
		revenue, ok = checkedAddInt64(revenue, line)
		if !ok {
			return 0, errSalesMoneyOverflow
		}
	}
	return revenue, nil
}

func salesOrderAdditionalCostsCents(order salesmodels.SalesOrder) (int64, error) {
	var total int64
	for _, value := range []int64{
		order.PackagingCostCents,
		order.ShippingCostCents,
		order.PlatformFeeCents,
		order.OtherCostCents,
	} {
		var ok bool
		total, ok = checkedAddInt64(total, value)
		if !ok {
			return 0, errSalesMoneyOverflow
		}
	}
	return total, nil
}

func eurosToCents(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) ||
		value >= float64(math.MaxInt64)/100 ||
		value < float64(math.MinInt64)/100 {
		return 0, false
	}
	return int64(math.Round(value * 100)), true
}

// proportionalCostCents rounds half-up to the nearest cent without converting
// persisted Sales money through floating point or overflowing intermediate
// multiplication.
func proportionalCostCents(totalCostCents, allocatedUnits, unitsPrinted int64) (int64, bool) {
	if totalCostCents < 0 || allocatedUnits < 0 || unitsPrinted <= 0 {
		return 0, false
	}
	numerator := new(big.Int).Mul(big.NewInt(totalCostCents), big.NewInt(allocatedUnits))
	numerator.Add(numerator, new(big.Int).Quo(big.NewInt(unitsPrinted), big.NewInt(2)))
	numerator.Quo(numerator, big.NewInt(unitsPrinted))
	if !numerator.IsInt64() {
		return 0, false
	}
	return numerator.Int64(), true
}

func checkedMultiplyNonnegative(left, right int64) (int64, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}
	if left != 0 && right > math.MaxInt64/left {
		return 0, false
	}
	return left * right, true
}

func checkedAddInt64(left, right int64) (int64, bool) {
	if (right > 0 && left > math.MaxInt64-right) ||
		(right < 0 && left < math.MinInt64-right) {
		return 0, false
	}
	return left + right, true
}

func checkedSubtractInt64(left, right int64) (int64, bool) {
	if (right > 0 && left < math.MinInt64+right) ||
		(right < 0 && left > math.MaxInt64+right) {
		return 0, false
	}
	return left - right, true
}

func int64Pointer(value int64) *int64 {
	return &value
}
